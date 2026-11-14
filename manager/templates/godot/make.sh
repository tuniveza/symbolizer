#!/usr/bin/env bash
# ./make.sh [args] = check + play | build (import + script check, no window) | edit | clean
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
case "${1:-}" in
  build) exec godot-project build ;;
  edit)  shift; exec godot-project edit "$@" ;;
  clean) rm -rf .godot/imported .godot/shader_cache; exit ;;   # re-imported on next build
esac
godot-project build
exec godot-project run "$@"
