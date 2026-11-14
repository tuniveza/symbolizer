#!/usr/bin/env bash
# MonoGame: ./make.sh [build|clean|args...]
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"

case "$1" in
  clean) rm -rf bin obj Content/bin Content/obj ;;
  build) dotnet tool restore >/dev/null && dotnet build -nologo -v q ;;
  *)     dotnet tool restore >/dev/null && dotnet run "$@" ;;
esac
