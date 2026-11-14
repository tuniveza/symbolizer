#!/usr/bin/env bash
# HaxeFlixel (lime): ./make.sh [build|clean|args...]
#   TARGET=neko ./make.sh   -> fast-compiling neko build instead of native linux (hxcpp)
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
TARGET="${TARGET:-linux}"

# build.hxml is only for editor completion / on-save type checks; keep it in sync with Project.xml
if [ ! -f build.hxml ] || [ Project.xml -nt build.hxml ]; then
  { lime display neko; echo "--no-output"; } > build.hxml
fi

case "$1" in
  clean) rm -rf export ;;
  build) lime build "$TARGET" ;;
  *)     lime test "$TARGET" "$@" ;;
esac
