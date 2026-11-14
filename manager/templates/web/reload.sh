#!/usr/bin/env bash
# serve, and restart the server on file changes (refresh the browser to see them)
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
exec watchexec -r -e html,css,js,json -- ./make.sh
