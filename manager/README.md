# manager — one program for everything in ~/an

The command-line half of Symbolizer's Manager mode (`sy`, then the Manager). It used to be called `lb`.
Its toolchains and libraries live in the Storage area: `~/an/st/tools`.

`manager` is on your PATH (loaded from `~/.zshrc` / `~/.bashrc`). Type **`manager`** for the menu,
or use the commands below. `manager help` prints them all.

## Every project works the same way

```sh
./make.sh            # build whatever changed, then run   (args go to the program)
./make.sh build      # build only (no window)
./make.sh clean      # remove build output
./reload.sh          # rebuild + rerun on every save
```

From anywhere inside a project: `manager run`, `manager build`, `manager clean`, `manager watch`.

## Commands

| | |
|---|---|
| `manager` | menu: run/watch the project you're in, pick any project, new project, maintenance |
| `manager open [dir\|file]` | open in its editor: Godot projects in the newest Godot, Construct 3 in the browser (path copied to clipboard), GDevelop, Android Studio, everything else in VSCodium. `-n` shows what it would run. Editors can be changed in `manager.conf` (`CODE_EDITOR=…`) |
| `manager new <lang> [name] [dir]` | start a project from a template (`manager langs` lists 20); no name, or the current folder's name, fills the current folder instead of making `name/name` |
| `manager projects` | every project: its type, its name, its folder, and what the folders above it mean |
| `manager names [path]` | what every two-letter folder stands for, or spell out one path (`manager names ca/ap/pv`). Names live in `names.tsv` — edit it to rename anything |
| `manager gui` | every project as a card with pictures and videos, in a window of its own: run / watch / build / open / folder buttons, search, groups. Also in Walker as **manager** |
| `manager index [--force]` | refresh what `manager gui` shows. A systemd timer (`manager-index.timer`) does it every 3 days at idle priority, only on mains power; unchanged projects are skipped |
| `manager scan` | folders that look like projects but have no `make.sh` yet |
| `manager adopt [dir] [-y]` | give them `make.sh` + `reload.sh` (detects 25 project types) and test-build |
| `manager check [dir]` | build every project in a clean environment; logs of failures in `manager/.logs/` |
| `manager tidy [-n] [--builds]` | delete junk (`.DS_Store`, `__pycache__`, `a.out`), trash stray venvs / `__MACOSX`; `--builds` also cleans every project |
| `manager dupes [-n] [--min MB]` | find archives already unpacked, `(1)` copies, identical `.bak`, identical big files |
| `manager setup [what…]` | install / repair toolchains + libraries (`--list`, `--force`, `all`) |
| `manager status` | what's installed, project count, last check |
| `manager libs` | C/C++ libraries available to `USE =` |

`-n` = dry run. Nothing is removed without asking, and everything except pure junk goes to the trash.

## On a new machine

```sh
~/an/ca/sy/manager/bin/manager setup       # shell hook, pacman packages (asks for sudo), every toolchain + library
manager check                    # confirm every project builds
```

`manager setup all` also installs the optional `retro` (SNES/PS1) and `rust` components.
DragonRuby is paid, so setup copies it from a download you already have.

## What's in here

Here (`~/an/ca/sy/manager`):

| | |
|---|---|
| `bin/manager` | the command |
| `lib/` | its code (`setup.sh`, `projects.sh`, `tidy.sh`, `dupes.py`, `menu.sh`) |
| `manager.conf` | settings: project roots, folders to ignore |
| `names.tsv` | readable names for folders and projects |
| `templates/` | `manager new` starters |
| `gui/` | `manager gui`: `server.py` (the window, http://localhost:8767), `index.py` + `capture.py` (pictures/videos), `www/` (the page). Output in `~/.cache/an-manager/gui` |

In the Storage area (`~/an/st/tools`, "Build tools"):

| | |
|---|---|
| `env.sh` | loaded by `~/.zshrc` / `~/.bashrc`; sets `$AN_TOOLS`, `$AN_MANAGER` and PATH |
| `bin/` | `watchexec`, `godot-project`, `ps1-make` |
| `c/` | ~30 C/C++ libs + `tools.mk` — see `docs/c.md` |
| `py/venv` | shared Python venv; `tools-python`, `tools-pip` |
| `js/` | build helpers for vite / static / worker projects |
| `hx/` | haxelib repo |
| `tc/` | downloaded toolchains (safe to delete; `manager setup` rebuilds them) |
| `env.d/` | one small env file per language |
| `docs/` | per-language notes |

## Pictures and videos in `manager gui`

Each project gets, in order: hand-picked media from the Application Organizer (`ca/ap/heat-map`),
screenshots/previews/videos found in its folder (named `screenshot…`, `preview…`, or in a `screenshots/`,
`media/`, `docs/` folder — never downloaded asset packs), a 7-second recording of it running on an
invisible screen (needs `xorg-server-xvfb`; muted; Nova projects are never run), and failing all that,
a picture of its main source file with a clip paging through it. Put a `.no-capture` file in a
project to stop it being run. Add your own screenshot to a project and it shows up at the next refresh.

A C/C++ project Makefile is just:

```make
TARGET = game
USE    = sdl3 glad cglm stb        # any of these or system pkg-config names (`manager libs`)
include $(HOME)/an/st/tools/c/tools.mk
```

System packages that need sudo are installed by `manager setup system` (it asks first).
