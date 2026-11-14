# manager tidy  — remove junk + leftover build/setup garbage
# manager dupes — find redundant copies (unpacked archives, "(1)" copies, identical .bak, identical big files)
# Nothing is touched without asking; anything that isn't pure junk goes to the trash (recoverable).

_list() { # _list <title> <file with "bytes<TAB>path<TAB>note" lines> -> prints it; fails if empty
  local f="$2" n total
  n="$(wc -l < "$f")"; [ "$n" -gt 0 ] || return 1
  total="$(awk -F'\t' '{s+=$1} END{print s+0}' "$f")"
  info "$1: $n item(s), $(human "$total")"
  awk -F'\t' '{printf "%s\t%s%s\n", $1, $2, ($3 != "" ? "   ("$3")" : "")}' "$f" \
    | numfmt --field=1 --to=iec --suffix=B --padding=8 2>/dev/null \
    | sed "s|$HOME/an/||g; s|$HOME|~|g; s|^|  |" | tr '\t' ' '
}
_list_and_confirm() { # -> 0 to proceed
  _list "$1" "$2" || return 1
  [ "${MGR_DRY:-}" = 1 ] && return 1
  confirm "  remove these?"
}

mgr_tidy() { # manager tidy [-n] [--builds] [dirs...]
  local builds=0 dirs=() pe a
  for a in "$@"; do
    case "$a" in -n|--dry-run) MGR_DRY=1 ;; --builds) builds=1 ;; -y|--yes) MGR_YES=1 ;; *) dirs+=("$(readlink -f "$a")") ;; esac
  done
  [ ${#dirs[@]} -gt 0 ] || dirs=("${ROOTS[@]}")
  mapfile -t pe < <(prune_expr)
  local tmp; tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' RETURN

  # 1. pure junk: always regenerable or useless -> deleted
  find "${dirs[@]}" -path "$AN_TOOLS" -prune -o \( -name node_modules -o -name .git \) -prune -o \
       \( -type f \( -name .DS_Store -o -name Thumbs.db -o -name desktop.ini -o -name '*.pyc' -o -name a.out -o -regex '.*/core\.[0-9]+' \) \
          -o -type d -name __pycache__ -prune \) -printf '%p\n' 2>/dev/null \
    | while IFS= read -r p; do printf '%s\t%s\t\n' "$(dsize "$p")" "$p"; done > "$tmp/junk"
  if _list_and_confirm "junk files (.DS_Store, __pycache__, a.out, core dumps)" "$tmp/junk"; then
    cut -f2 "$tmp/junk" | while IFS= read -r p; do rm -rf -- "$p"; done; ok "deleted"
  fi

  # 2. macOS resource-fork folders and stray per-project Python venvs (manager/py/venv replaces them) -> trash
  { find "${dirs[@]}" -path "$AN_TOOLS" -prune -o -name node_modules -prune -o -type d -name __MACOSX -prune -printf '%p\tmacOS junk\n' 2>/dev/null
    find "${dirs[@]}" -path "$AN_TOOLS" -prune -o -name node_modules -prune -o -name .git -prune -o -type f -name pyvenv.cfg -printf '%h\tpython venv (manager uses manager/py/venv)\n' 2>/dev/null
  } | while IFS=$'\t' read -r p why; do
        # a venv whose folder IS the project (bin/ lib/ next to code) — only its venv parts go
        if [ "$why" != "macOS junk" ] && [ -f "$p/make.sh" ]; then
          for v in bin include lib lib64 pyvenv.cfg; do
            [ -e "$p/$v" ] || continue
            # lib/ may hold real code in a project; only take it if it's nothing but python3.x/
            case "$v" in
              lib|lib64|include) [ -z "$(ls -A "$p/$v" | grep -v '^python3')" ] || continue ;;
              bin) [ -f "$p/bin/activate" ] || continue ;;
            esac
            printf '%s\t%s\t%s\n' "$(dsize "$p/$v")" "$p/$v" "$why"
          done
        else
          printf '%s\t%s\t%s\n' "$(dsize "$p")" "$p" "$why"
        fi
      done > "$tmp/trash"
  if _list_and_confirm "leftover folders (moved to trash)" "$tmp/trash"; then
    cut -f2 "$tmp/trash" | while IFS= read -r p; do trash "$p"; done; ok "trashed"
  fi

  # 3. optional: every project's own build output (./make.sh clean)
  if [ $builds = 1 ]; then
    mgr_projects "${dirs[@]}" | while IFS= read -r d; do
      [ -f "$d/.skip-check" ] && continue
      (cd "$d" && ./make.sh clean >/dev/null 2>&1) && ok "cleaned $(short "$d")" || warn "clean failed in $(short "$d")"
    done
  fi
  [ "${MGR_DRY:-}" = 1 ] && dim "(dry run — nothing was changed)"
  true
}

mgr_dupes() { # manager dupes [-n] [--min MB] [dirs...]
  local dirs=() a
  while [ $# -gt 0 ]; do
    case "$1" in -n|--dry-run) MGR_DRY=1 ;; -y|--yes) MGR_YES=1 ;; --min) export MGR_DUPES_MIN_MB="$2"; shift ;;
      *) dirs+=("$(readlink -f "$1")") ;; esac
    shift
  done
  [ ${#dirs[@]} -gt 0 ] || dirs=("$AN" "$HOME/Downloads" "$HOME/Documents" "$HOME/Desktop")
  info "scanning $(for d in "${dirs[@]}"; do printf '%s ' "$(short "$d")"; done)for duplicates..."
  local tmp; tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' RETURN
  MGR_DUPES_NO_SAME="$(IFS=:; echo "${IGNORE[*]}")" \
  MGR_DUPES_SKIP="$AN_TOOLS/tc:$AN_TOOLS/c/repos:$AN_TOOLS/py/venv:$AN_TOOLS/hx/haxelib" python3 -I "$AN_MANAGER/lib/dupes.py" "${dirs[@]}" > "$tmp/all"
  local kind title
  for kind in extracted copy same; do
    case $kind in
      extracted) title="archives that are already unpacked" ;;
      copy)      title='"(1)" copies and .bak files identical to the original' ;;
      same)      title="big files with an identical copy elsewhere (oldest copy is kept)" ;;
    esac
    awk -F'\t' -v k=$kind '$1==k {print $2"\t"$3"\t"$4}' "$tmp/all" > "$tmp/$kind"
    [ -s "$tmp/$kind" ] || continue
    if [ $kind = same ] && [ "${MGR_DRY:-}" != 1 ] && [ -t 0 ]; then
      # identical big files can be intentional (assets per project) — pick which ones go
      _list "$title" "$tmp/$kind"
      info "pick the ones to trash (tab = select, enter = done, esc = none)"
      cut -f2,3 "$tmp/$kind" | sed "s|$HOME/an/||g; s|$HOME|~|g" | pick "trash" multi > "$tmp/picked" || true
      while IFS=$'\t' read -r p _; do
        p="${p/#\~/$HOME}"; [ -e "$p" ] || p="$AN/$p"
        trash "$p" && ok "trashed $(short "$p")"
      done < "$tmp/picked"
    elif _list_and_confirm "$title (moved to trash)" "$tmp/$kind"; then
      cut -f2 "$tmp/$kind" | while IFS= read -r p; do trash "$p"; done; ok "trashed"
    fi
  done
  [ -s "$tmp/all" ] || ok "no duplicates found"
  [ "${MGR_DRY:-}" = 1 ] && dim "(dry run — nothing was changed)"
  true
}
