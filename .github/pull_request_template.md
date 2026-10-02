## What changes

<!-- One or two sentences. Link the issue if there is one. -->

## How it was tested

- [ ] `make test` (unit)
- [ ] `make itest` (integration) when IMAP/SMTP behavior changes
- [ ] `make e2e` (browser) when the interface or an API used by it changes
- [ ] New or changed behavior has a test; a bug fix has a regression test

## Security checklist

- [ ] Input from email content, headers, requests or files is validated or escaped
- [ ] No secrets, personal data or infrastructure details in code, logs or fixtures
- [ ] Attack cases (negative tests) are covered when touching auth, sanitizing, files or network
- [ ] New dependencies are justified and maintained

## Screenshots

<!-- For interface changes: before and after, light and dark theme. -->
