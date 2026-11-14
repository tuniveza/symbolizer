#!/usr/bin/env bash
# vite dev server: hot-reloads in the browser on every save
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
. "$AN_TOOLS/js/lib.sh"
js_deps
js_x vite "$@"
