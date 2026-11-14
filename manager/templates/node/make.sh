#!/usr/bin/env bash
# bun + TypeScript script. ./make.sh [args] runs it, ./make.sh build typechecks, ./make.sh test
# ./make.sh exe   compiles a standalone binary to ./__NAME__
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
. "$AN_TOOLS/js/lib.sh"
case "${1:-}" in
  build) js_deps; js_run check ;;
  test)  shift; js_deps; bun test "$@" ;;
  exe)   js_deps; bun build src/index.ts --compile --outfile __NAME__ ;;
  clean) rm -f __NAME__; [ "${2:-}" = all ] && rm -rf node_modules; true ;;
  *)     js_deps; exec bun src/index.ts "$@" ;;
esac
