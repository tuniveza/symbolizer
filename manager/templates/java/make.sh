#!/usr/bin/env bash
# plain Java (no gradle): sources in src/, classes in build/
#   ./make.sh [build|clean|args...]    MAIN=pkg.Class to change the entry point
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
MAIN="${MAIN:-Main}"

build() {
  # recompile everything when any source is newer than the last build (javac is fast; avoids stale classes)
  if [ ! -f build/.stamp ] || [ -n "$(find src -name '*.java' -newer build/.stamp -print -quit)" ]; then
    rm -rf build && mkdir -p build
    find src -name '*.java' -print0 | xargs -0 javac -d build
    [ -d res ] && cp -r res/. build/
    touch build/.stamp
  fi
}

case "$1" in
  clean) rm -rf build ;;
  build) build ;;
  *)     build && java -cp build "$MAIN" "$@" ;;
esac
