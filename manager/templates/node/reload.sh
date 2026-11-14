#!/usr/bin/env bash
# rerun on every save
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
exec bun --watch src/index.ts "$@"
