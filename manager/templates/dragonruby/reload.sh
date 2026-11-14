#!/usr/bin/env bash
# DragonRuby hot-reloads app/*.rb by itself while running
exec "$(dirname "$0")/make.sh" "$@"
