#!/usr/bin/env bash
# ./make.sh [args] = build if changed + run | build | clean | release
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
BIN=__NAME__
case "${1:-}" in
  clean)   rm -f "$BIN"; exit ;;
  release) odin build . -out:"$BIN" -o:speed; exit ;;
  build)   shift; RUN=0 ;;
  *)       RUN=1 ;;
esac
# odin compiles the whole package at once; skip when nothing is newer than the binary
if [ ! -x "$BIN" ] || [ -n "$(find . -name '*.odin' -newer "$BIN" -print -quit)" ]; then
  odin build . -out:"$BIN" -debug -vet-unused-imports
fi
[ "$RUN" = 1 ] && exec ./"$BIN" "$@"
true
