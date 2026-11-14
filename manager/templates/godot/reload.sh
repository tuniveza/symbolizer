#!/usr/bin/env bash
# replays the game whenever a script, scene or shader is saved
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
exec watchexec -r -e gd,tscn,tres,gdshader -i '.godot/**' -- ./make.sh "$@"
