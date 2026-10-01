#!/bin/sh
# Performance budget from docs/ARCHITECTURE.md: initial JS+CSS ≤ 120 KB gzip.
# Optional: image size ≤ 25 MB when an image name is given.
set -eu
cd "$(git rev-parse --show-toplevel)"
budget=$((120 * 1024))
total=0
for f in web/dist/assets/*.js web/dist/assets/*.css; do
  [ -f "$f" ] || continue
  n=$(gzip -9 -c "$f" | wc -c | tr -d ' ')
  total=$((total + n))
  printf '  %-45s %6d B gzip\n' "$(basename "$f")" "$n"
done
printf 'JS+CSS total: %d B gzip (budget %d B)\n' "$total" "$budget"
[ "$total" -le "$budget" ] || { echo "✗ web bundle over budget"; exit 1; }
if [ "${1:-}" != "" ]; then
  size=$(docker image inspect "$1" --format '{{.Size}}')
  printf 'Image %s: %d MB (budget 25 MB)\n' "$1" $((size / 1024 / 1024))
  [ "$size" -le $((25 * 1024 * 1024)) ] || { echo "✗ image over budget"; exit 1; }
fi
echo "✓ size budget ok"
