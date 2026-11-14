# bitwin: brick's own windows, as bits brick writes itself

The owner's words: "it's bitstreams, so it should be bitstream windows".
A brick program that opens a window speaks the Wayland protocol itself, over a Unix socket, with raw
system calls, and draws every pixel itself into shared memory. No libraries, no libc, no helper app,
no GPU driver. Exactly like the rest of brick: brick writes every byte.

This file is the contract between the parts. Each part is a Python module that adds machine-code
routines to a program (like `runtime.py` and `screen.py` do), plus tests.

| Part | Module | What it is |
|---|---|---|
| Wayland client | `wayland.py` | connect, window, shared-memory buffers, present, input events |
| Fake compositor (tests only) | `tests/wayland_stub.py` | a tiny Wayland server in Python that saves frames and fakes clicks/keys |
| Painting | `paint.py` + `font.py` | boxes, gradients, textures, rounded corners, borders, shadows, text, lines, ellipses, triangles |
| 3D view ("context (gl)") | `glspace.py` | brick's own software 3D: shapes, depth buffer, light, spin |
| Elements + English | `bitwin.py` | window, button, canvas, menu, tab, text, gl view; position, size, depth, style; events; the English words |

## Rules for every part

- Python standard library only. Machine code only through `asm.Asm` (see `asm.py`; the section
  "more instructions, for drawing and windows" was added for this work). If you need another instruction,
  add it at the END of that section, encode it carefully, and check it with
  `objdump -D -b binary -mi386:x86-64 -M intel`.
- brick's calling convention (see `runtime.py`): inputs in rax, rcx, rdx; the answer in rax (extra answers
  in rcx, rdx, r8 where this file says so). A routine may change ANY register except rsp. Callers keep what
  they need on the stack. Routines that need more inputs take one pointer (rax) to a block of 8-byte fields.
- Label names are prefixed by the module: `wl_`, `paint_`, `gl_`, `bw_`. Writable space: `a.reserve()` or
  `a.space[...]` (zero-filled, costs nothing in the file). Fixed bytes: `a.data[...]`.
- Each module has `add_all(a)` that adds its routines. Programs that don't use windows must stay byte-for-byte
  the same (only `bitwin` decides to call these).
- Brick texts: 8-byte length, the bytes, a zero byte. A pointer to a text points at the length.
- Whole numbers are signed 64-bit. Colours are `0xRRGGBB` in a 64-bit field.
- Never `git stash`, `git checkout`, `git reset` or commit: the trainer process is writing files in `texts/`
  all the time. Never edit anything in `texts/`. Temporary files go in your scratch folder.
- Tests: plain Python scripts in `tests/` like the others (`python3 tests/test_x.py`, exit code 0 = pass,
  they print `passed: name` lines and `N of M tests passed.`). Run with `BRICK_TRAINER=off BRICK_MEMORY=off`.

## Shared memory labels

| Label | Size | Owner | Fields (8 bytes each) |
|---|---|---|---|
| `gui_target` | 32 | wayland.py (tests may set it) | 0 pixels address, 8 width, 16 height, 24 stride in bytes |
| `gui_clip` | 32 | paint.py | 0 x0, 8 y0, 16 x1, 24 y1 (x1, y1 not included) |
| `gui_scale` | 8 | wayland.py | the screen's scale x 120 (120 = 1.0, 192 = 1.6); set by `wl_open`, 120 when no window is open |

Declare them with `a.space.setdefault("gui_target", 32)` so whichever module comes first makes them.
Real pixels everywhere: `gui_target`, every event coordinate (pointer x, y, scroll position and amount) and
the "resized" width and height are in REAL buffer pixels (the logical size x scale, rounded), so callers never
convert. Only `wl_open`'s width and height (and the window's min/max size) are logical. To keep things the same
size on every screen, multiply logical sizes by `gui_scale / 120`.
Pixels: 4 bytes each, `XRGB8888` (Wayland format 1): the 32-bit little-endian value `0x00RRGGBB`.

## paint.py

All painting goes into `gui_target`, only inside `gui_clip`. Everything may be partly or fully off-screen.
Opacity blending: `out = (src * a + dst * (255 - a)) / 255` per channel (small rounding differences are fine).
`paint.add_all(a, text_sizes=None)`: text_sizes = the text sizes (px) the program uses, so only those letters
are added (None = not known: a default set of about 31 KB). Until a clip is set, the clip is the whole target.
Lines go from pixel centre to pixel centre. Text `size px` is the em size; anything not ASCII 32..126 draws a box.

| Routine | Input | Does |
|---|---|---|
| `paint_clip_all` | nothing | clip = the whole target |
| `paint_clip` | rax = block {x0, y0, x1, y1} | clip = that, cut to the target |
| `paint_box` | rax = block {x, y, w, h, style address} | the box with its style: shadow, fill, texture, border, rounded corners |
| `paint_ellipse` | rax = block {x, y, w, h, style address} | the ellipse inside that box, same style rules (no corners) |
| `paint_text` | rax = block {x, y, text address, colour, size px, transparency} | text, top-left at x, y -> rax = width drawn |
| `paint_measure` | rax = text address, rcx = size px | -> rax = width, rcx = height |
| `paint_line` | rax = block {x1, y1, x2, y2, colour, thickness, transparency} | a line |
| `paint_triangle` | rax = block {x1, y1, x2, y2, x3, y3, colour, transparency} | a filled triangle |

STYLE block, 128 bytes, 8-byte fields. All zeros = nothing drawn (fill 0), fully opaque.

| Offset | Field | Values |
|---|---|---|
| 0 | fill | 0 none, 1 solid colour, 2 linear gradient, 3 radial gradient (centre to edge) |
| 8 | colour | solid colour, or gradient start |
| 16 | colour 2 | gradient end |
| 24 | direction | linear: 0 down (top to bottom), 1 up, 2 right (left to right), 3 left, 4 down-right, 5 up-right |
| 32 | texture | 0 none, 1 stripes (diagonal), 2 horizontal lines, 3 vertical lines, 4 dots, 5 checks, 6 grain (noise from a hash of x, y: same every frame), 7 bricks, 8 grid |
| 40 | texture colour | the marks are painted in this colour over the fill |
| 48 | texture size | px; 0 = 8 |
| 56 | transparency | 0 = solid ... 255 = invisible (applies to the whole element) |
| 64 | corner radius | px |
| 72 | border width | px; 0 = no border |
| 80 | border colour | |
| 88 | shadow | 0 = none, else offset in px of a soft dark shadow below-right |
| 96 | text colour | used by bitwin for the element's label |
| 104 | text size | px; 0 = 16 |
| 112 | text align | 0 centre, 1 left, 2 right (bitwin) |
| 120 | reserved | |

## wayland.py

| Routine | Input | Answer |
|---|---|---|
| `wl_open` | rax = title text, rcx = width, rdx = height, r8 = 1 if resizable else 0 | rax = 1 if the window is open, 0 if there is no Wayland desktop |
| `wl_present` | nothing | shows what is in `gui_target`; afterwards `gui_target` may point at another buffer, whose contents are undefined: redraw everything before the next present |
| `wl_event` | rax = wait in ms (-1 = until something happens, 0 = just check) | rax = kind, rcx, rdx, r8 (below) |
| `wl_close` | nothing | the window goes |

Fixed-size windows (r8 = 0) set min size = max size, so tiling desktops like Hyprland float them at that size.
The compositor may still choose a size: then the buffers change and a "resized" event says so.
`wl_open` waits for the first configure. Pings are answered inside `wl_event` (and inside `wl_present`).
HiDPI: with `wp_fractional_scale_manager_v1` + `wp_viewporter` the buffers are logical size x preferred scale
and the viewport shows them at the logical size; else with wl_surface v6 the whole-number
`preferred_buffer_scale` is used (`set_buffer_scale`); else the scale is 1 (120). When the scale changes after
`wl_open` (the window moved to another screen) the buffers change and a "resized" event gives the new real size.
app_id: "brick" (or the BRICK_APP_ID environment variable, for tests).

Event kinds (rax from `wl_event`):

| rax | Meaning | rcx | rdx | r8 |
|---|---|---|---|---|
| 0 | nothing happened in time | | | |
| 1 | pointer moved | x | y | |
| 2 | button pressed | x | y | 1 left, 2 right, 3 middle |
| 3 | button released | x | y | button |
| 4 | key typed | character (ASCII; 8 backspace, 9 tab, 10 enter, 27 escape; 1001 left, 1002 right, 1003 up, 1004 down, 1005 home, 1006 end, 1007 delete) | 1 if ctrl is held | |
| 5 | resized (size or scale changed) | new width (real px) | new height (real px) | |
| 6 | the person closed the window | | | |
| 7 | scrolled | x | y | amount in real px, at least 1 either way (+ = down) |
| 8 | pointer left the window | | | |

`wl_pointer` (16 bytes): the last pointer x, y (real px). Unused event fields are 0.
`wl_event` with no window open answers 6. `wl_present` waits at most 1 s for a buffer to come back.

## glspace.py

Software 3D. GLVIEW block: first fields fixed so bitwin can place it: 0 x, 8 y, 16 w, 24 h (px, set by bitwin
before each render). The rest glspace defines and documents in its own docstring.

| Routine | Input | Does |
|---|---|---|
| `gl_new_view` | nothing | rax = a new GLVIEW with sensible defaults (camera, light, background) |
| `gl_render` | rax = GLVIEW | draws the view into `gui_target` inside its rect and `gui_clip`, with a depth buffer |
| `gl_animating` (8-byte label) | | non-zero while any view has something moving, so bitwin redraws about 60 times a second |

More routines (`gl_add`, `gl_find`, `gl_find_kind`, `gl_last`, `gl_turn_camera`, `gl_zoom`, `gl_check_animating`), every
field offset, and the English (`glspace.read(words, names)` -> op, `glspace.emit(compiler, op, view_slot)`): see the
docstring and comments in `glspace.py`. `gui_clip` all zeros (never set) counts as the whole target.

## bitwin.py

Elements, their places and sizes in many units, depth (front/back), styles, events, the English, and the
event loop. Built after the parts above, on top of them.

### bitwin runtime (machine code, `bw_` routines)

An element table `bw_elements`: up to 256 elements of 512 bytes, found by index. Element 0 is the window.

ELEMENT record (8-byte fields):

| Offset | Field | Values |
|---|---|---|
| 0 | kind | 1 window, 2 button, 3 canvas, 4 menu, 5 tabs, 6 text, 7 gl view, 8 box (panel), 9 input box |
| 8 | name | text address |
| 16 | parent | element index the position is measured in (0 = the window) |
| 24 | tab page | 0 = always shown; else (tabs element index * 65536 + page number + 1): shown only while that page is picked |
| 32/40 | x value / x unit | units below |
| 48/56 | y value / y unit | |
| 64/72 | w value / w unit | |
| 80/88 | h value / h unit | |
| 96 | other element | for the relative units (below, right of ...) |
| 104 | gap | px gap for the relative units |
| 112/120/128/136 | x, y, w, h | worked out by the layout before each frame, in LOGICAL window px (absolute; the program's own numbers, scaled only when drawn) |
| 144 | depth | bigger = in front. New elements get the next number (later = in front) |
| 152 | flags | 1 hidden, 2 disabled, 4 hovered, 8 pressed, 16 focused, 32 open (menu), 64 has hover style |
| 160 | text | the label, content, or input text (text address) |
| 168 | on click | address of a compiled handler routine, or 0 |
| 176 | items | address of an ITEMS block (menus, tabs), or 0 |
| 184 | picked | the picked item number (tabs: the page shown; menus: last chosen) |
| 192 | data | canvas: its shape list; gl view: its GLVIEW address; input box: the caret (byte position) |
| 200 | on change | handler for input boxes (text changed) and tabs (page changed) |
| 208-255 | for the runtime | 208 shown this frame, 216 flow neighbour (unit 9), 224 hovered tab + 1 |
| 256-383 | STYLE | the 128-byte STYLE block (paint.py) |
| 384-511 | hover STYLE fields | 384 hover colour, 392 hover colour 2, 400 hover text colour (used when flag 64) |

Units (x/y/w/h unit fields):

| Unit | x / y means | w / h means |
|---|---|---|
| 0 px | value px from the parent's left/top | value px |
| 1 percent | value % of the parent's width/height | value % of the parent's width/height |
| 2 from the far edge | value px from the parent's right/bottom edge (to the element's right/bottom edge) | parent size minus value px |
| 3 centred | centred in the parent, then moved by value px | |
| 4 after | right of / below the other element, by gap px, then moved by value px (x: right of, y: below) | |
| 5 before | left of / above the other element, by gap px, then moved by value px | |
| 6 aligned | same x / y as the other element, plus value | same w / h as the other element, plus value |
| 7 fit | | fit the text (plus padding) |
| 8 per mille | value / 10 % (for decimals like 12.5%) | value / 10 % |
| 9 flow | under the element made before it (same parent, same tab page or none): x lines up with it, y = its bottom + 10; under menus/tabs: x = 20, y = their bottom + 16; nothing before: 20, 20 in the parent; then moved by value px | |

The compiler turns real-world units into px or percent before storing them: pt = 4/3 px, mm = 3.78 px,
cm = 37.8 px, in = 96 px, em = the element's text size.

ITEMS block: 8 count, then up to 32 items of 16 bytes: {text address, handler address}.

Shape list (canvas): 8 count, 8 capacity, then shapes of 64 bytes: {kind (1 box, 2 ellipse, 3 line, 4 triangle,
5 text, 6 dot), then the paint block fields for that kind, style address or colour}. Coordinates are relative to
the canvas (logical px; window mouse x/y minus the canvas x/y gives canvas coordinates). Shapes stay until the
canvas is cleared. Exactly (8-byte fields after the kind):

| Kind | Fields |
|---|---|
| 1 box, 2 ellipse | x, y, w, h, style address (0 = a solid fill of the next field), colour |
| 3 line | x1, y1, x2, y2, colour, thickness (0 = 1), transparency |
| 4 triangle | x1, y1, x2, y2, x3, y3, colour; its transparency goes in the kind field: kind = 4 + 256 x transparency |
| 5 text | x, y, text address, colour, size (0 = 16), transparency |
| 6 dot | x, y (its centre), diameter (0 = 6), colour |

| Routine | Input | Does |
|---|---|---|
| `bw_open` | rax = title, rcx = w, rdx = h, r8 = resizable | opens the window (wl_open), makes element 0. No desktop: says so on the console and ends the program |
| `bw_new` | rax = kind, rcx = name text | rax = new element index, with the kind's default style, size and the next depth |
| `bw_elem` | rax = index | rax = the element's record address |
| `bw_find` | rax = name text (capitals don't matter) | rax = index or -1 |
| `bw_set_text` | rax = index, rcx = text | |
| `bw_add_item` | rax = index, rcx = text | rax = item number |
| `bw_item_handler` | rax = index, rcx = item number, rdx = handler | |
| `bw_front` / `bw_back` | rax = index | in front of / behind everything |
| `bw_in_front_of` / `bw_behind` | rax = index, rcx = other index | just in front of / just behind the other |
| `bw_draw` | rax = canvas index, rcx = shape (64 bytes, copied) | adds a shape to a canvas |
| `bw_clear` | rax = canvas index | |
| `bw_changed` | nothing | something changed: redraw before waiting for the next event |
| `bw_run` | nothing | handles events until the window is closed; returns then |
| `bw_wait` | rax = ms | handles events (and runs handlers) for that long |
| `bw_wait_click` | rax = index (-1 = anything) | handles events until that element is clicked |
| `bw_close` | nothing | closes the window (bw_run returns) |

New elements: menus line up along the top (the first at 0 0, the rest right of the menu before); tabs go
across the top (under the menus, w 100%); every other kind flows (unit 9 for x and y). Default sizes: buttons,
menus, texts and tabs fit their text (unit 7), input boxes 220 wide, canvases 300 x 200, boxes 200 x 150,
gl views 300 x 300. Texts wrap within their width when it isn't "fit".

Event labels (8 bytes each): `bw_mouse_x`, `bw_mouse_y`, `bw_key` (last key, contract key codes),
`bw_last` (index of the element of the last event), `bw_chosen` (text of the last chosen menu item / tab).
`bw_mouse_x` / `bw_mouse_y` are logical px (the program's numbers), like every element field.
Window handlers (address or 0): `bw_on_close`, `bw_on_key`, `bw_on_move`, `bw_on_resize`, `bw_every` + `bw_every_ms`.

Handlers are compiled routines (`call`ed with no inputs; they may change any register). Behaviour: hovering
shows the hover style; pressing a button darkens it; a click (press + release on the same element) runs its
handler; only the frontmost element under the pointer gets it. A menu is a button in a menu bar (menus line up at
the top left unless placed); clicking opens its item list in front of everything; choosing runs the item handler.
Tabs: a row of tab buttons; picking one shows its page's elements. Input box: click to focus, typing edits
(backspace, arrows, home, end, delete, enter runs on change). Text: a label, can wrap within its width.
Details: a picked tab runs its item handler (if set), then the tabs' on change. A click on the bare window runs
element 0's on click and counts for `bw_wait_click(-1)`. `bw_wait` with no window open (or after it closed) just
sleeps the rest of the time; `bw_run` / `bw_wait_click` return at once. Handlers may change anything and may wait.

### The English (bitwin words in the compiler)

A program that uses any bitwin word gets the window routines. At the end of the program, if a window is open,
it stays open and handles events until it is closed (an implicit `bw_run`). `wait 2 seconds` while a window is
open keeps it alive (`bw_wait`). Without window words, everything is byte-for-byte as before. "the window" always
means brick's own bitwin window (`make the window 800 by 600` opens it with that size if nothing opened it yet);
the console's own window is "the console" (`put the console on the left`, `make the console 800 by 600`), and
`write` always goes to the console. (The words are in `bitwords.py`; the window opens the first time it's needed.)

Examples it must read (and many more ways of saying them):

```text
open a window called My Game, 800 by 600, in navy
open a resizable window 50% by 50%
make a button called start saying Start at 20 20, 120 by 40, in orange with round corners
add a button saying Quit at the bottom right
put a text called score at the top saying Score: 0, in gold, size 24
make a canvas called board at 20 80, 400 by 300 with a gradient from black to navy going down
make a menu called file with New, Open and Quit
make tabs called pages with Home, Settings and About
put start on the Home tab
make a gl context called view 300 by 300 to the right of board      (also: 3d view, gl view, context)
make an input box called name below start
make a box called panel at 10% 10%, 80% by 80%, with a striped texture in grey, 50% transparent
start is 20 from the right and 20 from the bottom
put start below board with a gap of 10
make start 2 cm wide and 1 cm tall
put start in front of board / put start behind board / bring start to the front / send panel to the back / set the depth of start to 5
make start red / give start a gradient from red to orange / make board checked in grey / give start a 2 px gold border / give panel a shadow / make panel half see-through / round the corners of start by 12 / make the text of start white and size 20
make start turn yellow when the mouse is over it
when start is clicked, set the text of score to "Score: 1"
when start is clicked:            (an indented block below)
when New is chosen from file, clear board
when the Settings tab is picked, write settings
when a key is pressed, write the key
when board is clicked, draw a red dot on board at the mouse x and the mouse y
every 100 milliseconds, move ball right 5
when the window is closed, write bye
draw a circle on board at 50 50, 30 by 30, in red / draw a line on board from 0 0 to 100 100 in white, 3 px thick
draw a triangle on board at 0 0, 50 0 and 25 40 / write hello on board at 10 10 in gold / clear board
move start to 100 100 / move start right 20 / resize start to 200 by 50 / hide start / show start / disable start
set the text of score to "Score: " joined with points / write the text of name / the x of start / the width of the window
wait until start is clicked / close the window
put a red cube in view / spin the cube          (3D words: glspace.read / glspace.emit)
```

### Change from the owner: "window" vs "console"

"window" always means brick's own bitwin window (said without `open a window` first, it opens one).
The terminal is "the console": `put the console on the left`, `make the console 800 by 600`,
`set the title of the console to X`. `write` goes to the console. `brick console game.txt` runs a program in a
console of its own (`brick window` still works as an alias). texts/examples/window.txt is now console.txt.
