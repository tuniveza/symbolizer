#!/usr/bin/env bash
# ./make.sh [args] = build if changed + run | build | test | clean | release
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
BIN=bin/__NAME__
case "${1:-}" in
  clean)   rm -rf bin; exit ;;
  test)    exec crystal spec ;;
  release) shards install; crystal build src/__NAME__.cr -o "$BIN" --release; exit ;;
  build)   shift; RUN=0 ;;
  *)       RUN=1 ;;
esac
shards check >/dev/null 2>&1 || shards install
mkdir -p bin
# raylib comes from manager (pkg-config raylib -> ~/an/st/tools/c/lib/libraylib.a)
if [ ! -x "$BIN" ] || [ -n "$(find src lib shard.yml -newer "$BIN" -print -quit)" ]; then
  crystal build src/__NAME__.cr -o "$BIN" --debug
fi
[ "$RUN" = 1 ] && exec ./"$BIN" "$@"
true
