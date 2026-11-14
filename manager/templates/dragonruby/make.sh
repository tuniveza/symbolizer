#!/usr/bin/env bash
# ./make.sh = run (DragonRuby has no compile step) | build = sanity check | clean | publish
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"
case "${1:-}" in
  build)   [ -f app/main.rb ] && [ -x dragonruby ] && echo ok; exit ;;
  clean)   rm -rf logs tmp builds; exit ;;
  publish) shift; exec ./dragonruby-publish --only-package "$@" ;;
esac
exec ./dragonruby "$@"
