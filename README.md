<h1 align="center">
  <img src="docs/assets/logo-horizontal.png" width="420" alt="Roosty Mail">
</h1>

<p align="center">
  A lightweight, modern webmail you host yourself.<br>
  <a href="https://roosty.dev">roosty.dev</a>
</p>

> [!WARNING]
> Roosty Mail is in early development. The first working version runs locally with Docker, but it is not ready for production mail yet. See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) to try it.

## What it is

Roosty Mail is an open-source webmail for any IMAP/SMTP server. It works like a thin, fast layer on top of your existing mail server: your server stays the source of truth, and Roosty gives you a modern interface to read, write and organize mail.

It is built for people who run their own mail or use providers such as MXroute, Dovecot-based hosts or Stalwart, and want something lighter and nicer than the webmails available today.

## Goals

- **Light.** One small container for amd64 and arm64. Under a second to the inbox, around 120 KB of JS and CSS on first load.
- **Safe by default.** Email HTML is sanitized twice and rendered in an isolated frame. Remote images go through a signed proxy. No telemetry.
- **Yours.** Ready-made light and dark themes with your own accent color, plus a theme editor for custom themes.
- **Simple to self-host.** Configuration through environment variables, SQLite for settings, no extra services to run.
- **Ready for JMAP.** The internal API follows the JMAP model, so a native JMAP backend can be added later.

## Planned stack

| Layer | Choice |
|---|---|
| Backend | Go: IMAP/SMTP proxy, MIME parsing, sanitizer, image proxy (Bun + imapflow is being evaluated) |
| Frontend | SolidJS SPA, virtualized lists, IndexedDB cache |
| Storage | SQLite (settings, themes, optional header cache) |
| Real time | IMAP IDLE on the server, Server-Sent Events to the browser |

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design and the trade-offs behind each choice.

## Try it locally

```sh
git clone https://github.com/roostymail/roosty && cd roosty
docker compose up --build -d
```

Then open http://localhost:8080/admin/setup (setup code `roosty-local-setup`) and sign in to the webmail with `marina@roosty.test` / `roosty123`. Details in [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Roadmap

1. **Foundations.** Repository, CI, dev environment with Dovecot, Stalwart and GreenMail.
2. **MVP.** Folders, message list, safe reading, compose with attachments and drafts, push updates, built-in themes.
3. **v1.** Conversations, search, keyboard shortcuts, identities, undo send, theme editor, offline reading, translations.
4. **Later.** Contacts (CardDAV), filters (ManageSieve), PGP, OAuth for Gmail and Microsoft 365, plugins.

## Get involved

The project is just starting. Ideas, questions and feedback are welcome in [issues](https://github.com/roostymail/roosty/issues). Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

To report a security problem, follow [SECURITY.md](SECURITY.md). Do not open a public issue.

## Packages

| Registry | Name |
|---|---|
| GitHub Container Registry | `ghcr.io/roostymail/roosty` |
| Docker Hub | `roostymail/roosty` |
| npm | `roosty` |

## License

Roosty Mail is licensed under the [GNU Affero General Public License v3.0](LICENSE). If you run a modified version as a service for others, you must make your changes available to its users.
