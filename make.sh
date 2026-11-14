#!/usr/bin/env bash
# ./make.sh [args] = build (go caches, so only changed packages recompile) + run | build | test | clean
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
BIN=sy
case "${1:-}" in
  clean) go clean -testcache; exit ;;  # keeps ./sy: ~/.local/bin/sy points at it
  test)  shift; exec go test ./... "$@" ;;
  build) go vet ./... && go build -o "$BIN" .; exit ;;
esac
go build -o "$BIN" .
exec ./"$BIN" "$@"
