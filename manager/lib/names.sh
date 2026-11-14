# readable names for the two-letter folders and the projects (from $AN_MANAGER/names.tsv)

declare -gA MGR_NAME=() MGR_ABOUT=() MGR_BARE=()
_lb_names_loaded=0

mgr_names_load() {
  [ $_lb_names_loaded = 1 ] && return
  _lb_names_loaded=1
  local p n a
  [ -r "$AN_MANAGER/names.tsv" ] || return 0
  while IFS=$'\t' read -r p n a; do
    case "$p" in ''|'#'*) continue ;; '*/'*) MGR_BARE[${p#*/}]="$n"; continue ;; '~/'*) p="$HOME/${p#\~/}" ;; '~') p="$HOME" ;; *) p="$AN/$p" ;; esac
    MGR_NAME[$p]="$n"; [ -n "$a" ] && MGR_ABOUT[$p]="$a"
  done < "$AN_MANAGER/names.tsv"
}

mgr_name() { # mgr_name <abs dir> -> its name (names.tsv, then */folder entries, then the folder itself)
  mgr_names_load
  local d="${1%/}" b; b="${d##*/}"
  echo "${MGR_NAME[$d]:-${MGR_BARE[$b]:-$b}}"
}

mgr_about() { mgr_names_load; echo "${MGR_ABOUT[${1%/}]:-}"; }

mgr_trail() { # mgr_trail <abs dir> -> what the folders above it mean, e.g. "Claude apps › Apps"
  mgr_names_load
  local d="${1%/}" top out="" seg
  case "$d" in "$AN"/*) top="$AN" ;; "$HOME"/*) top="$HOME" ;; *) return ;; esac
  d="$(dirname "$d")"
  while [ "$d" != "$top" ] && [ "$d" != / ]; do
    seg="$(mgr_name "$d")"; out="$seg${out:+ › $out}"; d="$(dirname "$d")"
  done
  echo "$out"
}

# manager names            every short folder name and what it stands for
# manager names <path...>  spell out a path, e.g. manager names ca/ap/pv
mgr_names() {
  mgr_names_load
  local a d p part acc parts=() all=()
  if [ $# -eq 0 ]; then
    info "short folder names in ~/an  (edit $(short "$AN_MANAGER/names.tsv") to change them)"
    mapfile -t all < <(printf '%s\n' "${!MGR_NAME[@]}" | sed 's|/|\x01|g' | LC_ALL=C sort | sed 's|\x01|/|g')
    for d in "${all[@]}"; do
      case "$d" in "$AN"/*) ;; *) continue ;; esac
      # projects are in `manager projects`, unless named folders sit inside them
      [ -x "$d/make.sh" ] && ! printf '%s\n' "${all[@]}" | grep -qF "$d/" && continue
      p="${d#$AN/}"; local up="${p//[^\/]/}"
      printf '%*s%s%-6s%s %s%s%s\n' $(( ${#up} * 4 )) '' "$c_b" "${p##*/}" "$c_0" "${MGR_NAME[$d]}" \
        "${MGR_ABOUT[$d]:+$c_d  — ${MGR_ABOUT[$d]}}" "$c_0"
    done
    echo; dim "  any folder called: $(for b in $(printf '%s\n' "${!MGR_BARE[@]}" | sort); do printf '%s = %s   ' "$b" "${MGR_BARE[$b]}"; done)"
    return
  fi
  for a in "$@"; do
    d="$(readlink -m "$(case "$a" in /*|~*) echo "${a/#\~/$HOME}" ;; *) [ -e "$a" ] && readlink -m "$a" || abspath "$a" ;; esac)")"
    case "$d" in "$AN"/*) acc="$AN"; p="${d#$AN/}" ;; "$HOME"/*) acc="$HOME"; p="${d#$HOME/}" ;; *) echo "  $a: not under ~/an"; continue ;; esac
    printf '%s%s%s\n' "$c_b" "$(short "$d")" "$c_0"
    IFS=/ read -ra parts <<<"$p"
    for part in "${parts[@]}"; do
      acc="$acc/$part"
      printf '  %-14s %s%s%s\n' "$part" "$(mgr_name "$acc")" "$c_d" "${MGR_ABOUT[$acc]:+  — ${MGR_ABOUT[$acc]}}$c_0"
    done
  done
}
