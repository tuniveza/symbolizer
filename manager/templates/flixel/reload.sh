#!/usr/bin/env bash
# rebuild + restart on save (neko target: compiles fastest)
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
export TARGET="${TARGET:-neko}"
exec watchexec -r -e hx,xml -w source -w assets -w Project.xml -- ./make.sh "$@"
