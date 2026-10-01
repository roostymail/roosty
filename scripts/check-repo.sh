#!/bin/sh
# Blocks commits that carry things that must never reach the public repository
# or a release. Runs as a pre-commit hook (make hooks) and in CI.
#
#   scripts/check-repo.sh            check staged changes (pre-commit)
#   scripts/check-repo.sh --all      check every tracked file (CI)
#
# Personal or infrastructure terms (your name, server IPs, private domains)
# go in .git/info/forbidden-patterns, one regex per line. That file stays on
# your machine and is never committed.
set -eu
cd "$(git rev-parse --show-toplevel)"

if [ "${1:-}" = "--all" ]; then
  files=$(git ls-files)
  content() { cat -- "$1"; }
else
  files=$(git diff --cached --name-only --diff-filter=ACMR)
  content() { git show ":$1"; }
fi
[ -z "$files" ] && exit 0

fail=0
report() { echo "✗ $1"; fail=1; }

for f in $files; do
  case "$f" in
    *.env|*.env.*|.env*|*.pem|*.key|*.p12|*.pfx|id_rsa*|*.db|*.sqlite|*.sqlite3) report "$f: arquivo sensível não pode ser versionado" ;;
    */node_modules/*|node_modules/*|web/dist/*|server/data/*|*/.DS_Store|.DS_Store) report "$f: arquivo gerado ou local" ;;
  esac
  [ -f "$f" ] || continue
  size=$(wc -c < "$f" | tr -d ' ')
  case "$f" in web/package-lock.json|LICENSE) ;; *)
    [ "$size" -gt 1048576 ] && report "$f: maior que 1 MB ($size bytes)" ;;
  esac
done

# Secrets and debug leftovers in the content being committed.
secrets='-----BEGIN [A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{40,}|xox[baprs]-[A-Za-z0-9-]{10,}|sk_live_[A-Za-z0-9]{20,}|(api[_-]?key|secret|passwd|password)["'"'"' ]*[:=]["'"'"' ]*[A-Za-z0-9/+_-]{24,}'
leftovers='DO NOT COMMIT|NAO COMMITAR|NÃO COMMITAR|(describe|it|test)\.only\(|debugger;'
local_patterns=".git/info/forbidden-patterns"

for f in $files; do
  [ -f "$f" ] || continue
  case "$f" in scripts/check-repo.sh|*.sum|web/package-lock.json) continue ;; esac
  body=$(content "$f" 2>/dev/null) || continue
  echo "$body" | grep -nEI -e "$secrets" >/dev/null 2>&1 && report "$f: parece conter um segredo"
  echo "$body" | grep -nEI -e "$leftovers" >/dev/null 2>&1 && report "$f: marcador de debug ou 'não commitar'"
  # Invisible bidi control characters in source ("Trojan Source", CVE-2021-42574).
  if echo "$body" | perl -CSD -ne 'exit 1 if /[\x{202A}-\x{202E}\x{2066}-\x{2069}]/' 2>/dev/null; then :; else report "$f: contém caractere invisível de inversão de texto (use o escape \\u202E)"; fi
  if [ -f "$local_patterns" ]; then
    grep -vE '^\s*(#|$)' "$local_patterns" | while IFS= read -r p; do
      if echo "$body" | grep -iqE -- "$p"; then echo "✗ $f: contém termo privado da lista local"; echo x > .git/check-repo-fail; fi
    done
  fi
done
[ -f .git/check-repo-fail ] && { rm -f .git/check-repo-fail; fail=1; }

if [ "$fail" -ne 0 ]; then
  echo "Commit bloqueado. Corrija os itens acima (ou, se for falso positivo, ajuste scripts/check-repo.sh)."
  exit 1
fi
echo "✓ check-repo: nada sensível encontrado"
