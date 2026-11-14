#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
exec watchexec -r -e cs,mgcb,csproj -i 'bin/**' -i 'obj/**' -- ./make.sh "$@"
