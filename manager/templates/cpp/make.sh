#!/usr/bin/env bash
# build (only what changed) and run:   ./make.sh [args]
#   ./make.sh build [release|asan]   build only       ./make.sh clean
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
case "$1" in
  build) shift; make "$@" ;;
  clean) make clean ;;
  *)     make && exec ./__NAME__ "$@" ;;
esac
