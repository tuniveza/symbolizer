# shared helpers for manager subcommands (sourced by bin/manager)

AN="$HOME/an"
if [ -t 1 ]; then
  c_b=$'\e[1m' c_d=$'\e[2m' c_r=$'\e[31m' c_g=$'\e[32m' c_y=$'\e[33m' c_c=$'\e[36m' c_0=$'\e[0m'
else
  c_b= c_d= c_r= c_g= c_y= c_c= c_0=
fi

info() { printf '%s==>%s %s\n' "$c_b$c_c" "$c_0" "$*"; }
ok()   { printf '  %sok%s    %s\n' "$c_g" "$c_0" "$*"; }
warn() { printf '  %s!!%s    %s\n' "$c_y" "$c_0" "$*" >&2; }
bad()  { printf '  %sFAIL%s  %s\n' "$c_r" "$c_0" "$*"; }
dim()  { printf '%s%s%s\n' "$c_d" "$*" "$c_0"; }
die()  { printf 'manager: %s\n' "$*" >&2; exit 1; }
short() { local p="$1"; p="${p#$AN/}"; echo "${p/#$HOME/\~}"; }

# confirm "question" -> yes? (MGR_YES=1 answers yes; no terminal answers no)
confirm() {
  [ "${MGR_YES:-}" = 1 ] && return 0
  [ -t 0 ] || return 1
  local a; read -r -p "$1 [y/N] " a; [[ "$a" == [yY]* ]]
}

# move to the desktop trash (recoverable); falls back to $AN_MANAGER/.trash
trash() {
  local p
  for p in "$@"; do
    [ -e "$p" ] || [ -L "$p" ] || continue
    if command -v gio >/dev/null && gio trash -- "$p" 2>/dev/null; then continue; fi
    mkdir -p "$AN_MANAGER/.trash"; mv -- "$p" "$AN_MANAGER/.trash/$(date +%s)-$(basename "$p")"
  done
}

human() { numfmt --to=iec --suffix=B "${1:-0}" 2>/dev/null || echo "${1}B"; }
dsize() { du -sb "$@" 2>/dev/null | awk '{s+=$1} END{print s+0}'; }

# settings
abspath() { case "$1" in /*) echo "$1" ;; *) echo "$AN/$1" ;; esac; }
ROOTS=(co pj ca); IGNORE=(); SETUP_DEFAULT=()
# shellcheck source=/dev/null
[ -r "$AN_MANAGER/manager.conf" ] && . "$AN_MANAGER/manager.conf"
_r=(); for x in "${ROOTS[@]}"; do x="$(abspath "$x")"; [ -d "$x" ] && _r+=("$x"); done; ROOTS=("${_r[@]}")
_i=(); for x in "${IGNORE[@]}"; do _i+=("$(abspath "$x")"); done; IGNORE=("${_i[@]}")
unset _r _i x

is_ignored() { # is_ignored <abs dir>   (IGNORE entries may use * wildcards)
  local i
  for i in "${IGNORE[@]}"; do
    # shellcheck disable=SC2254
    case "$1/" in $i/*) return 0 ;; esac
  done
  # dependencies a package manager downloaded into a project (crystal shards -> lib/)
  case "$1/" in */lib/*) [ -f "${1%%/lib/*}/shard.yml" ] && return 0 ;; esac
  return 1
}

# directories never worth descending into
PRUNE_NAMES=(node_modules .git .hg venv .venv __pycache__ .godot .gradle .cache .wrangler .idea .vscode
             build dist out obj target .dragonruby .manager-trash
             vendor vendored deps third_party external)
prune_expr() { # prints a find expression (as array elements) that prunes PRUNE_NAMES
  local n first=1
  printf '%s\n' '('
  for n in "${PRUNE_NAMES[@]}"; do
    [ $first = 1 ] || printf '%s\n' '-o'
    printf '%s\n' -name "$n"; first=0
  done
  # the Manager's own project templates are not projects
  printf '%s\n' -o -path "${AN_MANAGER:-$HOME/an/ca/sy/manager}/templates"
  printf '%s\n' ')' -prune
}

# fzf picker with plain-bash fallback:  pick "prompt" [--multi] < lines
pick() {
  local prompt="$1" multi="${2:-}"
  if command -v fzf >/dev/null; then   # fzf reads keys from /dev/tty itself
    fzf --height=~90% --reverse --prompt="$prompt> " ${multi:+--multi --bind ctrl-a:select-all}
  else
    local items=() it i=1 a
    while IFS= read -r it; do items+=("$it"); done
    for it in "${items[@]}"; do printf '%3d) %s\n' $i "$it" >&2; i=$((i+1)); done
    read -r -p "$prompt (number${multi:+s, space separated}): " a </dev/tty
    for i in $a; do [ "$i" -ge 1 ] 2>/dev/null && echo "${items[$((i-1))]}"; done
  fi
}
