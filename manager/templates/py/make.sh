#!/usr/bin/env bash
# ./make.sh        check + run main.py   (args go to the program)
# ./make.sh build  check syntax + imports only (shared venv: $AN_TOOLS/py/venv)
# ./make.sh clean  remove __pycache__
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
MAIN=main.py
SRC=.

case "${1:-}" in
  build) exec "$AN_PY" "$AN_TOOLS/py/check.py" $SRC ;;
  clean) find . -name __pycache__ -type d -prune -exec rm -r {} +; exit ;;
esac
"$AN_PY" "$AN_TOOLS/py/check.py" $SRC
exec "$AN_PY" "$MAIN" "$@"
