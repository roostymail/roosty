// Command roosty runs the Roosty Mail server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/roostymail/roosty/server/internal/server"
	"github.com/roostymail/roosty/server/internal/settings"
	"github.com/roostymail/roosty/server/internal/store"
	"github.com/roostymail/roosty/server/internal/web"
)

var version = "dev"

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	listen := env("ROOSTY_LISTEN", ":8080")
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		resp, err := http.Get("http://127.0.0.1" + listen + "/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	level := slog.LevelInfo
	if os.Getenv("ROOSTY_DEBUG") == "true" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	if err := run(listen); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(listen string) error {
	dataDir := env("ROOSTY_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	st, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	cfg, err := settings.NewManager(st)
	if err != nil {
		return err
	}
	srv, err := server.New(st, cfg, server.Options{DataDir: dataDir, Web: web.FS(), Version: version, SetupToken: os.Getenv("ROOSTY_SETUP_TOKEN")})
	if err != nil {
		return err
	}
	hs := &http.Server{Addr: listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	slog.Info("Roosty Mail listening", "addr", listen, "version", version, "mailConfigured", cfg.Configured())
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
