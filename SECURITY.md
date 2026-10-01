# Security policy

A webmail has access to everything in a person's mailbox, so we treat security reports as the highest priority.

## Supported versions

Roosty Mail has no releases yet. Once releases start, the latest minor version will receive security fixes.

## Reporting a vulnerability

Report privately through GitHub: open the [Security tab](https://github.com/roostymail/roosty/security) and choose **Report a vulnerability**.

Please include:

- what an attacker can do and under which conditions
- steps or a proof of concept to reproduce it
- the version or commit you tested

Do not open a public issue, pull request or discussion for a vulnerability.

## What to expect

- We confirm receipt within 3 working days.
- We share an assessment and a planned fix date within 10 working days.
- We publish a GitHub Security Advisory, request a CVE when appropriate, and credit you unless you prefer to stay anonymous.

## Scope

Examples of issues we especially want to hear about:

- cross-site scripting through email content, headers or attachments
- server-side request forgery through the image proxy
- session theft, CSRF or authentication bypass
- leaking a user's IP address or read status to a sender when remote content is blocked
