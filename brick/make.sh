#!/usr/bin/env bash
# ./make.sh        check + run brick.py   (args go to the program, e.g. ./make.sh gui)
# ./make.sh build  check syntax + imports only (shared venv: $AN_TOOLS/py/venv)
# ./make.sh clean  remove __pycache__
# ./make.sh engine build the imagination engine in Go (trainer/imagination/engine/imagine; without it brick uses node)
set -e
cd "$(dirname "$0")"
export PYTHONPATH="$PWD${PYTHONPATH:+:$PYTHONPATH}"   # trainer/ imports brick's own modules from here
. "$HOME/an/st/tools/env.sh"
MAIN=brick.py
SRC="*.py trainer/*.py"   # tests/ and old/ find their imports at run time

engine() {   # the Go engine: same answers as node run.js, much faster (needs go; brick falls back to node without it)
  command -v go >/dev/null || return 0
  (cd trainer/imagination/engine && go build -o imagine ./cmd/imagine && touch imagine) || echo "engine: go build failed; brick will use node" >&2
}

case "${1:-}" in
  engine) engine; exit ;;
  build) engine; exec "$AN_PY" "$AN_TOOLS/py/check.py" $SRC ;;
  clean) find . -name __pycache__ -type d -prune -exec rm -r {} +; exit ;;
esac
engine
"$AN_PY" "$AN_TOOLS/py/check.py" $SRC
exec "$AN_PY" "$MAIN" "$@"
