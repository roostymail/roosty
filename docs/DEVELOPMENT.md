# Running Roosty Mail locally

## With Docker (recommended)

Requirements: Docker with Compose.

```sh
docker compose up --build -d
```

This starts three containers:

| Service | What it does |
|---|---|
| `roosty` | Roosty Mail on http://localhost:8080 |
| `greenmail` | A test IMAP/SMTP server with two mailboxes |
| `seed` | Delivers 7 sample emails once (including an XSS test message) |

Test accounts:

| Email | Password |
|---|---|
| `marina@roosty.test` | `roosty123` |
| `ana@roosty.test` | `roosty123` |

First run:

1. Open http://localhost:8080/admin/setup.
2. Enter the setup code `roosty-local-setup` (in production the code is random and printed in `docker compose logs roosty`).
3. Create the admin account and finish the wizard. The mail server fields are locked because they come from environment variables in `docker-compose.yml`.
4. Sign in to the webmail at http://localhost:8080 with one of the test accounts.

Send mail between `marina@` and `ana@` to see real-time delivery. Reset everything with `docker compose down -v`.

## Without Docker

Requirements: Go 1.27+, Node 22+.

```sh
docker compose -f deploy/dev/compose.mail.yml up -d   # test mail server + sample emails on ports 3143/3025
make web                                              # build the web app into the Go embed folder
cd server && ROOSTY_IMAP_HOST=127.0.0.1 ROOSTY_IMAP_PORT=3143 ROOSTY_IMAP_SECURITY=none \
  ROOSTY_SMTP_HOST=127.0.0.1 ROOSTY_SMTP_PORT=3025 ROOSTY_SMTP_SECURITY=none \
  ROOSTY_LISTEN=:8090 go run ./cmd/roosty
```

For frontend work, run `npm run dev` in `web/` (Vite on :5173 proxies `/api` to :8090).

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `ROOSTY_LISTEN` | `:8080` | HTTP listen address |
| `ROOSTY_DATA_DIR` | `./data` (`/data` in Docker) | SQLite database, uploads and logos |
| `ROOSTY_SETUP_TOKEN` | random | First-run setup code |
| `ROOSTY_IMAP_HOST` / `_PORT` / `_SECURITY` | | IMAP server; security is `tls`, `starttls` or `none` |
| `ROOSTY_SMTP_HOST` / `_PORT` / `_SECURITY` | | SMTP submission server |
| `ROOSTY_TLS_SKIP_VERIFY` | `false` | Accept invalid certificates (testing only) |
| `ROOSTY_ACCESS_MODE` | `domains` | `domains` or `list` |
| `ROOSTY_ALLOWED_DOMAINS` | | Comma-separated domains allowed to sign in |
| `ROOSTY_ALLOWED_ACCOUNTS` | | Comma-separated addresses (with `list` mode) |
| `ROOSTY_BRAND_NAME` / `ROOSTY_BRAND_ACCENT` / `ROOSTY_DEFAULT_THEME` | | Branding defaults |
| `ROOSTY_DEBUG` | `false` | Verbose logs |

Values set through environment variables are shown locked in the admin panel.

Put Roosty behind a reverse proxy with HTTPS in production (Traefik, Caddy or Nginx). Session cookies are marked `Secure` when the request arrives over HTTPS or with `X-Forwarded-Proto: https`.

## Tests

```sh
make test
```
