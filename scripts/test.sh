#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Keep test output stable in Pi/Git Bash/Windows terminals.
export CI=1
export NO_COLOR=1
export FORCE_COLOR=0
export TERM=${TERM:-xterm-256color}

# Windows Git Bash may not inherit the Go installer PATH immediately.
if [ -x "/c/Program Files/Go/bin/go.exe" ]; then
  export PATH="/c/Program Files/Go/bin:$PATH"
fi

if [ -f tests/.env ]; then
  set -a
  # shellcheck disable=SC1091
  . tests/.env
  set +a
fi

if command -v go >/dev/null 2>&1; then
  go test ./...
else
  echo "go not found; skipping Go tests" >&2
fi

if command -v npm >/dev/null 2>&1; then
  (cd web && npm install && npm test && npm run build)
else
  echo "npm not found; skipping web build" >&2
fi

if grep -R "8765" -n internal cmd api web/src web/vite.config.ts config.example.yaml README.md --exclude-dir=node_modules --exclude-dir=dist >/tmp/uuagent-port-grep.txt 2>/dev/null; then
  echo "Found legacy port 8765 references:" >&2
  cat /tmp/uuagent-port-grep.txt >&2
  exit 1
fi
