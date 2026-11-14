# manager open — open a project in the editor it belongs in
#   Godot -> newest Godot · Construct 3 -> editor.construct.net · GDevelop -> GDevelop
#   Android -> Android Studio · anything else -> VSCodium
# Override per machine in manager.conf: CODE_EDITOR, ANDROID_STUDIO, GDEVELOP, GAMEMAKER

CODE_EDITOR="${CODE_EDITOR:-codium}"
ANDROID_STUDIO="${ANDROID_STUDIO:-/opt/android-studio/bin/studio}"
GDEVELOP="${GDEVELOP:-$HOME/AppImages/gdevelop_5.appimage}"
GAMEMAKER="${GAMEMAKER:-$(command -v GameMaker || command -v gamemaker || true)}"

_detach() { # start a GUI app detached from this terminal (MGR_OPEN_DRY=1: just print it)
  if [ -n "${MGR_OPEN_DRY:-}" ]; then printf '  would run: %q' "$1"; shift; printf ' %q' "$@"; echo; return; fi
  setsid -f "$@" >/dev/null 2>&1 </dev/null
}
_note() { echo "  $*"; [ -z "${MGR_OPEN_DRY:-}" ] && command -v notify-send >/dev/null && notify-send -a manager "manager open" "$*" 2>/dev/null || true; }

# Opening a project with `godot -e --path` skips Godot's Project Manager, so it never gets added
# to the Project Manager's list. Add it ourselves (only while no Godot is running — Godot rewrites
# this file when it exits).
godot_register() {
  local cfg="$HOME/.local/share/godot/projects.cfg" p="${1%/}"
  [ -n "${MGR_OPEN_DRY:-}" ] && return 0
  grep -qsF "[$p]" "$cfg" && return 0
  pgrep -f 'tc/godot/godot' >/dev/null && return 0
  mkdir -p "$(dirname "$cfg")"
  printf '[%s]\n\nfavorite=false\n\n' "$p" >> "$cfg"
}

# engine project file in a folder (or one level down), echoed as "kind<TAB>path"
mgr_engine_of() {
  local d="$1" f
  [ -f "$d" ] && d="$(dirname "$d")"
  f="$(find "$d" -maxdepth 2 -name project.godot -not -path '*/.godot/*' -print -quit 2>/dev/null)"
  [ -n "$f" ] && { printf 'godot\t%s\n' "$f"; return; }
  f="$(find "$d" -maxdepth 2 \( -name '*.c3proj' -o -name '*.c3p' \) -print -quit 2>/dev/null)"
  [ -n "$f" ] && { printf 'construct\t%s\n' "$f"; return; }
  f="$(grep -lsm1 '"gdVersion"' "$d"/*.json "$d"/*/*.json 2>/dev/null | head -1)"
  [ -n "$f" ] && { printf 'gdevelop\t%s\n' "$f"; return; }
  f="$(find "$d" -maxdepth 2 -name '*.yyp' -print -quit 2>/dev/null)"
  [ -n "$f" ] && { printf 'gamemaker\t%s\n' "$f"; return; }
  is_android "$d" && { printf 'android\t%s\n' "$d"; return; }
  printf 'code\t%s\n' "$d"
}

# what `manager open` would use, in words (for the menu)
mgr_editor_name() {
  case "$(mgr_engine_of "$1" | cut -f1)" in
    godot) echo Godot ;; construct) echo "Construct 3" ;; gdevelop) echo GDevelop ;;
    gamemaker) [ -n "$GAMEMAKER" ] && echo GameMaker || echo VSCodium ;;
    android) [ -x "$ANDROID_STUDIO" ] && echo "Android Studio" || echo VSCodium ;;
    *) echo VSCodium ;;
  esac
}

mgr_open() { # manager open [-n] [dir|file]
  [ "${1:-}" = -n ] && { export MGR_OPEN_DRY=1; shift; }
  local target="${1:-}" kind f d
  if [ -z "$target" ]; then
    # the project you're in, else the current folder
    target="$PWD"; d="$PWD"
    while [ "$d" != / ]; do [ -x "$d/make.sh" ] && { target="$d"; break; }; d="$(dirname "$d")"; done
  fi
  target="$(readlink -f "$target")"; [ -e "$target" ] || die "no such file or folder: $1"
  IFS=$'\t' read -r kind f < <(mgr_engine_of "$target")
  [ -z "${MGR_OPEN_DRY:-}" ] && mgr_recent_add "$target"
  d="$( [ -d "$target" ] && echo "$target" || dirname "$target")"
  case "$kind" in
    godot)
      godot_register "$(dirname "$f")"
      _detach "$AN_TOOLS/bin/godot-project" latest -e --path "$(dirname "$f")"
      echo "  opening $(short "$(dirname "$f")") in $("$AN_TOOLS/bin/godot-project" latest --version 2>/dev/null | cut -d. -f1-3 | sed 's/^/Godot /')" ;;
    construct)
      # Construct 3 lives in the browser and can't be handed a local path, so open the editor
      # and put the project on the clipboard for its folder/file picker
      local what="folder" p; p="$(dirname "$f")"
      case "$f" in *.c3p) what="file"; p="$f" ;; esac
      [ -z "${MGR_OPEN_DRY:-}" ] && command -v wl-copy >/dev/null && printf '%s' "$p" | wl-copy
      _detach xdg-open "https://editor.construct.net/"
      _note "Construct 3 opened. Menu > Project > Open > Local project $what, then Ctrl+L and Ctrl+V — the path is on your clipboard: $(short "$p")" ;;
    gdevelop)
      [ -x "$GDEVELOP" ] || die "GDevelop not found at $GDEVELOP (set GDEVELOP in manager.conf)"
      _detach env DESKTOPINTEGRATION=1 "$GDEVELOP" --no-sandbox "$f"
      echo "  opening $(short "$f") in GDevelop" ;;
    gamemaker)
      if [ -n "$GAMEMAKER" ]; then _detach "$GAMEMAKER" "$f"; echo "  opening $(short "$f") in GameMaker"
      else _detach "$CODE_EDITOR" "$d"; _note "GameMaker isn't installed — opened $(short "$d") in VSCodium"; fi ;;
    android)
      if [ -x "$ANDROID_STUDIO" ]; then _detach "$ANDROID_STUDIO" "$d"; echo "  opening $(short "$d") in Android Studio"
      else _detach "$CODE_EDITOR" "$d"; echo "  opening $(short "$d") in VSCodium"; fi ;;
    *)
      command -v "$CODE_EDITOR" >/dev/null || die "$CODE_EDITOR not found (set CODE_EDITOR in manager.conf)"
      _detach "$CODE_EDITOR" "$target"
      echo "  opening $(short "$target") in VSCodium" ;;
  esac
}

# recently opened / run projects, newest first (kept in $AN_MANAGER/.state/recent)
mgr_recent_add() {
  local f="$AN_MANAGER/.state/recent" p="${1%/}"
  mkdir -p "$AN_MANAGER/.state"
  { grep -vxF "$p" "$f" 2>/dev/null; echo "$p"; } | tail -30 > "$f.tmp" && mv "$f.tmp" "$f"
}
# mgr_by_recent: reorder stdin (paths) so recently used ones come first
mgr_by_recent() {
  local all; all="$(cat)"
  { [ -f "$AN_MANAGER/.state/recent" ] && tac "$AN_MANAGER/.state/recent" | grep -xF -f <(printf '%s\n' "$all"); printf '%s\n' "$all"; } \
    | awk 'NF && !seen[$0]++'
}

# everything worth opening: projects with make.sh + engine projects anywhere in ROOTS
mgr_openables() {
  local pe
  mapfile -t pe < <(prune_expr)
  {
    mgr_projects
    { find "${ROOTS[@]}" -path "$AN_TOOLS" -prune -o "${pe[@]}" -o -type f \( -name project.godot -o -name '*.c3proj' -o -name '*.c3p' -o -name '*.yyp' \) -printf '%h\n' 2>/dev/null
      grep -rlsm1 --include='*.json' --exclude-dir=node_modules --exclude-dir=.git '"gdVersion"' "${ROOTS[@]}" 2>/dev/null | xargs -r -d '\n' -n1 dirname
    } | while IFS= read -r d; do is_ignored "$d" || echo "$d"; done
  } | sort -u | mgr_by_recent
}
