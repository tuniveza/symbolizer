# manager with no arguments: an interactive menu over everything manager can do

declare -A LANG_ABOUT=(
  [c]="C — SDL3 + OpenGL window"         [cpp]="C++ — SDL3 + OpenGL + Dear ImGui"
  [raylib]="C — raylib window"            [py]="Python — pygame-ce game loop"
  [py-cli]="Python — command-line script" [ts]="TypeScript — Vite canvas game"
  [web]="plain HTML/JS page"              [node]="TypeScript script (bun)"
  [love]="Lua — LÖVE game"                [flixel]="Haxe — HaxeFlixel game"
  [monogame]="C# — MonoGame"              [java]="Java — plain javac"
  [lwjgl]="Java — LWJGL (gradle)"         [odin]="Odin — raylib window"
  [crystal]="Crystal — raylib"            [godot]="Godot 4 — 2D scene"
  [dragonruby]="Ruby — DragonRuby game"   [go]="Go"
  [gleam]="Gleam"                         [v]="V"
)

# menu lines are "name  ·  path"; the path is ~/an-relative (or ~/...)
_menu_path() { local p="${1#$AN/}"; echo "${p/#$HOME/\~}"; }
_menu_abs() { local p="${1##*  ·  }"; p="${p/#\~/$HOME}"; abspath "$p"; }

_pause() { [ -t 0 ] && { echo; read -r -s -n1 -p "${c_d}press any key for the menu${c_0}"; echo; }; }

_project_menu() { # _project_menu <dir>
  local d="$1" a
  a="$(printf '%s\n' "open in $(mgr_editor_name "$d")" "run" "watch (rebuild on save)" "build" "clean" "show in file manager" | pick "$(mgr_name "$d") ($(short "$d"))")" || return 0
  case "$a" in run|watch*|build|clean) mgr_recent_add "$d" ;; esac
  case "$a" in
    run)   cd "$d" && exec ./make.sh ;;
    watch*) cd "$d" && [ -x reload.sh ] && exec ./reload.sh ;;
    build) (cd "$d" && ./make.sh build) && ok built || bad "build failed"; _pause ;;
    clean) (cd "$d" && ./make.sh clean) && ok cleaned; _pause ;;
    open*) mgr_open "$d"; sleep 1 ;;
    show*) setsid -f xdg-open "$d" >/dev/null 2>&1 ;;
  esac
}

_new_menu() {
  local lang name where dest a
  lang="$(for t in $(ls "$AN_MANAGER/templates"); do printf '%-11s %s\n' "$t" "${LANG_ABOUT[$t]:-}"; done | pick "language" | awk '{print $1}')" || return 0
  [ -n "$lang" ] || return 0
  where="$PWD"
  read -r -e -p "folder [$(short "$where")]: " -i "" a; [ -n "${a:-}" ] && where="$(readlink -m "${a/#\~/$HOME}")"
  read -r -p "name (empty = $(basename "$where") itself): " name
  dest="$(mgr_new_dest "$AN_MANAGER/templates/$lang" "$name" "$where")"
  "$AN_MANAGER/bin/manager" new "$lang" "$name" "$where" || { _pause; return; }
  confirm "build + run it now?" && cd "$dest" && exec ./make.sh
  echo "  cd $(short "$dest")"; _pause
}

mgr_menu() {
  local here="" r choice
  r="$PWD"; while [ "$r" != / ]; do [ -x "$r/make.sh" ] && { here="$r"; break; }; r="$(dirname "$r")"; done
  while true; do
    clear 2>/dev/null
    printf '%slb%s — %s projects · %s toolchains ready\n\n' "$c_b" "$c_0" "$(mgr_projects | wc -l)" \
      "$(n=0; for c in "${ALL_COMPONENTS[@]}"; do "have_$c" 2>/dev/null && n=$((n+1)); done; echo "$n/${#ALL_COMPONENTS[@]}")"
    choice="$( {
      [ -n "$here" ] && printf '%s\n' "run   $(short "$here")" "watch $(short "$here")" "open  $(short "$here") in $(mgr_editor_name "$here")"
      printf '%s\n' \
        "gui                  every project with pictures + videos, in a window" \
        "open…                open a project in Godot / Construct / GDevelop / VSCodium" \
        "projects…            pick a project to run / watch / build" \
        "new project…         start from a template" \
        "check                build every project, report failures" \
        "adopt                give new folders make.sh + reload.sh" \
        "tidy                 delete junk + leftover venvs/__MACOSX" \
        "dupes                find unpacked archives, (1) copies, identical files" \
        "setup                install / repair toolchains + libraries" \
        "status               what's installed" \
        "names                what the two-letter folders stand for" \
        "help" \
        "quit"
    } | pick "manager")" || return 0
    case "$choice" in
      run\ *)   mgr_recent_add "$here"; cd "$here" && exec ./make.sh ;;
      watch\ *) mgr_recent_add "$here"; cd "$here" && exec ./reload.sh ;;
      open\ \ *) mgr_open "$here"; sleep 1 ;;
      gui*)     setsid -f "$AN_PY" "$AN_MANAGER/gui/server.py" >/dev/null 2>&1; sleep 1 ;;
      open…*)   local o; o="$(mgr_openables | while IFS= read -r d; do printf '%-14s %-32s  ·  %s\n' "$(mgr_editor_name "$d")" "$(mgr_name "$d")" "$(_menu_path "$d")"; done | pick "open")" \
                  && [ -n "$o" ] && { mgr_open "$(_menu_abs "$o")"; sleep 1; } ;;
      projects*) local p; p="$(mgr_projects | mgr_by_recent | while IFS= read -r d; do printf '%-32s  ·  %s\n' "$(mgr_name "$d")" "$(_menu_path "$d")"; done | pick "project")" \
                  && [ -n "$p" ] && _project_menu "$(_menu_abs "$p")" ;;
      names*)   mgr_names; _pause ;;
      new*)     _new_menu ;;
      check*)   mgr_check; _pause ;;
      adopt*)   mgr_adopt; _pause ;;
      tidy*)    mgr_tidy; _pause ;;
      dupes*)   mgr_dupes; _pause ;;
      setup*)   mgr_setup; _pause ;;
      status*)  mgr_status; _pause ;;
      help)     mgr_help; _pause ;;
      *)        return 0 ;;
    esac
  done
}
