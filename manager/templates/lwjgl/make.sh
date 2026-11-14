#!/usr/bin/env bash
# Java + LWJGL (gradle): ./make.sh [build|clean|args...]
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"

case "$1" in
  clean) ./gradlew -q clean ;;
  build) ./gradlew -q installDist ;;
  *)     ./gradlew -q installDist && lib/build/install/lib/bin/lib "$@" ;;
esac
