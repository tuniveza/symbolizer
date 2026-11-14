#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
exec watchexec -r -e java,kts,vert,frag,glsl -i '**/build/**' -i 'lib/bin/**' -- ./make.sh "$@"
