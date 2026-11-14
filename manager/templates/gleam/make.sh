#!/usr/bin/env bash
# ./make.sh [args] = build if changed + run | build | test | clean
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
# gleam.toml defaults to Erlang; use the JS target (node) until Erlang is installed
command -v erl >/dev/null || export GLEAM_TARGET="${GLEAM_TARGET:-javascript}"
T=(); [ -n "${GLEAM_TARGET:-}" ] && T=(--target "$GLEAM_TARGET")
case "${1:-}" in
  clean) exec gleam clean ;;
  test)  exec gleam test "${T[@]}" ;;
  build) exec gleam build "${T[@]}" ;;   # gleam only recompiles changed modules
esac
exec gleam run "${T[@]}" -- "$@"
