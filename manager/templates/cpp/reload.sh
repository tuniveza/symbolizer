#!/usr/bin/env bash
# rebuild + restart on every save
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
exec watchexec -r -c -e c,h,cpp,hpp,cc -i 'build/**' -- ./make.sh "$@"
