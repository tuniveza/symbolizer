# project discovery, type detection and adopting (giving a folder make.sh + reload.sh)

# every project = a dir with an executable make.sh
mgr_projects() { # [dirs...]
  local roots=("$@") pe
  [ ${#roots[@]} -gt 0 ] || roots=("${ROOTS[@]}")
  mapfile -t pe < <(prune_expr)
  find "${roots[@]}" -path "$AN_TOOLS" -prune -o "${pe[@]}" -o -name make.sh -perm -u+x -printf '%h\n' 2>/dev/null | sort -u
}

# folders that look like projects but have no make.sh yet (shallowest match wins)
mgr_candidates() { # [dirs...]
  local roots=("$@") pe d f
  [ ${#roots[@]} -gt 0 ] || roots=("${ROOTS[@]}")
  mapfile -t pe < <(prune_expr)
  declare -A strong=() normal=() weak=() proj=()
  while IFS=$'\t' read -r d f; do
    case "$f" in
      project.godot|package.json|Cargo.toml|go.mod|gleam.toml|shard.yml|settings.gradle|settings.gradle.kts|Project.xml|*.csproj)
        strong[$d]=1 ;;
      Makefile|makefile|GNUmakefile|CMakeLists.txt|build.gradle|build.gradle.kts|main.lua|conf.lua|v.mod|main.v|main.py|*.toc|*.odin)
        normal[$d]=1 ;;
      main.rb) [ "$(basename "$d")" = app ] && normal[$(dirname "$d")]=1 ;;
      make.sh) proj[$d]=1 ;;
      index.html) weak[$d]=1 ;;
      *.py) weak[$d]=1 ;;
      *.c|*.cpp|*.cc) grep -qsE '(^|[^a-z_])main[[:space:]]*\(' "$d/$f" && weak[$d]=1 ;;
      *.asm) grep -qsE '^[[:space:]]*(main|_start)[[:space:]]*:' "$d/$f" && weak[$d]=1 ;;
    esac
  done < <(find "${roots[@]}" -path "$AN_TOOLS" -prune -o "${pe[@]}" -o -type f \( \
      -name project.godot -o -name package.json -o -name Cargo.toml -o -name go.mod -o -name gleam.toml \
      -o -name shard.yml -o -name 'settings.gradle*' -o -name Project.xml -o -name '*.csproj' \
      -o -name Makefile -o -name makefile -o -name GNUmakefile -o -name CMakeLists.txt -o -name 'build.gradle*' \
      -o -name main.lua -o -name conf.lua -o -name v.mod -o -name main.v -o -name main.py -o -name '*.toc' \
      -o -name '*.odin' -o -name main.rb -o -name make.sh -o -name index.html -o -name '*.py' \
      -o -name '*.c' -o -name '*.cpp' -o -name '*.cc' -o -name '*.asm' \) -printf '%h\t%f\n' 2>/dev/null)

  # a weak folder named src/source/code really belongs to its parent
  for d in "${!weak[@]}"; do
    case "$(basename "$d")" in src|source|code|Source)
      local p; p="$(dirname "$d")"
      [ -z "${proj[$p]:-}" ] && weak[$p]=1 && unset "weak[$d]" ;;
    esac
  done

  local all=() kind
  for d in "${!strong[@]}"; do all+=("$d	s"); done
  for d in "${!normal[@]}"; do [ -z "${strong[$d]:-}" ] && all+=("$d	n"); done
  for d in "${!weak[@]}"; do [ -z "${strong[$d]:-}${normal[$d]:-}" ] && all+=("$d	w"); done

  # keep: not adopted, not ignored, no adopted/candidate ancestor (manifests may nest
  # inside adopted projects), and weak folders must not contain other projects
  local keep=() a anc skip
  while IFS=$'\t' read -r d kind; do
    [ -n "${proj[$d]:-}" ] && continue
    is_ignored "$d" && continue
    skip=0; anc="$(dirname "$d")"
    while [ "$anc" != / ] && [ "$anc" != "$AN" ] && [ "$anc" != "$HOME" ]; do
      if [ -n "${proj[$anc]:-}" ] && [ "$kind" != s ]; then skip=1; break; fi
      for a in "${keep[@]}"; do [ "$a" = "$anc" ] && { skip=1; break 2; }; done
      anc="$(dirname "$anc")"
    done
    [ $skip = 1 ] && continue
    if [ "$kind" = w ]; then
      for a in "${!proj[@]}" "${!strong[@]}" "${!normal[@]}"; do
        case "$a" in "$d"/*) skip=1; break ;; esac
      done
      [ $skip = 1 ] && continue
    fi
    keep+=("$d")
  done < <(printf '%s\n' "${all[@]}" | sort)   # parents sort before children
  [ ${#keep[@]} -gt 0 ] && printf '%s\n' "${keep[@]}"
  return 0
}

# gradle project that builds an Android app? (new-style plugin aliases never spell out com.android)
is_android() {
  grep -qs 'com\.android\|plugins\.android\.' "$1"/build.gradle* "$1"/*/build.gradle* "$1"/settings.gradle* \
    || [ -n "$(find "$1" -maxdepth 4 -name AndroidManifest.xml -not -path '*/build/*' -print -quit 2>/dev/null)" ]
}

# what kind of project is this folder?
mgr_detect() {
  local d="$1"
  [ -f "$d/project.godot" ] && { echo godot; return; }
  compgen -G "$d/*.csproj" >/dev/null && { echo dotnet; return; }
  [ -f "$d/Project.xml" ] && { echo lime; return; }
  [ -f "$d/gleam.toml" ] && { echo gleam; return; }
  [ -f "$d/shard.yml" ] && { echo crystal; return; }
  [ -f "$d/go.mod" ] && { echo go; return; }
  [ -f "$d/Cargo.toml" ] && { echo rust; return; }
  if compgen -G "$d/settings.gradle*" >/dev/null || compgen -G "$d/build.gradle*" >/dev/null; then
    if is_android "$d"; then echo android; else echo gradle; fi
    return
  fi
  if [ -f "$d/package.json" ]; then
    if compgen -G "$d/wrangler.*" >/dev/null; then echo js-worker
    elif grep -q '"vite"' "$d/package.json"; then echo js-vite
    else echo js-node; fi
    return
  fi
  [ -f "$d/app/main.rb" ] && { echo dragonruby; return; }
  compgen -G "$d/*.toc" >/dev/null && { echo wow; return; }
  compgen -G "$d/*.odin" >/dev/null && { echo odin; return; }
  { [ -f "$d/v.mod" ] || grep -qs '^fn main' "$d"/*.v; } && { echo v; return; }
  if [ -f "$d/main.lua" ]; then
    if grep -qs 'love\.' "$d"/*.lua; then echo love; else echo lua; fi
    return
  fi
  { [ -f "$d/Makefile" ] || [ -f "$d/makefile" ] || [ -f "$d/GNUmakefile" ]; } && { echo make; return; }
  [ -f "$d/CMakeLists.txt" ] && { echo cmake; return; }
  compgen -G "$d/*.asm" >/dev/null && { echo nasm; return; }
  if [ -n "$(find "$d" -maxdepth 3 \( -name '*.cpp' -o -name '*.cc' \) -print -quit)" ]; then echo cpp; return; fi
  if [ -n "$(find "$d" -maxdepth 3 -name '*.c' -print -quit)" ]; then echo c; return; fi
  if compgen -G "$d/*.py" >/dev/null || compgen -G "$d/src/*.py" >/dev/null; then echo py; return; fi
  [ -f "$d/index.html" ] && { echo web; return; }
  echo unknown
}

# ---- script writers ---------------------------------------------------------
_HDR='#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
. "$HOME/an/st/tools/env.sh"'

_put() { # _put <file> <content>
  printf '%s\n' "$2" > "$1"; chmod +x "$1"
}
_reload() { # _reload <dir> <exts> : generic watchexec reload.sh
  _put "$1/reload.sh" "#!/usr/bin/env bash
# rebuild + rerun on every save
cd \"\$(dirname \"\$0\")\"
. \"\$HOME/an/st/tools/env.sh\"
exec watchexec -r -c -e $2 -i 'build/**' -i 'node_modules/**' -- ./make.sh \"\$@\""
}
_from() { # _from <template> <dir> <name> : copy a template's make.sh + reload.sh
  local f
  for f in make.sh reload.sh; do
    sed "s/__NAME__/$3/g" "$AN_MANAGER/templates/$1/$f" > "$2/$f"; chmod +x "$2/$f"
  done
}
_from_js() { # _from_js <kind> <dir>
  cp "$AN_TOOLS/js/mk/$1.make.sh" "$2/make.sh"; cp "$AN_TOOLS/js/mk/$1.reload.sh" "$2/reload.sh"
  chmod +x "$2/make.sh" "$2/reload.sh"
}
_c_use() { # guess USE= from #includes
  local d="$1" use=() inc
  inc="$(grep -rhoE '#\s*include\s*[<"][^>"]+' --include='*.c' --include='*.cpp' --include='*.cc' --include='*.h' --include='*.hpp' "$d" 2>/dev/null | sed -E 's/.*[<"]//' | sort -u)"
  grep -q '^SDL3/' <<<"$inc" && use+=(sdl3)
  grep -q '^SDL2/\|^SDL\.h' <<<"$inc" && use+=(sdl2)
  grep -q '^raylib\.h' <<<"$inc" && use+=(raylib)
  grep -q '^GLFW/' <<<"$inc" && use+=(glfw3)
  grep -q '^glad/' <<<"$inc" && use+=(glad)
  grep -q '^GL/gl\.h\|SDL_opengl' <<<"$inc" && ! grep -q '^glad/' <<<"$inc" && use+=(gl)
  grep -q '^cglm/' <<<"$inc" && use+=(cglm)
  grep -q '^glm/' <<<"$inc" && use+=(glm)
  grep -q '^stb_' <<<"$inc" && use+=(stb)
  grep -q '^cgltf' <<<"$inc" && use+=(cgltf)
  grep -q '^miniaudio' <<<"$inc" && use+=(miniaudio)
  grep -q '^imgui' <<<"$inc" && use+=(imgui)
  grep -q '^box2d/' <<<"$inc" && use+=(box2d)
  grep -q '^flecs' <<<"$inc" && use+=(flecs)
  grep -q '^entt/' <<<"$inc" && use+=(entt)
  grep -q '^nlohmann/' <<<"$inc" && use+=(nlohmann_json)
  grep -q '^sokol_' <<<"$inc" && use+=(sokol)
  grep -q '^webgpu/' <<<"$inc" && use+=(wgpu)
  echo "${use[*]}"
}

# mgr_adopt_one <dir> [type] : write make.sh + reload.sh (+ Makefile for bare C/C++)
mgr_adopt_one() {
  local d="${1%/}" t="${2:-}" name main
  t="${t:-$(mgr_detect "$d")}"
  name="$(basename "$d" | tr -c 'A-Za-z0-9_\n-' '_')"
  case "$t" in
    godot|gleam|go|odin|v|love) _from "$t" "$d" "$name" ;;
    lime)    _from flixel "$d" "$name" ;;
    js-vite) _from_js vite "$d" ;;
    js-worker) _from_js worker "$d" ;;
    web)     _from_js static "$d" ;;
    crystal)
      _from crystal "$d" "$name"
      main="$(sed -nE 's/^ *main: *//p' "$d/shard.yml" | head -1)"
      [ -z "$main" ] && main="$(cd "$d" && ls src/*.cr 2>/dev/null | head -1)"
      [ -n "$main" ] && sed -i "s|src/$name.cr|$main|g" "$d/make.sh" ;;
    py)
      _from py "$d" "$name"
      main="$(cd "$d" && { ls main.py __main__.py 2>/dev/null; grep -lE '^if __name__ *== *.__main__.' ./*.py src/*.py 2>/dev/null; ls ./*.py src/*.py 2>/dev/null; } | head -1)"
      main="${main#./}"
      sed -i "s|^MAIN=main.py|MAIN=${main:-main.py}|" "$d/make.sh" ;;
    dragonruby)
      _from dragonruby "$d" "$name"
      if [ ! -x "$d/dragonruby" ] && [ -d "$AN_TOOLS/tc/dragonruby" ]; then
        local f
        for f in dragonruby dragonruby-publish dragonruby-httpd .dragonruby font.ttf; do
          [ -e "$d/$f" ] || cp -a "$AN_TOOLS/tc/dragonruby/$f" "$d/"
        done
      fi ;;
    dotnet)
      _put "$d/make.sh" "$_HDR
# .NET: ./make.sh [build|clean|args...]
tools() { [ ! -f .config/dotnet-tools.json ] || dotnet tool restore >/dev/null; }
case \"\$1\" in
  clean) dotnet clean -nologo -v q >/dev/null; rm -rf bin obj ;;
  build) tools; dotnet build -nologo -v q ;;
  *)     tools; dotnet run \"\$@\" ;;
esac"
      _reload "$d" cs,csproj ;;
    rust)
      _put "$d/make.sh" "$_HDR
# Rust (cargo only rebuilds what changed): ./make.sh [build|clean|test|args...]
case \"\$1\" in
  clean) cargo clean ;;
  build) cargo build ;;
  test)  shift; cargo test \"\$@\" ;;
  *)     cargo run -- \"\$@\" ;;
esac"
      _reload "$d" rs,toml ;;
    android)
      main="$(grep -rhoE 'applicationId *=? *\"[^\"]+\"' "$d"/app/build.gradle* 2>/dev/null | grep -oE '\"[^\"]+\"' | tr -d '\"' | head -1)"
      _put "$d/make.sh" "$_HDR
# Android (gradle): ./make.sh [build|clean]
# run = build, then install + launch on a connected device/emulator if adb sees one
APP_ID=${main:-change.me}
case \"\$1\" in
  clean) ./gradlew -q clean ;;
  build) ./gradlew -q assembleDebug ;;
  *)
    ./gradlew -q assembleDebug
    if adb get-state >/dev/null 2>&1; then
      ./gradlew -q installDebug
      adb shell monkey -p \"\$APP_ID\" -c android.intent.category.LAUNCHER 1 >/dev/null
    else
      echo \"no device attached — apk: app/build/outputs/apk/debug/app-debug.apk\"
    fi ;;
esac"
      _reload "$d" java,kt,xml,kts,gradle ;;
    gradle)
      _put "$d/make.sh" "$_HDR
# gradle: ./make.sh [build|clean|args...]   (run needs the 'application' plugin)
case \"\$1\" in
  clean) ./gradlew -q clean ;;
  build) ./gradlew -q assemble ;;
  *)     ./gradlew -q run --args=\"\$*\" ;;
esac"
      _reload "$d" java,kt,kts,gradle ;;
    js-node)
      main="$(sed -nE 's/^ *"main" *: *"([^"]+)".*/\1/p' "$d/package.json" | head -1)"
      _put "$d/make.sh" "$_HDR
. \"\$AN_TOOLS/js/lib.sh\"
# node/bun project: ./make.sh [build|clean|args...]
has() { grep -q \"\\\"\$1\\\" *:\" package.json; }   # does package.json define this script?
case \"\${1:-}\" in
  build) js_deps
         if has build; then js_run build
         elif has typecheck; then js_run typecheck
         elif [ -f tsconfig.json ]; then js_x tsc --noEmit
         else node --check ${main:-index.js} && echo \"[build] syntax ok\"; fi ;;
  clean) rm -rf dist; [ \"\${2:-}\" = all ] && rm -rf node_modules; true ;;
  *)     js_deps
         if has dev; then js_run dev \"\$@\"
         elif has start; then js_run start \"\$@\"
         else exec node ${main:-index.js} \"\$@\"; fi ;;
esac"
      _reload "$d" ts,js,mjs,json ;;
    wow)
      _put "$d/make.sh" "$_HDR
# WoW addon: ./make.sh build = Lua 5.1 syntax check; ./make.sh = check + copy into \$WOW_ADDONS/$name
check() { find . -name '*.lua' -not -path './.git/*' -print0 | xargs -0 -r luac5.1 -p; }
case \"\$1\" in
  clean) ;;
  build) check ;;
  *) check
     if [ -n \"\$WOW_ADDONS\" ]; then
       mkdir -p \"\$WOW_ADDONS/$name\"
       rsync -a --delete --exclude .git ./ \"\$WOW_ADDONS/$name/\" && echo \"installed -> \$WOW_ADDONS/$name\"
     else echo \"syntax OK. set WOW_ADDONS=<path to Interface/AddOns> to install\"; fi ;;
esac"
      _reload "$d" lua,toc,xml ;;
    lua)
      _put "$d/make.sh" "$_HDR
# Lua: ./make.sh build = syntax check, ./make.sh = run main.lua
check() { find . -name '*.lua' -not -path './.*' -print0 | xargs -0 -r luac -p; }
case \"\$1\" in
  clean) ;;
  build) check ;;
  *)     check && exec lua main.lua \"\$@\" ;;
esac"
      _reload "$d" lua ;;
    make)
      local mf; mf="$(ls "$d"/Makefile "$d"/makefile "$d"/GNUmakefile 2>/dev/null | head -1)"
      main="$(sed -nE 's/^ *TARGET *[:?]?= *([^ #]+).*/\1/p' "$mf" | head -1)"
      _put "$d/make.sh" "$_HDR
# make: ./make.sh [build|clean|args...]
case \"\$1\" in
  build) shift; make \"\$@\" ;;
  clean) make clean ;;
  *)     make${main:+ && exec ./$main \"\$@\"} ;;
esac"
      _reload "$d" c,h,cpp,hpp,cc,asm,s,glsl,vert,frag ;;
    cmake)
      main="$(grep -oE 'add_executable *\( *[A-Za-z0-9_.-]+' "$d/CMakeLists.txt" | head -1 | sed -E 's/.*\( *//')"
      _put "$d/make.sh" "$_HDR
# CMake + Ninja (incremental): ./make.sh [build|clean|args...]
build() {
  [ -f build/build.ninja ] || cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Debug -DCMAKE_EXPORT_COMPILE_COMMANDS=ON
  cmake --build build
}
case \"\$1\" in
  clean) rm -rf build ;;
  build) build ;;
  *)     build && exec \"\$(find build -type f -name '${main:-$name}' -executable | head -1)\" \"\$@\" ;;
esac"
      _reload "$d" c,h,cpp,hpp,cc,txt,cmake ;;
    nasm)
      _put "$d/Makefile" "# NASM: assemble every .asm, link with clang
TARGET = $name
OBJS = \$(patsubst %.asm,build/%.o,\$(wildcard *.asm))

\$(TARGET): \$(OBJS)
	clang -no-pie \$^ -o \$@

build/%.o: %.asm Makefile
	@mkdir -p build
	nasm -f elf64 -g -F dwarf \$< -o \$@

clean:
	rm -rf build \$(TARGET)
.PHONY: clean"
      mgr_adopt_one "$d" make; return ;;
    c|cpp)
      if [ ! -f "$d/Makefile" ] && [ ! -f "$d/makefile" ]; then
        _put "$d/Makefile" "TARGET = $name
USE    = $(_c_use "$d")
include \$(HOME)/an/st/tools/c/tools.mk"
        chmod -x "$d/Makefile"
      fi
      _from c "$d" "$name" ;;
    *) warn "$(short "$d"): don't know how to build this ($t) — skipped"; return 1 ;;
  esac
  return 0
}
