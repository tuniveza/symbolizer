# manager setup — install or repair every build tool. Each component has:
#   have_<c>  -> already installed?      inst_<c> -> install / repair it
# Everything goes into $AN_TOOLS (no sudo) except `system`, which asks before running pacman.

SYSTEM_PKGS=(base-devel git clang cmake ninja nasm pkgconf mesa sdl3 glfw glew glm
  lua lua51 luajit haxe neko dotnet-sdk nodejs npm go python python-uv
  unzip p7zip rsync fzf wine mingw-w64-gcc erlang
  xorg-server-xvfb ffmpeg libvips chromium)   # last line: manager gui (records projects on an invisible screen)

ALL_COMPONENTS=(shell system watchexec c py js hx java love odin crystal godot gleam v dragonruby retro rust)
declare -A ABOUT=(
  [shell]="load the build tools in ~/.zshrc and ~/.bashrc"
  [system]="pacman packages the build tools need (asks for sudo)"
  [watchexec]="file watcher used by every reload.sh"
  [c]="C/C++ libraries in st/tools/c (raylib, imgui, glad, ...)"
  [py]="shared Python venv st/tools/py/venv"
  [js]="bun"
  [hx]="haxelib repo: flixel, lime, openfl, hxcpp, heaps"
  [java]="Temurin JDK 21"
  [love]="LÖVE 11.5"
  [odin]="Odin compiler + vendor libs"
  [crystal]="Crystal + shards"
  [godot]="Godot 4 (latest stable)"
  [gleam]="Gleam"
  [v]="V compiler (built in ~/an/co/v)"
  [dragonruby]="DragonRuby engine copy (paid — copied from a download you have)"
  [retro]="optional: SNES wla-dx + PS1 mipsel gcc"
  [rust]="optional: rustup + cargo in st/tools/tc/rust"
)

TC="$AN_TOOLS/tc"; DL="$AN_TOOLS/.dl"

# ---- download helpers -------------------------------------------------------
gh_asset() { # gh_asset <owner/repo> <regex> [tag] -> download url of the latest (or tagged) release asset
  local api="https://api.github.com/repos/$1/releases" json
  if [ -n "${3:-}" ]; then json="$(curl -fsSL "$api/tags/$3")"
  # some projects (Odin) only publish pre-releases, so there's no "latest" — take the newest of any kind
  else json="$(curl -fsSL "$api/latest" 2>/dev/null || curl -fsSL "$api?per_page=5")"; fi
  grep -oE '"browser_download_url": *"[^"]+"' <<<"$json" | sed -E 's/.*"(https[^"]+)"/\1/' | grep -E "$2" | head -1
}
fetch() { # fetch <url> -> prints local file path (cached in $AN_TOOLS/.dl)
  [ -n "$1" ] || { echo "no download url found (GitHub rate limit or release renamed?)" >&2; return 1; }
  mkdir -p "$DL"; local f="$DL/$(basename "${1%%\?*}")"
  if [ ! -s "$f" ]; then
    curl -fL $([ -t 2 ] && echo --progress-bar || echo -sS) -o "$f.part" "$1" && mv -f "$f.part" "$f" \
      || { rm -f "$f.part"; echo "download failed: $1" >&2; return 1; }
  fi
  echo "$f"
}
unpack() { # unpack <archive> <dest> : extract, flattening a single top-level folder
  local tmp; tmp="$(mktemp -d "$DL/x.XXXX")"
  case "$1" in
    *.zip) unzip -q "$1" -d "$tmp" ;;
    *)     tar xf "$1" -C "$tmp" ;;
  esac
  rm -rf "$2"; mkdir -p "$(dirname "$2")"
  if [ "$(ls -A "$tmp" | wc -l)" = 1 ] && [ -d "$tmp/$(ls -A "$tmp")" ]; then
    mv "$tmp/$(ls -A "$tmp")" "$2"; rmdir "$tmp"
  else
    mv "$tmp" "$2"
  fi
}

# ---- components -------------------------------------------------------------
have_shell() { grep -qs 'an/st/tools/env.sh' "$HOME/.zshrc" && grep -qs 'an/st/tools/env.sh' "$HOME/.bashrc"; }
inst_shell() {
  local rc
  for rc in "$HOME/.zshrc" "$HOME/.bashrc"; do
    grep -qs 'an/st/tools/env.sh' "$rc" || printf '\n# build tools for ~/an (see ~/an/st/tools)\n[ -r ~/an/st/tools/env.sh ] && . ~/an/st/tools/env.sh\n' >> "$rc"
  done
}

missing_pkgs() { command -v pacman >/dev/null && pacman -T "${SYSTEM_PKGS[@]}" 2>/dev/null; }
have_system() { [ -z "$(missing_pkgs)" ]; }
inst_system() {
  command -v pacman >/dev/null || { warn "not an Arch system — install these yourself: ${SYSTEM_PKGS[*]}"; return 1; }
  local m; m="$(missing_pkgs | tr '\n' ' ')"
  echo "  missing: $m"
  if confirm "  run: sudo pacman -S --needed $m ?"; then
    # shellcheck disable=SC2086
    sudo pacman -S --needed $m
  else
    echo "  skipped — run it yourself:  sudo pacman -S --needed $m"; return 1
  fi
}

have_watchexec() { [ -x "$AN_TOOLS/bin/watchexec" ]; }
inst_watchexec() {
  local f; f="$(fetch "$(gh_asset watchexec/watchexec 'x86_64-unknown-linux-musl\.tar\.xz$')")" || return 1
  unpack "$f" "$DL/watchexec" && cp "$DL/watchexec/watchexec" "$AN_TOOLS/bin/" && rm -rf "$DL/watchexec"
}

have_c() { [ -f "$AN_TOOLS/c/lib/libraylib.a" ] && [ -f "$AN_TOOLS/c/lib/libimgui.a" ] && [ -f "$AN_TOOLS/c/include/glad/gl.h" ]; }
inst_c() { "$AN_TOOLS/c/build-libs.sh"; }

have_py() { [ -x "$AN_TOOLS/py/venv/bin/python" ] && PYGAME_HIDE_SUPPORT_PROMPT=1 "$AN_TOOLS/py/venv/bin/python" -c 'import pygame, numpy' 2>/dev/null; }
inst_py() { "$AN_TOOLS/py/setup.sh"; }

have_js() { [ -x "$HOME/.bun/bin/bun" ] || command -v bun >/dev/null; }
inst_js() { curl -fsSL https://bun.sh/install | bash; }

HX_LIBS=(hxcpp lime openfl flixel flixel-addons flixel-ui heaps format hscript hxp)
have_hx() { local l; for l in "${HX_LIBS[@]}"; do [ -d "$AN_TOOLS/hx/haxelib/$l" ] || return 1; done; [ -x "$AN_TOOLS/hx/bin/lime" ]; }
inst_hx() {
  command -v haxelib >/dev/null || { warn "haxe is not installed (manager setup system)"; return 1; }
  mkdir -p "$AN_TOOLS/hx/haxelib" "$AN_TOOLS/hx/bin" "$AN_TOOLS/hx/hxcpp-cache"
  printf '%s' "$AN_TOOLS/hx/haxelib" > "$HOME/.haxelib"
  local l
  for l in "${HX_LIBS[@]}"; do
    [ -d "$AN_TOOLS/hx/haxelib/$l" ] || HAXELIB_PATH="$AN_TOOLS/hx/haxelib" haxelib install "$l" --always --quiet || return 1
  done
  for l in lime openfl flixel; do
    printf '#!/bin/sh\nexec haxelib run %s "$@"\n' "$l" > "$AN_TOOLS/hx/bin/$l"; chmod +x "$AN_TOOLS/hx/bin/$l"
  done
}

have_java() { [ -x "$TC/jdk/current/bin/javac" ]; }
inst_java() {
  local f; f="$(fetch 'https://api.adoptium.net/v3/binary/latest/21/ga/linux/x64/jdk/hotspot/normal/eclipse')" || return 1
  local v; v="$(tar tf "$f" | head -1 | cut -d/ -f1)"
  unpack "$f" "$TC/jdk/$v" && ln -sfn "$v" "$TC/jdk/current"
}

have_love() { [ -x "$TC/love/bin/love" ] && [ -d "$TC/love/love-11.5" ]; }
inst_love() {
  local f; f="$(fetch "$(gh_asset love2d/love 'x86_64\.AppImage$' 11.5)")" || return 1
  chmod +x "$f"; rm -rf "$DL/squashfs-root"
  (cd "$DL" && "$f" --appimage-extract >/dev/null)
  rm -rf "$TC/love/love-11.5"; mkdir -p "$TC/love/bin"; mv "$DL/squashfs-root" "$TC/love/love-11.5"
  printf '#!/bin/sh\nexec "$(dirname "$(readlink -f "$0")")/../love-11.5/AppRun" "$@"\n' > "$TC/love/bin/love"
  chmod +x "$TC/love/bin/love"
}

have_odin() { [ -x "$TC/odin/odin" ] && compgen -G "$TC/odin/vendor/stb/lib/*.a" >/dev/null; }
inst_odin() {
  local f; f="$(fetch "$(gh_asset odin-lang/Odin 'linux-amd64.*\.(tar\.gz|zip)$')")" || return 1
  unpack "$f" "$TC/odin" || return 1
  # the tarball ships vendor C sources only; build the static libs Odin links against
  local d
  for d in stb miniaudio cgltf kb_text_shape box3d; do
    [ -d "$TC/odin/vendor/$d/src" ] && (cd "$TC/odin/vendor/$d/src" && { make -j"$(nproc)" >/dev/null 2>&1 || bash ./*.sh >/dev/null 2>&1; }) || true
  done
  [ -f "$TC/odin/vendor/box2d/build_box2d.sh" ] && (cd "$TC/odin/vendor/box2d" && bash build_box2d.sh >/dev/null 2>&1) || true
}

have_crystal() { [ -x "$TC/crystal/bin/crystal" ]; }
inst_crystal() {
  local f; f="$(fetch "$(gh_asset crystal-lang/crystal 'linux-x86_64\.tar\.gz$')")" || return 1
  unpack "$f" "$TC/crystal"
}

have_godot() { [ -x "$TC/godot/godot" ]; }
inst_godot() {
  local f; f="$(fetch "$(gh_asset godotengine/godot 'stable_linux\.x86_64\.zip$')")" || return 1
  mkdir -p "$TC/godot"; unzip -qo "$f" -d "$DL/godot"
  mv -f "$DL/godot"/Godot_*linux.x86_64 "$TC/godot/godot"; rm -rf "$DL/godot"
}

have_gleam() { [ -x "$TC/gleam/gleam" ]; }
inst_gleam() {
  local f; f="$(fetch "$(gh_asset gleam-lang/gleam 'x86_64-unknown-linux-musl\.tar\.gz$')")" || return 1
  mkdir -p "$TC/gleam"; tar xf "$f" -C "$TC/gleam"
}

have_v() { [ -x "$AN/co/v/v" ]; }
inst_v() {
  local d="$AN/co/v"
  [ -d "$d/.git" ] || git clone --depth 1 https://github.com/vlang/v "$d" || return 1
  # plain `make` re-downloads a bootstrap that may not match this checkout; try self-hosting first
  (cd "$d" && { { [ -x ./v ] && ./v self; } || make; })
}

have_dragonruby() { [ -x "$TC/dragonruby/dragonruby" ]; }
inst_dragonruby() {
  # DragonRuby is paid, so it can't be downloaded — copy an engine you already have
  local src z
  src="$(find "$AN" "$HOME/Downloads" -path "$TC" -prune -o -type f -name dragonruby -perm -u+x -printf '%h\n' 2>/dev/null | while read -r d; do [ -d "$d/.dragonruby" ] && [ -d "$d/mygame" ] && echo "$d" && break; done)"
  if [ -z "$src" ]; then
    z="$(ls -t "$HOME"/Downloads/dragonruby*linux*.zip "$AN"/st/dragonruby*linux*.zip 2>/dev/null | head -1)"
    [ -n "$z" ] && unpack "$z" "$DL/dr" && src="$DL/dr"
  fi
  [ -n "$src" ] || { warn "no DragonRuby found — download the Linux zip from itch.io into ~/Downloads, then: manager setup dragonruby"; return 1; }
  rm -rf "$TC/dragonruby"; mkdir -p "$TC/dragonruby"
  (cd "$src" && tar cf - --exclude=./app --exclude=./logs --exclude=./tmp --exclude=./builds .) | tar xf - -C "$TC/dragonruby"
  rm -rf "$DL/dr"
}

have_retro() { [ -x "$TC/retro/wla-dx/bin/wla-65816" ] && [ -x "$TC/retro/mipsel/bin/mipsel-none-elf-gcc" ]; }
inst_retro() {
  local w="$AN/co/ar/snes/wla-dx"
  if [ ! -x "$TC/retro/wla-dx/bin/wla-65816" ]; then
    [ -d "$w" ] || git clone --depth 1 https://github.com/vhelin/wla-dx "$w" || return 1
    cmake -S "$w" -B "$DL/wla" -G Ninja -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$TC/retro/wla-dx" >/dev/null \
      && cmake --build "$DL/wla" && cmake --install "$DL/wla" >/dev/null && rm -rf "$DL/wla" || return 1
  fi
  if [ ! -x "$TC/retro/mipsel/bin/mipsel-none-elf-gcc" ]; then
    command -v podman >/dev/null || { warn "PS1 gcc needs podman (sudo pacman -S podman)"; return 1; }
    local img=ghcr.io/grumpycoders/pcsx-redux-build:latest c m="$TC/retro/mipsel"
    podman pull "$img" && c="$(podman create "$img")" || return 1
    mkdir -p "$m/bin" "$m/lib/gcc" "$m/libexec/gcc"
    podman export "$c" | tar xf - -C "$DL" usr/local/bin usr/local/mipsel-none-elf usr/local/lib/gcc/mipsel-none-elf usr/local/libexec/gcc/mipsel-none-elf 2>/dev/null
    cp -a "$DL"/usr/local/bin/mipsel-none-elf-* "$m/bin/"
    cp -a "$DL/usr/local/mipsel-none-elf" "$m/"
    cp -a "$DL/usr/local/lib/gcc/mipsel-none-elf" "$m/lib/gcc/"
    cp -a "$DL/usr/local/libexec/gcc/mipsel-none-elf" "$m/libexec/gcc/"
    rm -rf "$DL/usr"; podman rm "$c" >/dev/null; podman rmi "$img" >/dev/null   # image is 1.8 GB; only the compiler is kept
  fi
}

have_rust() { [ -x "$TC/rust/cargo/bin/cargo" ]; }
inst_rust() {
  export RUSTUP_HOME="$TC/rust/rustup" CARGO_HOME="$TC/rust/cargo"
  curl -fsSL https://sh.rustup.rs | sh -s -- -y --no-modify-path --profile default || return 1
  cat > "$AN_TOOLS/env.d/rust.sh" <<'EOF'
# Rust (rustup + cargo in st/tools/tc/rust)
export RUSTUP_HOME="$AN_TOOLS/tc/rust/rustup" CARGO_HOME="$AN_TOOLS/tc/rust/cargo"
case ":$PATH:" in *":$CARGO_HOME/bin:"*) ;; *) export PATH="$CARGO_HOME/bin:$PATH" ;; esac
EOF
}

# ---- driver -----------------------------------------------------------------
mgr_setup() { # manager setup [components...|all|--list] [--force]
  local force=0 comps=() c
  for c in "$@"; do
    case "$c" in
      --force|-f) force=1 ;;
      --list|-l)
        for c in "${ALL_COMPONENTS[@]}"; do
          if "have_$c" 2>/dev/null; then printf '  %sok%s  ' "$c_g" "$c_0"; else printf '  %s--%s  ' "$c_y" "$c_0"; fi
          printf '%-11s %s\n' "$c" "${ABOUT[$c]}"
        done; return ;;
      all) comps=("${ALL_COMPONENTS[@]}") ;;
      *) [ -n "${ABOUT[$c]:-}" ] || die "unknown component '$c' (manager setup --list)"; comps+=("$c") ;;
    esac
  done
  [ ${#comps[@]} -gt 0 ] || comps=("${SETUP_DEFAULT[@]}")
  mkdir -p "$TC" "$DL"
  local failed=()
  for c in "${comps[@]}"; do
    if [ $force = 0 ] && "have_$c"; then ok "$c"; continue; fi
    info "installing $c — ${ABOUT[$c]}"
    if "inst_$c" && "have_$c"; then ok "$c"; else bad "$c"; failed+=("$c"); fi
  done
  # shellcheck source=/dev/null
  . "$AN_TOOLS/env.sh"
  if [ ${#failed[@]} -gt 0 ]; then echo; warn "not done: ${failed[*]}  (rerun: manager setup ${failed[*]})"; return 1; fi
  echo; dim "all set. open a new terminal (or: source ~/an/st/tools/env.sh) to pick up PATH changes."
}
