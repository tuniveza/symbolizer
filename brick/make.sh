#!/usr/bin/env bash
# ./make.sh        check + run brick.py   (args go to the program, e.g. ./make.sh gui)
# ./make.sh build  check syntax + imports only (shared venv: $AN_TOOLS/py/venv)
# ./make.sh clean  remove __pycache__
set -e
cd "$(dirname "$0")"
export PYTHONPATH="$PWD${PYTHONPATH:+:$PYTHONPATH}"   # trainer/ imports brick's own modules from here
. "$HOME/an/st/tools/env.sh"
MAIN=brick.py
SRC="*.py trainer/*.py"   # tests/ and old/ find their imports at run time

case "${1:-}" in
  build) exec "$AN_PY" "$AN_TOOLS/py/check.py" $SRC ;;
  clean) find . -name __pycache__ -type d -prune -exec rm -r {} +; exit ;;
esac
"$AN_PY" "$AN_TOOLS/py/check.py" $SRC
exec "$AN_PY" "$MAIN" "$@"
