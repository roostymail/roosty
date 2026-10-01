# Contributing to Roosty Mail

Thanks for your interest. Roosty Mail is in its earliest stage, so the most useful contributions right now are ideas, feedback on the architecture and reports about how your mail server behaves.

## Before you start

- For anything larger than a small fix, open an issue first so we can agree on the approach.
- Security problems go through [SECURITY.md](SECURITY.md), never public issues.
- Everyone taking part follows the [Code of Conduct](https://github.com/roostymail/.github/blob/main/CODE_OF_CONDUCT.md).

## Pull requests

- Keep each pull request focused on one change.
- Add or update tests for behavior you change.
- Describe what changed and how you tested it.
- Respect the performance budget in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Changes that grow the initial bundle need a reason.

## Workflow

- `main` is always releasable. Nobody pushes to it directly; every change arrives through a pull request with green CI.
- Work happens on short-lived branches (`feat/…`, `fix/…`, `chore/…`). Pull requests are **squash-merged**, so `main` gets one clean commit per change, and work-in-progress commits never reach it.
- Releases are tags on `main` (`v0.1.0`). Images, checksums and signatures are built by CI from the tag only, never from a laptop.
- Run `make hooks` once after cloning. The pre-commit hook (`scripts/check-repo.sh`) blocks secrets, private keys, `.env` and database files, generated folders, debug leftovers and files over 1 MB. Put private terms of your own (name, server IPs, internal domains) in `.git/info/forbidden-patterns`; that file never leaves your machine.

## Sign-off (DCO)

We use the [Developer Certificate of Origin](https://developercertificate.org/) instead of a CLA. Sign off every commit to confirm you have the right to submit it:

```sh
git commit -s -m "Describe your change"
```

This adds a `Signed-off-by: Your Name <you@example.com>` line to the commit.

## License

By contributing, you agree that your contributions are licensed under the [AGPL-3.0](LICENSE), the same license as the project.
