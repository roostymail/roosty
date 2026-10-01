#!/bin/sh
# Waits for GreenMail and delivers the sample messages once.
send() { curl -sS --crlf smtp://greenmail:3025 --mail-from seed@roosty.test --mail-rcpt marina@roosty.test --upload-file "$1"; }
first=$(ls /seed/*.eml | head -1)
until send "$first" 2>/dev/null; do sleep 2; done
echo "sent $(basename "$first")"
for f in $(ls /seed/*.eml | tail -n +2); do send "$f" && echo "sent $(basename "$f")"; done
