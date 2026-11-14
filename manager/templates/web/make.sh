#!/usr/bin/env bash
# plain html/js — nothing to compile.
# ./make.sh          serve on http://localhost:${PORT:-5173}
# ./make.sh build    check that every local src/href in the html exists
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
. "$AN_TOOLS/js/lib.sh"
case "${1:-}" in
  build) js_check_static index.html ;;
  clean) true ;;
  *)     js_serve . ;;
esac
