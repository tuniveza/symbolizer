#!/usr/bin/env bash
# ./make.sh [args] = build (go caches, so only changed packages recompile) + run | build | test | clean
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
BIN=__NAME__
case "${1:-}" in
  clean) rm -f "$BIN"; go clean -testcache; exit ;;
  test)  shift; exec go test ./... "$@" ;;
  build) go vet ./... && go build -o "$BIN" .; exit ;;
esac
go build -o "$BIN" .
exec ./"$BIN" "$@"
