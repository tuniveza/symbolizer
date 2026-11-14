#!/usr/bin/env bash
# LÖVE: ./make.sh [build|clean|args...]
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"

check() {  # LÖVE runs LuaJIT, so syntax-check with it
  find . -name '*.lua' -not -path './.*' -print0 | while IFS= read -r -d '' f; do
    luajit -b "$f" /dev/null || exit 1
  done
}

case "$1" in
  clean) ;;  # nothing is built
  build) check ;;
  *)     check && love . "$@" ;;
esac
