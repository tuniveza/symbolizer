#!/usr/bin/env bash
# ./make.sh            production build (typecheck + vite build) then preview it
# ./make.sh build      build only          ./make.sh clean [all]   (all = also node_modules)
# ./reload.sh          dev server with hot reload
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
. "$AN_TOOLS/js/lib.sh"
case "${1:-}" in
  build) js_deps; js_run build ;;
  clean) rm -rf dist node_modules/.vite node_modules/.tmp ./*.tsbuildinfo; [ "${2:-}" = all ] && rm -rf node_modules; true ;;
  *)     js_deps; js_run build; js_x vite preview "$@" ;;
esac
