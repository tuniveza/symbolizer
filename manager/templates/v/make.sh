#!/usr/bin/env bash
# ./make.sh [args] = build if changed + run | build | clean | release
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
BIN=__NAME__
case "${1:-}" in
  clean)   rm -f "$BIN"; exit ;;
  release) v -cc gcc -prod -o "$BIN" .; exit ;;
  build)   shift; RUN=0 ;;
  *)       RUN=1 ;;
esac
if [ ! -x "$BIN" ] || [ -n "$(find . -name '*.v' -newer "$BIN" -print -quit)" ]; then
  v -cc gcc -g -o "$BIN" .   # bundled tcc cannot link gg atomics on this glibc
fi
[ "$RUN" = 1 ] && exec ./"$BIN" "$@"
true
