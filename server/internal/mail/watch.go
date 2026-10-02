package mail

import (
	"context"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/roostymail/roosty/server/internal/settings"
)

// Watch keeps a dedicated connection in IDLE on a mailbox and calls notify
// whenever the server reports a change. It returns when ctx is done or the
// connection fails.
func Watch(ctx context.Context, ms settings.MailServer, cr Creds, mailbox string, notify func()) error {
	changed := make(chan struct{}, 1)
	signal := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	opts := &imapclient.Options{UnilateralDataHandler: &imapclient.UnilateralDataHandler{
		Expunge: func(uint32) { signal() },
		Mailbox: func(d *imapclient.UnilateralDataMailbox) {
			if d.NumMessages != nil {
				signal()
			}
		},
		Fetch: func(*imapclient.FetchMessageData) { signal() },
	}}
	c, err := Login(ms, cr, opts)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-changed:
				notify()
			}
		}
	}()
	for {
		idle, err := c.Idle()
		if err != nil {
			return err
		}
		timer := time.NewTimer(20 * time.Minute)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = idle.Close()
			_ = idle.Wait()
			_ = c.Logout().Wait()
			return nil
		case <-c.Closed():
			timer.Stop()
			return context.Canceled
		case <-timer.C:
			if err := idle.Close(); err != nil {
				return err
			}
			if err := idle.Wait(); err != nil {
				return err
			}
		}
	}
}
