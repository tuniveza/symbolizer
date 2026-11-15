"""bitwin's Wayland client: a real window, spoken in the Wayland protocol by the program itself.

No libwayland, no libc: a Unix socket, raw system calls, and messages put together byte by byte.
The pixels live in shared memory (memfd + mmap), handed to the compositor with SCM_RIGHTS.

Routines (see docs/bitwin-contract.md):
  wl_open     rax = title text, rcx = width, rdx = height, r8 = 1 if resizable -> rax = 1 open, 0 no desktop
  wl_present  shows gui_target, then points gui_target at the other (free) buffer
  wl_event    rax = wait in ms (-1 forever, 0 just look) -> rax = kind, rcx, rdx, r8
  wl_close    the window goes (the connection is closed)
  wl_pointer  16 bytes: the last pointer x, y
  gui_scale   8 bytes: the screen's scale x 120 (192 = 1.6)

HiDPI: gui_target, pointer positions, scroll amounts and "resized" sizes are all in real buffer pixels
(logical size x scale). The scale comes from wp_fractional_scale_v1 (the buffer is then shown at the logical
size by a wp_viewport), else from wl_surface.preferred_buffer_scale (wl_surface v6, set_buffer_scale),
else it's 1. Only wl_open's width and height (and min/max size) are logical.

How it works inside. Everything lives in one zero-filled block, `wl_state`, and every wl_ routine keeps
rbx pointing at it. Internal routines may change rax, rcx, rdx, rsi, rdi, r8-r11 but keep rbx, r12-r15.
Outgoing requests are put together in an out buffer (wl_begin, wl_put, wl_put_text, wl_end) and sent with
sendmsg (wl_flush, which can attach one fd). Incoming bytes collect in an in buffer (wl_read: poll +
recvmsg; every fd that arrives with them is closed, since the only one a compositor sends us is the
keymap, which we don't need: the keys are mapped for a US layout). wl_dispatch_all takes every complete
message out of the in buffer and wl_handle acts on it; what the program should hear about goes into a
small queue of events (wl_push), which wl_event hands out one at a time (wl_pop).
Every object id we make has a kind in a 256-byte table (wl_new_id), so an incoming event is known by its
object; ids come back for reuse when the compositor says delete_id.
Two buffers in one pool: the program draws into one while the compositor may still hold the other;
wl_present waits (at most a second) for the next buffer's release before handing it out.
"""
from asm import (ABOVE, BELOW, EQUAL, GREATER, LESS, NOT_BELOW, NOT_EQUAL, NOT_GREATER,
                 R8, R9, R10, R11, R12, R13, RAX, RBX, RCX, RDI, RDX, RSI)

MAX_SIDE = 16384           # biggest width or height we make buffers for
WAIT_MS = 5000             # how long wl_open waits for the compositor's answers
RELEASE_MS = 1000          # how long wl_present waits for a buffer to come back

# kinds of objects (the byte in the kinds table)
(K_FREE, K_DISPLAY, K_REGISTRY, K_CALLBACK, K_COMPOSITOR, K_SHM, K_WM, K_SEAT, K_SURFACE, K_XSURF,
 K_TOPLEVEL, K_POINTER, K_KEYBOARD, K_POOL, K_BUF0, K_BUF1, K_VIEWPORTER, K_FRACMGR, K_VIEWPORT, K_FRACSCALE,
 K_DEAD) = range(21)
SCALE_NONE, SCALE_FRACTIONAL, SCALE_WHOLE = 0, 1, 2        # how the scale is known (SCALE_MODE)

# 8-byte fields of wl_state
_names = """FD DEAD OPEN REGISTRY COMPOSITOR SHM WM SEAT SURFACE XSURF TOPLEVEL POINTER KEYBOARD POOL BUF0 BUF1
CALLBACK POOL_ADDR POOL_SIZE BUFSIZE STRIDE W H PEND_W PEND_H CONFIGURED SYNCED FREE0 FREE1 CUR MODS LOCKED
TITLE REQ_W REQ_H RESIZABLE IN_POS IN_LEN OUT_LEN MSG_START Q_HEAD Q_COUNT W_TIMEOUT W_DEADLINE W_FIELD
T_FD T_SENT T0 T_DISPLAY T_APPID TB_TEXT TB_MAX TB_KIND TB_FIELD TB_VER TB_ID
LOG_W LOG_H SCALE SCALE_MODE BUFSCALE VIEWPORTER FRACMGR VIEWPORT FRACSCALE""".split()
for _n, _name in enumerate(_names):
    globals()[_name] = 8 * _n
assert len(_names) * 8 <= 768
KINDS = 768                # 256 bytes: the kind of each object id
MSGH = 1024                # struct msghdr (56 bytes)
IOV = 1088                 # struct iovec (16 bytes)
ADDR = 1152                # struct sockaddr_un (110 bytes)
VERS = 1280                # the version each global was bound at: VERS + its field
RESET_END = 2048           # wl_open and wl_close zero everything before this
CMSG = 2048                # 256 bytes of ancillary data (fds)
PFD = 2304                 # struct pollfd
TS = 2320                  # struct timespec
QUEUE = 4096               # 64 events of 32 bytes: kind, rcx, rdx, r8
QUEUE_LENGTH = 64
OUT = 8192                 # 4096 bytes going out
OUT_FLUSH_AT = 3000
IN = 12288                 # 8192 bytes coming in
IN_SIZE = 8192
STATE_SIZE = IN + IN_SIZE

TITLE_LIMIT = 1000         # bytes of a title or app id we send (a Wayland message is at most 4096)


def add_all(a):
    a.space["wl_state"] = STATE_SIZE
    a.space.setdefault("gui_target", 32)
    a.space.setdefault("gui_scale", 8)
    a.space["wl_pointer"] = 16
    a.data["wl_keys"] = key_table()
    for routine in (open_window, present, event, close, new_id, begin, put, put_text, end, flush, now, read,
                    dispatch_all, handle, try_bind, push, pop, wait, make_buffers, set_target, to_real, scale_to):
        routine(a)


# ---- keys: evdev key codes -> characters, US layout ----

def key_table():
    """112 bytes for plain keys, then 112 with shift. 0 = no character; 0x81.. = 1001.. (arrows etc.)."""
    plain, shifted = [0] * 112, [0] * 112

    def keys(first, normal, shift):
        for n, (c, s) in enumerate(zip(normal, shift)):
            plain[first + n], shifted[first + n] = ord(c), ord(s)

    keys(2, "1234567890-=", "!@#$%^&*()_+")
    keys(16, "qwertyuiop[]", "QWERTYUIOP{}")
    keys(30, "asdfghjkl;'`", 'ASDFGHJKL:"~')
    keys(43, "\\zxcvbnm,./", "|ZXCVBNM<>?")
    keys(55, "*", "*")
    keys(57, " ", " ")
    keys(71, "789-456+1230.", "789-456+1230.")       # the number pad (with num lock on)
    keys(98, "/", "/")
    for code, value in [(1, 27), (14, 8), (15, 9), (28, 10), (96, 10), (105, 0x81), (106, 0x82),
                        (103, 0x83), (108, 0x84), (102, 0x85), (107, 0x86), (111, 0x87)]:
        plain[code] = shifted[code] = value
    return bytes(plain + shifted)


# ---- small helpers for writing the machine code ----

def field_address(a, reg, offset_reg):
    """reg = rbx + offset_reg."""
    a.mov(reg, RBX)
    a.op("add", reg, offset_reg)


def store_n(a, offset, value):
    a.set(RAX, value)
    a.store(RBX, offset, RAX)


def store_n_keep(a, offset, value):
    """Like store_n, but changes r9 instead of rax."""
    a.set(R9, value)
    a.store(RBX, offset, R9)


def request(a, obj, opcode, *args):
    """Code that adds one request to the out buffer. obj: a field holding the object id, or 'display'.
    args: ('f', field) 32 bits of a field; ('n', number); ('m', offset) 32 bits of the incoming message
    at r12 + offset; ('new', kind, field) a new object id, kept in field; ('t', field) a text in field."""
    if obj == "display":
        a.set(RAX, 1)
    else:
        a.load(RAX, RBX, obj)
    a.set(RCX, opcode)
    a.call("wl_begin")
    for arg in args:
        if arg[0] == "f":
            a.load(RAX, RBX, arg[1])
        elif arg[0] == "n":
            a.set(RAX, arg[1])
        elif arg[0] == "m":
            a.load32(RAX, R12, arg[1])
        elif arg[0] == "new":
            a.set(RAX, arg[1])
            a.call("wl_new_id")
            a.store(RBX, arg[2], RAX)
        elif arg[0] == "t":
            a.load(RAX, RBX, arg[1])
            a.call("wl_put_text")
            continue
        a.call("wl_put")
    a.call("wl_end")


def zero_state(a):
    """Zero wl_state up to RESET_END (changes rdi, rsi, r9)."""
    top = a.new_label("wl")
    a.mov(RDI, RBX)
    a.lea(RSI, RBX, RESET_END)
    a.set(R9, 0)
    a.label(top)
    a.store(RDI, 0, R9)
    a.op_n("add", RDI, 8)
    a.op("cmp", RDI, RSI)
    a.jump_if(BELOW, top)


def clamp(a, reg, low, high):
    a.set(R11, low)
    a.op("cmp", reg, R11)
    a.move_if(LESS, reg, R11)
    a.set(R11, high)
    a.op("cmp", reg, R11)
    a.move_if(GREATER, reg, R11)


def mark(a, field, kind):
    """kinds[the id in field] = kind."""
    a.load(RAX, RBX, field)
    a.lea(RSI, RBX, KINDS)
    a.op("add", RSI, RAX)
    a.set(RCX, kind)
    a.store_byte(RSI, 0, RCX)


# ---- the four routines programs call ----

def open_window(a):
    fail, fail_closed, have_display, connect, not_open = (a.new_label("wl") for _ in range(5))
    a.label("wl_open")
    a.address(RBX, "wl_state")
    a.load(RSI, RBX, OPEN)
    a.op("test", RSI, RSI)
    a.jump_if(EQUAL, not_open)
    a.set(RAX, 1)                                  # one window at a time: it's already open
    a.ret()
    a.label(not_open)
    zero_state(a)
    store_n_keep(a, SCALE, 120)
    a.store(RBX, TITLE, RAX)
    for reg, field, default in ((RCX, REQ_W, 640), (RDX, REQ_H, 480)):
        keep = a.new_label("wl")
        a.op_n("cmp", reg, 0)
        a.jump_if(GREATER, keep)
        a.set(reg, default)
        a.label(keep)
        clamp(a, reg, 1, MAX_SIDE)
        a.store(RBX, field, reg)
    a.store(RBX, RESIZABLE, R8)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, have_display)
    a.address(RAX, a.text("brick"))
    a.store(RBX, TITLE, RAX)
    a.label(have_display)

    # where the compositor listens: $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY, or WAYLAND_DISPLAY if it starts with /
    named = a.new_label("wl")
    a.address(RAX, a.text("WAYLAND_DISPLAY="))
    a.call("getenv")
    a.address(RBX, "wl_state")
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, named)
    a.address(RAX, a.text("wayland-0"))
    a.label(named)
    a.store(RBX, T_DISPLAY, RAX)
    a.load_byte(RCX, RAX, 8)
    a.op_n("cmp", RCX, ord("/"))
    a.jump_if(EQUAL, connect)
    a.address(RAX, a.text("XDG_RUNTIME_DIR="))
    a.call("getenv")
    a.address(RBX, "wl_state")
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, fail_closed)
    a.address(RCX, a.text("/"))
    a.call("join")
    a.address(RBX, "wl_state")
    a.load(RCX, RBX, T_DISPLAY)
    a.call("join")
    a.address(RBX, "wl_state")
    a.label(connect)                               # rax = the path
    a.load(RCX, RAX)
    a.op_n("cmp", RCX, 107)
    a.jump_if(ABOVE, fail_closed)
    a.set(RDX, 1)                                  # AF_UNIX
    a.store16(RBX, ADDR, RDX)
    a.lea(RSI, RAX, 8)
    a.lea(RDI, RBX, ADDR + 2)
    a.copy_bytes()
    a.set(RAX, 41)                                 # socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0)
    a.set(RDI, 1)
    a.set(RSI, 0x80001)
    a.set(RDX, 0)
    a.syscall()
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, fail_closed)
    a.store(RBX, FD, RAX)
    a.mov(RDI, RAX)                                # connect
    a.lea(RSI, RBX, ADDR)
    a.set(RDX, 110)
    a.set(RAX, 42)
    a.syscall()
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, fail)

    # the globals: registry, then a sync so we know when they've all been announced
    a.lea(RSI, RBX, KINDS)
    a.set(RCX, K_DISPLAY)
    a.store_byte(RSI, 1, RCX)
    request(a, "display", 1, ("new", K_REGISTRY, REGISTRY))
    request(a, "display", 0, ("new", K_CALLBACK, CALLBACK))
    a.set(RAX, WAIT_MS)
    a.set(RCX, SYNCED)
    a.call("wl_wait")
    a.op_n("cmp", RAX, 1)
    a.jump_if(NOT_EQUAL, fail)
    for needed in (COMPOSITOR, SHM, WM):
        a.load(RAX, RBX, needed)
        a.op("test", RAX, RAX)
        a.jump_if(EQUAL, fail)

    # the window: surface, xdg_surface, toplevel, its title, app id and size limits
    request(a, COMPOSITOR, 0, ("new", K_SURFACE, SURFACE))
    # how we learn the screen's scale: fractional (viewporter + fractional scale), whole (wl_surface v6), none
    whole, scaled = a.new_label("wl"), a.new_label("wl")
    a.load(RAX, RBX, VIEWPORTER)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, whole)
    a.load(RAX, RBX, FRACMGR)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, whole)
    store_n(a, SCALE_MODE, SCALE_FRACTIONAL)
    request(a, VIEWPORTER, 1, ("new", K_VIEWPORT, VIEWPORT), ("f", SURFACE))
    request(a, FRACMGR, 1, ("new", K_FRACSCALE, FRACSCALE), ("f", SURFACE))
    a.jump(scaled)
    a.label(whole)
    a.load(RAX, RBX, VERS + COMPOSITOR)
    a.op_n("cmp", RAX, 6)
    a.jump_if(LESS, scaled)
    store_n(a, SCALE_MODE, SCALE_WHOLE)
    a.label(scaled)
    request(a, WM, 2, ("new", K_XSURF, XSURF), ("f", SURFACE))
    request(a, XSURF, 1, ("new", K_TOPLEVEL, TOPLEVEL))
    request(a, TOPLEVEL, 2, ("t", TITLE))
    app = a.new_label("wl")
    a.address(RAX, a.text("BRICK_APP_ID="))
    a.call("getenv")
    a.address(RBX, "wl_state")
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, app)
    a.address(RAX, a.text("brick"))
    a.label(app)
    a.store(RBX, T_APPID, RAX)
    request(a, TOPLEVEL, 3, ("t", T_APPID))
    resizable = a.new_label("wl")
    a.load(RAX, RBX, RESIZABLE)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, resizable)
    request(a, TOPLEVEL, 8, ("f", REQ_W), ("f", REQ_H))       # set_min_size
    request(a, TOPLEVEL, 7, ("f", REQ_W), ("f", REQ_H))       # set_max_size
    a.label(resizable)
    request(a, SURFACE, 6)                                     # commit, no buffer yet: asks for a configure
    a.set(RAX, WAIT_MS)
    a.set(RCX, CONFIGURED)
    a.call("wl_wait")
    a.op_n("cmp", RAX, 1)
    a.jump_if(NOT_EQUAL, fail)
    store_n(a, SYNCED, 0)
    request(a, "display", 0, ("new", K_CALLBACK, CALLBACK))
    a.set(RAX, WAIT_MS)
    a.set(RCX, SYNCED)
    a.call("wl_wait")
    a.op_n("cmp", RAX, 1)
    a.jump_if(NOT_EQUAL, fail)

    # the size: the compositor's if it chose one, else ours
    own, sized = a.new_label("wl"), a.new_label("wl")
    a.load(RAX, RBX, PEND_W)
    a.load(RCX, RBX, PEND_H)
    a.op_n("cmp", RAX, 0)
    a.jump_if(NOT_GREATER, own)
    a.op_n("cmp", RCX, 0)
    a.jump_if(GREATER, sized)
    a.label(own)
    a.load(RAX, RBX, REQ_W)
    a.load(RCX, RBX, REQ_H)
    a.label(sized)
    a.store(RBX, LOG_W, RAX)
    a.store(RBX, LOG_H, RCX)
    a.call("wl_make_buffers")
    a.set(RAX, -1)
    a.call("wl_flush")
    a.load(RAX, RBX, DEAD)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, fail)
    store_n(a, OPEN, 1)
    a.set(RAX, 1)
    a.ret()

    a.label(fail)
    a.load(RDI, RBX, FD)                           # close the socket
    a.set(RAX, 3)
    a.syscall()
    unmapped = a.new_label("wl")
    a.load(RDI, RBX, POOL_ADDR)
    a.op("test", RDI, RDI)
    a.jump_if(EQUAL, unmapped)
    a.load(RSI, RBX, POOL_SIZE)
    a.set(RAX, 11)
    a.syscall()
    a.label(unmapped)
    a.label(fail_closed)
    zero_state(a)
    store_n_keep(a, SCALE, 120)
    a.call("wl_set_target")
    a.set(RAX, 0)
    a.ret()


def present(a):
    done = a.new_label("wl")
    a.label("wl_present")
    a.address(RBX, "wl_state")
    a.load(RAX, RBX, OPEN)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, done)
    a.load(RAX, RBX, DEAD)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, done)
    a.load(RAX, RBX, CUR)
    a.shift_left(RAX, 3)
    field_address(a, RSI, RAX)
    a.load(RCX, RSI, BUF0)
    a.store(RBX, T0, RCX)
    request(a, SURFACE, 1, ("f", T0), ("n", 0), ("n", 0))                 # attach
    request(a, SURFACE, 2, ("n", 0), ("n", 0), ("n", 0x7FFFFFFF), ("n", 0x7FFFFFFF))   # damage: all
    request(a, SURFACE, 6)                                                # commit
    a.load(RAX, RBX, CUR)
    a.shift_left(RAX, 3)
    field_address(a, RSI, RAX)
    a.set(RCX, 0)
    a.store(RSI, FREE0, RCX)                      # the compositor has it now
    a.load(RAX, RBX, CUR)
    a.op_n("xor", RAX, 1)
    a.store(RBX, CUR, RAX)
    a.set(RAX, -1)
    a.call("wl_flush")
    a.load(RCX, RBX, CUR)                         # wait until the other buffer is free
    a.shift_left(RCX, 3)
    a.op_n("add", RCX, FREE0)
    a.set(RAX, RELEASE_MS)
    a.call("wl_wait")
    a.set(RAX, -1)
    a.call("wl_flush")                            # pongs written while waiting
    a.call("wl_set_target")
    a.label(done)
    a.ret()


def event(a):
    have, nothing, gone, answer = (a.new_label("wl") for _ in range(4))
    a.label("wl_event")
    a.address(RBX, "wl_state")
    a.load(RCX, RBX, OPEN)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, gone)                        # no window: it's as good as closed
    a.load(RCX, RBX, Q_COUNT)
    a.op("test", RCX, RCX)
    a.jump_if(NOT_EQUAL, have)
    a.set(RCX, Q_COUNT)
    a.call("wl_wait")
    a.load(RCX, RBX, Q_COUNT)
    a.op("test", RCX, RCX)
    a.jump_if(NOT_EQUAL, have)
    a.set(RAX, 0)
    a.load(RCX, RBX, DEAD)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, answer)
    a.label(gone)                                 # the compositor went away: closed
    a.set(RAX, 6)
    a.label(answer)
    a.set(RCX, 0)
    a.set(RDX, 0)
    a.set(R8, 0)
    a.ret()
    a.label(have)
    a.set(RAX, -1)
    a.call("wl_flush")
    a.jump("wl_pop")


def close(a):
    done = a.new_label("wl")
    a.label("wl_close")
    a.address(RBX, "wl_state")
    a.load(RAX, RBX, OPEN)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, done)
    a.load(RDI, RBX, FD)                          # closing the connection takes the window with it
    a.set(RAX, 3)
    a.syscall()
    a.load(RDI, RBX, POOL_ADDR)
    a.load(RSI, RBX, POOL_SIZE)
    a.set(RAX, 11)                                # munmap
    a.syscall()
    zero_state(a)
    store_n_keep(a, SCALE, 120)
    a.call("wl_set_target")                       # gui_target: nothing (0 x 0)
    a.label(done)
    a.ret()


# ---- building requests ----

def new_id(a):
    """rax = kind -> rax = a free object id (now of that kind)."""
    top, found = a.new_label("wl"), a.new_label("wl")
    a.label("wl_new_id")
    a.set(RDX, 2)
    a.lea(RSI, RBX, KINDS + 2)
    a.label(top)
    a.load_byte(RCX, RSI)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, found)
    a.op_n("add", RSI, 1)
    a.op_n("add", RDX, 1)
    a.op_n("cmp", RDX, 255)
    a.jump_if(LESS, top)
    store_n(a, DEAD, 1)                           # no ids left (can't happen: they're reused)
    a.label(found)
    a.store_byte(RSI, 0, RAX)
    a.mov(RAX, RDX)
    a.ret()


def begin(a):
    """rax = object id, rcx = opcode: a new message starts in the out buffer."""
    room = a.new_label("wl")
    a.label("wl_begin")
    a.load(RDX, RBX, OUT_LEN)
    a.op_n("cmp", RDX, OUT_FLUSH_AT)
    a.jump_if(LESS, room)
    a.push(RAX)
    a.push(RCX)
    a.set(RAX, -1)
    a.call("wl_flush")
    a.pop(RCX)
    a.pop(RAX)
    a.label(room)
    a.load(RDX, RBX, OUT_LEN)
    a.store(RBX, MSG_START, RDX)
    a.lea(RDI, RBX, OUT)
    a.op("add", RDI, RDX)
    a.store32(RDI, 0, RAX)
    a.store32(RDI, 4, RCX)
    a.op_n("add", RDX, 8)
    a.store(RBX, OUT_LEN, RDX)
    a.ret()


def put(a):
    """rax = a 32-bit argument for the message."""
    a.label("wl_put")
    a.load(RDX, RBX, OUT_LEN)
    a.lea(RDI, RBX, OUT)
    a.op("add", RDI, RDX)
    a.store32(RDI, 0, RAX)
    a.op_n("add", RDX, 4)
    a.store(RBX, OUT_LEN, RDX)
    a.ret()


def put_text(a):
    """rax = a brick text: a Wayland string argument (length with the zero byte, bytes, zero, padding).
    At most TITLE_LIMIT bytes, never cutting a letter in half."""
    short, back = a.new_label("wl"), a.new_label("wl")
    a.label("wl_put_text")
    a.load(RCX, RAX)
    a.op_n("cmp", RCX, TITLE_LIMIT)
    a.jump_if(NOT_GREATER, short)
    a.set(RCX, TITLE_LIMIT)
    a.label(back)                                 # step back over UTF-8 continuation bytes
    a.lea(RSI, RAX, 8)
    a.op("add", RSI, RCX)
    a.load_byte(RDX, RSI)
    a.op_n("and", RDX, 0xC0)
    a.op_n("cmp", RDX, 0x80)
    a.jump_if(NOT_EQUAL, short)
    a.op_n("sub", RCX, 1)
    a.jump_if(NOT_EQUAL, back)
    a.label(short)
    a.lea(RSI, RAX, 8)
    a.load(RDX, RBX, OUT_LEN)
    a.lea(RDI, RBX, OUT)
    a.op("add", RDI, RDX)
    a.lea(R8, RCX, 1)
    a.store32(RDI, 0, R8)
    a.op_n("add", RDI, 4)
    a.push(RCX)
    a.copy_bytes()
    a.set(RAX, 0)
    a.store32(RDI, 0, RAX)                        # the zero byte and the padding
    a.pop(RCX)
    a.op_n("add", RCX, 1 + 3 + 4)
    a.op_n("and", RCX, -4)
    a.load(RDX, RBX, OUT_LEN)
    a.op("add", RDX, RCX)
    a.store(RBX, OUT_LEN, RDX)
    a.ret()


def end(a):
    """The message is complete: its size goes into its header."""
    a.label("wl_end")
    a.load(RDX, RBX, MSG_START)
    a.lea(RDI, RBX, OUT)
    a.op("add", RDI, RDX)
    a.load(RAX, RBX, OUT_LEN)
    a.op("sub", RAX, RDX)
    a.shift_left(RAX, 16)
    a.load32(RCX, RDI, 4)
    a.op("or", RAX, RCX)
    a.store32(RDI, 4, RAX)
    a.ret()


def flush(a):
    """rax = an fd to send along (-1 = none): send everything in the out buffer."""
    top, no_fd, failed, emptied, done = (a.new_label("wl") for _ in range(5))
    a.label("wl_flush")
    a.store(RBX, T_FD, RAX)
    a.load(RCX, RBX, OUT_LEN)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, done)
    a.load(RCX, RBX, DEAD)
    a.op("test", RCX, RCX)
    a.jump_if(NOT_EQUAL, emptied)
    a.set(RCX, 0)
    a.store(RBX, T_SENT, RCX)
    a.label(top)
    a.set(RAX, 0)
    for offset in (0, 8, 32, 40, 48):
        a.store(RBX, MSGH + offset, RAX)
    a.lea(RAX, RBX, IOV)
    a.store(RBX, MSGH + 16, RAX)
    a.set(RAX, 1)
    a.store(RBX, MSGH + 24, RAX)
    a.load(RCX, RBX, T_SENT)
    a.lea(RAX, RBX, OUT)
    a.op("add", RAX, RCX)
    a.store(RBX, IOV, RAX)
    a.load(RAX, RBX, OUT_LEN)
    a.op("sub", RAX, RCX)
    a.store(RBX, IOV + 8, RAX)
    a.load(RAX, RBX, T_FD)
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, no_fd)
    a.lea(RCX, RBX, CMSG)                         # one SCM_RIGHTS message carrying the fd
    a.store(RBX, MSGH + 32, RCX)
    a.set(RDX, 24)
    a.store(RBX, MSGH + 40, RDX)
    a.set(RDX, 20)
    a.store(RCX, 0, RDX)
    a.set(RDX, 1)
    a.store32(RCX, 8, RDX)                        # SOL_SOCKET
    a.store32(RCX, 12, RDX)                       # SCM_RIGHTS
    a.store(RCX, 16, RAX)                         # the fd (and 4 bytes of padding)
    a.label(no_fd)
    a.load(RDI, RBX, FD)
    a.lea(RSI, RBX, MSGH)
    a.set(RDX, 0x4000)                            # MSG_NOSIGNAL
    a.set(RAX, 46)                                # sendmsg
    a.syscall()
    a.op_n("cmp", RAX, -4)                        # interrupted: again
    a.jump_if(EQUAL, top)
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, failed)
    a.set(RCX, -1)
    a.store(RBX, T_FD, RCX)                       # the fd went with the first bytes
    a.load(RCX, RBX, T_SENT)
    a.op("add", RCX, RAX)
    a.store(RBX, T_SENT, RCX)
    a.load(RDX, RBX, OUT_LEN)
    a.op("cmp", RCX, RDX)
    a.jump_if(LESS, top)
    a.jump(emptied)
    a.label(failed)
    store_n(a, DEAD, 1)
    a.label(emptied)
    a.set(RAX, 0)
    a.store(RBX, OUT_LEN, RAX)
    a.label(done)
    a.ret()


# ---- receiving ----

def now(a):
    """-> rax = milliseconds on the monotonic clock."""
    a.label("wl_now")
    a.set(RAX, 228)                               # clock_gettime(CLOCK_MONOTONIC)
    a.set(RDI, 1)
    a.lea(RSI, RBX, TS)
    a.syscall()
    a.load(RAX, RBX, TS + 8)
    a.set(RDX, 0)
    a.set(RCX, 1_000_000)
    a.divide(RCX)
    a.load(RCX, RBX, TS)
    a.multiply_n(RCX, RCX, 1000)
    a.op("add", RAX, RCX)
    a.ret()


def read(a):
    """rax = wait in ms (-1 forever) -> more bytes in the in buffer: rax = 1, 0 nothing in time, -1 gone.
    Every fd that comes with the bytes is closed."""
    readable, gone, quiet, cm_top, cm_next, cm_done, fd_top = (a.new_label("wl") for _ in range(7))
    a.label("wl_read")
    a.mov(R10, RAX)
    a.load(RCX, RBX, IN_POS)                      # move what's left to the front
    a.load(RDX, RBX, IN_LEN)
    a.lea(RSI, RBX, IN)
    a.op("add", RSI, RCX)
    a.lea(RDI, RBX, IN)
    a.op("sub", RDX, RCX)
    a.store(RBX, IN_LEN, RDX)
    a.set(RAX, 0)
    a.store(RBX, IN_POS, RAX)
    a.mov(RCX, RDX)
    a.copy_bytes()
    a.load(RAX, RBX, FD)
    a.store32(RBX, PFD, RAX)
    a.set(RAX, 1)                                 # POLLIN
    a.store32(RBX, PFD + 4, RAX)
    a.lea(RDI, RBX, PFD)
    a.set(RSI, 1)
    a.mov(RDX, R10)
    a.set(RAX, 7)                                 # poll
    a.syscall()
    a.op_n("cmp", RAX, 0)
    a.jump_if(GREATER, readable)
    a.label(quiet)
    a.set(RAX, 0)
    a.ret()
    a.label(readable)
    a.load(RCX, RBX, IN_LEN)
    a.set(RDX, IN_SIZE)
    a.op("sub", RDX, RCX)
    a.jump_if(NOT_GREATER, gone)                  # a message bigger than the whole buffer
    a.lea(RAX, RBX, IN)
    a.op("add", RAX, RCX)
    a.store(RBX, IOV, RAX)
    a.store(RBX, IOV + 8, RDX)
    a.set(RAX, 0)
    for offset in (0, 8, 48):
        a.store(RBX, MSGH + offset, RAX)
    a.lea(RAX, RBX, IOV)
    a.store(RBX, MSGH + 16, RAX)
    a.set(RAX, 1)
    a.store(RBX, MSGH + 24, RAX)
    a.lea(RAX, RBX, CMSG)
    a.store(RBX, MSGH + 32, RAX)
    a.set(RAX, 256)
    a.store(RBX, MSGH + 40, RAX)
    a.load(RDI, RBX, FD)
    a.lea(RSI, RBX, MSGH)
    a.set(RDX, 0x40000000)                        # MSG_CMSG_CLOEXEC
    a.set(RAX, 47)                                # recvmsg
    a.syscall()
    a.op_n("cmp", RAX, -4)                        # interrupted
    a.jump_if(EQUAL, quiet)
    a.op_n("cmp", RAX, -11)                       # nothing after all
    a.jump_if(EQUAL, quiet)
    a.op_n("cmp", RAX, 0)
    a.jump_if(NOT_GREATER, gone)
    a.load(RCX, RBX, IN_LEN)
    a.op("add", RCX, RAX)
    a.store(RBX, IN_LEN, RCX)
    a.lea(R8, RBX, CMSG)                          # close every fd that came along
    a.load(R9, RBX, MSGH + 40)
    a.op("add", R9, R8)
    a.label(cm_top)
    a.lea(RAX, R8, 16)
    a.op("cmp", RAX, R9)
    a.jump_if(ABOVE, cm_done)
    a.load(RCX, R8)
    a.op_n("cmp", RCX, 16)
    a.jump_if(BELOW, cm_done)
    a.load32(RAX, R8, 8)
    a.op_n("cmp", RAX, 1)                         # SOL_SOCKET
    a.jump_if(NOT_EQUAL, cm_next)
    a.load32(RAX, R8, 12)
    a.op_n("cmp", RAX, 1)                         # SCM_RIGHTS
    a.jump_if(NOT_EQUAL, cm_next)
    a.lea(R10, R8, 16)
    a.mov(RDX, R8)
    a.op("add", RDX, RCX)
    a.label(fd_top)
    a.lea(RAX, R10, 4)
    a.op("cmp", RAX, RDX)
    a.jump_if(ABOVE, cm_next)
    a.load32(RDI, R10)
    a.set(RAX, 3)                                 # close
    a.syscall()
    a.op_n("add", R10, 4)
    a.jump(fd_top)
    a.label(cm_next)
    a.load(RCX, R8)
    a.op_n("add", RCX, 7)
    a.op_n("and", RCX, -8)
    a.op("add", R8, RCX)
    a.jump(cm_top)
    a.label(cm_done)
    a.set(RAX, 1)
    a.ret()
    a.label(gone)
    store_n(a, DEAD, 1)
    a.set(RAX, -1)
    a.ret()


def dispatch_all(a):
    """Act on every complete message in the in buffer."""
    top, bad, done = a.new_label("wl"), a.new_label("wl"), a.new_label("wl")
    a.label("wl_dispatch_all")
    a.push(R12)
    a.push(R13)
    a.label(top)
    a.load(RAX, RBX, DEAD)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, done)
    a.load(RCX, RBX, IN_POS)
    a.load(RDX, RBX, IN_LEN)
    a.op("sub", RDX, RCX)
    a.op_n("cmp", RDX, 8)
    a.jump_if(LESS, done)
    a.lea(R12, RBX, IN)
    a.op("add", R12, RCX)
    a.load32(R13, R12, 4)
    a.shift_right(R13, 16)
    a.op_n("cmp", R13, 8)
    a.jump_if(LESS, bad)
    a.mov(RAX, R13)
    a.op_n("and", RAX, 3)
    a.jump_if(NOT_EQUAL, bad)
    a.op("cmp", RDX, R13)
    a.jump_if(LESS, done)                         # the rest hasn't arrived yet
    a.op("add", RCX, R13)
    a.store(RBX, IN_POS, RCX)
    a.call("wl_handle")
    a.jump(top)
    a.label(bad)
    store_n(a, DEAD, 1)
    a.label(done)
    a.pop(R13)
    a.pop(R12)
    a.ret()


def handle(a):
    """r12 = one incoming message (r13 = its size): act on it."""
    labels = {}

    def on(kind, opcode):
        labels[(kind, opcode)] = a.new_label("wl")
        return labels[(kind, opcode)]

    handlers = [(K_DISPLAY, 0), (K_DISPLAY, 1), (K_REGISTRY, 0), (K_CALLBACK, 0), (K_WM, 0), (K_SEAT, 0),
                (K_XSURF, 0), (K_TOPLEVEL, 0), (K_TOPLEVEL, 1), (K_BUF0, 0), (K_BUF1, 0), (K_POINTER, 0),
                (K_POINTER, 1), (K_POINTER, 2), (K_POINTER, 3), (K_POINTER, 4), (K_KEYBOARD, 3),
                (K_KEYBOARD, 4), (K_FRACSCALE, 0), (K_SURFACE, 2)]
    for h in handlers:
        on(*h)
    known, done = a.new_label("wl"), a.new_label("wl")
    a.label("wl_handle")
    a.load32(RAX, R12)
    a.set(RDX, 0)
    a.op_n("cmp", RAX, 256)
    a.jump_if(NOT_BELOW, known)
    a.lea(RSI, RBX, KINDS)
    a.op("add", RSI, RAX)
    a.load_byte(RDX, RSI)
    a.label(known)
    a.shift_left(RDX, 8)
    a.load16(RCX, R12, 4)
    a.op("or", RDX, RCX)
    for (kind, opcode), label in labels.items():
        a.op_n("cmp", RDX, kind << 8 | opcode)
        a.jump_if(EQUAL, label)
    a.label(done)
    a.ret()

    def pointer_event(kind):
        a.address(RSI, "wl_pointer")
        a.load(RCX, RSI, 0)
        a.load(RDX, RSI, 8)
        a.set(RAX, kind)
        a.jump("wl_push")

    # wl_display.error: say it on stderr; the connection is finished
    a.label(labels[(K_DISPLAY, 0)])
    message = "brick: the Wayland desktop refused the window: "
    a.address(RSI, a.text(message))
    a.op_n("add", RSI, 8)
    a.set(RDX, len(message))
    a.set(RDI, 2)
    a.set(RAX, 1)
    a.syscall()
    a.load32(RDX, R12, 16)
    a.op_n("sub", RDX, 1)
    a.lea(RCX, R13, -20)
    a.op("cmp", RDX, RCX)
    a.move_if(GREATER, RDX, RCX)
    a.lea(RSI, R12, 20)
    a.set(RDI, 2)
    a.set(RAX, 1)
    a.syscall()
    a.address(RSI, a.text("\n"))
    a.op_n("add", RSI, 8)
    a.set(RDX, 1)
    a.set(RDI, 2)
    a.set(RAX, 1)
    a.syscall()
    store_n(a, DEAD, 1)
    a.ret()

    # wl_display.delete_id: that id is free again
    a.label(labels[(K_DISPLAY, 1)])
    a.load32(RAX, R12, 8)
    a.op_n("cmp", RAX, 256)
    a.jump_if(NOT_BELOW, done)
    a.lea(RSI, RBX, KINDS)
    a.op("add", RSI, RAX)
    a.set(RCX, K_FREE)
    a.store_byte(RSI, 0, RCX)
    a.ret()

    # wl_registry.global: bind the four we use
    a.label(labels[(K_REGISTRY, 0)])
    for name, version, kind, field in (("wl_compositor", 6, K_COMPOSITOR, COMPOSITOR), ("wl_shm", 1, K_SHM, SHM),
                                       ("xdg_wm_base", 2, K_WM, WM), ("wl_seat", 5, K_SEAT, SEAT),
                                       ("wp_viewporter", 1, K_VIEWPORTER, VIEWPORTER),
                                       ("wp_fractional_scale_manager_v1", 1, K_FRACMGR, FRACMGR)):
        a.address(RAX, a.text(name))
        a.set(RCX, version)
        a.set(RDX, kind)
        a.set(R8, field)
        a.call("wl_try_bind")
    a.ret()

    # wl_callback.done: the sync came back (the compositor deletes the callback itself)
    a.label(labels[(K_CALLBACK, 0)])
    store_n(a, SYNCED, 1)
    a.load32(RAX, R12)
    a.lea(RSI, RBX, KINDS)
    a.op("add", RSI, RAX)
    a.set(RCX, K_DEAD)
    a.store_byte(RSI, 0, RCX)
    a.ret()

    # xdg_wm_base.ping -> pong
    a.label(labels[(K_WM, 0)])
    request(a, WM, 3, ("m", 8))
    a.ret()

    # wl_seat.capabilities: a pointer (1) and a keyboard (2)
    a.label(labels[(K_SEAT, 0)])
    no_pointer = a.new_label("wl")
    a.load32(RAX, R12, 8)
    a.op_n("and", RAX, 1)
    a.jump_if(EQUAL, no_pointer)
    a.load(RAX, RBX, POINTER)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, no_pointer)
    request(a, SEAT, 0, ("new", K_POINTER, POINTER))
    a.label(no_pointer)
    a.load32(RAX, R12, 8)
    a.op_n("and", RAX, 2)
    a.jump_if(EQUAL, done)
    a.load(RAX, RBX, KEYBOARD)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, done)
    request(a, SEAT, 1, ("new", K_KEYBOARD, KEYBOARD))
    a.ret()

    # xdg_surface.configure: ack it; a new size from the compositor means new buffers
    a.label(labels[(K_XSURF, 0)])
    changed = a.new_label("wl")
    request(a, XSURF, 4, ("m", 8))
    store_n(a, CONFIGURED, 1)
    a.load(RAX, RBX, OPEN)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, done)
    a.load(RAX, RBX, PEND_W)
    a.load(RCX, RBX, PEND_H)
    a.op_n("cmp", RAX, 0)
    a.jump_if(NOT_GREATER, done)
    a.op_n("cmp", RCX, 0)
    a.jump_if(NOT_GREATER, done)
    a.load(RDX, RBX, LOG_W)
    a.op("cmp", RAX, RDX)
    a.jump_if(NOT_EQUAL, changed)
    a.load(RDX, RBX, LOG_H)
    a.op("cmp", RCX, RDX)
    a.jump_if(EQUAL, done)
    a.label(changed)
    a.store(RBX, LOG_W, RAX)
    a.store(RBX, LOG_H, RCX)
    a.call("wl_make_buffers")
    a.load(RCX, RBX, W)
    a.load(RDX, RBX, H)
    a.set(RAX, 5)
    a.set(R8, 0)
    a.jump("wl_push")

    # xdg_toplevel.configure: the size the compositor wants (0 = we choose)
    a.label(labels[(K_TOPLEVEL, 0)])
    for offset, field in ((8, PEND_W), (12, PEND_H)):
        a.load32_signed(RAX, R12, offset)
        clamp(a, RAX, 0, MAX_SIDE)
        a.store(RBX, field, RAX)
    a.ret()

    # xdg_toplevel.close
    a.label(labels[(K_TOPLEVEL, 1)])
    a.set(RAX, 6)
    a.set(RCX, 0)
    a.set(RDX, 0)
    a.set(R8, 0)
    a.jump("wl_push")

    # wl_buffer.release
    for kind, field in ((K_BUF0, FREE0), (K_BUF1, FREE1)):
        a.label(labels[(kind, 0)])
        store_n(a, field, 1)
        a.ret()

    # wl_pointer.enter / motion: where it is (24.8 fixed point, logical -> whole real pixels)
    motion = a.new_label("wl")
    a.label(labels[(K_POINTER, 0)])
    a.set(R9, 16)
    a.jump(motion)
    a.label(labels[(K_POINTER, 2)])
    a.set(R9, 12)
    a.label(motion)
    a.mov(RSI, R12)
    a.op("add", RSI, R9)
    a.load32_signed(RAX, RSI, 4)
    a.call("wl_to_real")
    a.store(RBX, T0, RAX)
    a.mov(RSI, R12)
    a.op("add", RSI, R9)
    a.load32_signed(RAX, RSI, 0)
    a.call("wl_to_real")
    a.mov(RCX, RAX)
    a.load(RDX, RBX, T0)
    a.address(RSI, "wl_pointer")
    a.store(RSI, 0, RCX)
    a.store(RSI, 8, RDX)
    a.set(RAX, 1)
    a.set(R8, 0)
    a.jump("wl_push")

    # wl_pointer.leave
    a.label(labels[(K_POINTER, 1)])
    a.set(R8, 0)
    pointer_event(8)

    # wl_pointer.button: BTN_LEFT 0x110 -> 1, right 0x111 -> 2, middle 0x112 -> 3; pressed 2, released 3
    a.label(labels[(K_POINTER, 3)])
    a.load32(R8, R12, 16)
    a.op_n("sub", R8, 0x10F)
    a.load32(RAX, R12, 20)
    a.set(R9, 3)
    a.op("sub", R9, RAX)
    a.address(RSI, "wl_pointer")
    a.load(RCX, RSI, 0)
    a.load(RDX, RSI, 8)
    a.mov(RAX, R9)
    a.jump("wl_push")

    # wl_pointer.axis: vertical scrolling only, in real pixels (at least 1 either way)
    a.label(labels[(K_POINTER, 4)])
    some = a.new_label("wl")
    a.load32(RAX, R12, 12)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, done)
    a.load32_signed(RAX, R12, 16)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, done)
    a.call("wl_to_real")
    a.mov(R8, RAX)
    a.op("test", R8, R8)
    a.jump_if(NOT_EQUAL, some)
    a.set(R8, 1)
    a.load32_signed(RAX, R12, 16)
    a.op("test", RAX, RAX)
    a.jump_if(GREATER, some)
    a.set(R8, -1)
    a.label(some)
    pointer_event(7)

    # wp_fractional_scale_v1.preferred_scale (x 120)
    a.label(labels[(K_FRACSCALE, 0)])
    a.load32(RAX, R12, 8)
    a.jump("wl_scale_to")

    # wl_surface.preferred_buffer_scale (a whole number): used when there's no fractional scale
    a.label(labels[(K_SURFACE, 2)])
    a.load(RAX, RBX, SCALE_MODE)
    a.op_n("cmp", RAX, SCALE_WHOLE)
    a.jump_if(NOT_EQUAL, done)
    a.load32_signed(RAX, R12, 8)
    a.multiply_n(RAX, RAX, 120)
    a.jump("wl_scale_to")

    # wl_keyboard.key: a key went down -> its character
    a.label(labels[(K_KEYBOARD, 3)])
    plain, no_caps, small = a.new_label("wl"), a.new_label("wl"), a.new_label("wl")
    a.load32(RAX, R12, 20)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, done)                        # released
    a.load32(RAX, R12, 16)
    a.op_n("cmp", RAX, 112)
    a.jump_if(NOT_BELOW, done)
    a.load(RCX, RBX, MODS)
    a.op_n("and", RCX, 1)                         # shift
    a.jump_if(EQUAL, plain)
    a.op_n("add", RAX, 112)
    a.label(plain)
    a.address(RSI, "wl_keys")
    a.op("add", RSI, RAX)
    a.load_byte(RAX, RSI)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, done)
    a.load(RCX, RBX, LOCKED)
    a.op_n("and", RCX, 2)                         # caps lock flips letters
    a.jump_if(EQUAL, no_caps)
    a.mov(RCX, RAX)
    a.op_n("or", RCX, 0x20)
    a.op_n("sub", RCX, ord("a"))
    a.op_n("cmp", RCX, 25)
    a.jump_if(ABOVE, no_caps)
    a.op_n("xor", RAX, 0x20)
    a.label(no_caps)
    a.op_n("cmp", RAX, 0x80)
    a.jump_if(BELOW, small)
    a.op_n("add", RAX, 1000 - 0x80)
    a.label(small)
    a.mov(RCX, RAX)
    a.load(RDX, RBX, MODS)
    a.shift_right(RDX, 2)                         # control
    a.op_n("and", RDX, 1)
    a.set(RAX, 4)
    a.set(R8, 0)
    a.jump("wl_push")

    # wl_keyboard.modifiers: depressed | latched (shift 1, control 4), locked (caps lock 2)
    a.label(labels[(K_KEYBOARD, 4)])
    a.load32(RAX, R12, 12)
    a.load32(RCX, R12, 16)
    a.op("or", RAX, RCX)
    a.store(RBX, MODS, RAX)
    a.load32(RAX, R12, 20)
    a.store(RBX, LOCKED, RAX)
    a.ret()


def try_bind(a):
    """r12 = a wl_registry.global message; rax = interface name (text), rcx = highest version we speak,
    rdx = kind, r8 = field for the id. If it's that interface and not bound yet: bind it."""
    no = a.new_label("wl")
    a.label("wl_try_bind")
    a.load32(R9, R12, 12)                         # the name's length, with its zero byte
    a.load(R10, RAX)
    a.lea(R11, R10, 1)
    a.op("cmp", R9, R11)
    a.jump_if(NOT_EQUAL, no)
    field_address(a, RSI, R8)
    a.load(R11, RSI)
    a.op("test", R11, R11)
    a.jump_if(NOT_EQUAL, no)
    a.store(RBX, TB_TEXT, RAX)
    a.store(RBX, TB_MAX, RCX)
    a.store(RBX, TB_KIND, RDX)
    a.store(RBX, TB_FIELD, R8)
    a.lea(RSI, R12, 16)
    a.lea(RDI, RAX, 8)
    a.mov(RCX, R10)
    a.compare_bytes()
    a.jump_if(NOT_EQUAL, no)
    a.lea(RAX, R9, 3)                             # the version follows the padded name
    a.op_n("and", RAX, -4)
    a.op("add", RAX, R12)
    a.load32(RAX, RAX, 16)
    a.load(RCX, RBX, TB_MAX)
    a.op("cmp", RAX, RCX)
    a.move_if(ABOVE, RAX, RCX)
    a.store(RBX, TB_VER, RAX)
    a.load(RAX, RBX, TB_KIND)
    a.call("wl_new_id")
    a.store(RBX, TB_ID, RAX)
    a.load(R8, RBX, TB_FIELD)
    field_address(a, RSI, R8)
    a.store(RSI, 0, RAX)
    a.load(RCX, RBX, TB_VER)
    a.store(RSI, VERS, RCX)
    request(a, REGISTRY, 0, ("m", 8), ("t", TB_TEXT), ("f", TB_VER), ("f", TB_ID))
    a.label(no)
    a.ret()


def push(a):
    """rax = kind, rcx, rdx, r8: an event for the program (a move right after a move replaces it)."""
    fresh, write, done = a.new_label("wl"), a.new_label("wl"), a.new_label("wl")
    a.label("wl_push")
    a.load(R9, RBX, Q_COUNT)
    a.op_n("cmp", RAX, 1)
    a.jump_if(NOT_EQUAL, fresh)
    a.op("test", R9, R9)
    a.jump_if(EQUAL, fresh)
    a.load(R10, RBX, Q_HEAD)
    a.op("add", R10, R9)
    a.op_n("sub", R10, 1)
    a.op_n("and", R10, QUEUE_LENGTH - 1)
    a.shift_left(R10, 5)
    a.lea(R11, RBX, QUEUE)
    a.op("add", R11, R10)
    a.load(R10, R11)
    a.op_n("cmp", R10, 1)
    a.jump_if(EQUAL, write)
    a.label(fresh)
    a.op_n("cmp", R9, QUEUE_LENGTH)
    a.jump_if(NOT_BELOW, done)                    # full: this one is lost
    a.load(R10, RBX, Q_HEAD)
    a.op("add", R10, R9)
    a.op_n("and", R10, QUEUE_LENGTH - 1)
    a.shift_left(R10, 5)
    a.lea(R11, RBX, QUEUE)
    a.op("add", R11, R10)
    a.op_n("add", R9, 1)
    a.store(RBX, Q_COUNT, R9)
    a.label(write)
    a.store(R11, 0, RAX)
    a.store(R11, 8, RCX)
    a.store(R11, 16, RDX)
    a.store(R11, 24, R8)
    a.label(done)
    a.ret()


def pop(a):
    """-> rax = kind, rcx, rdx, r8 of the oldest event (the queue must not be empty)."""
    a.label("wl_pop")
    a.load(R10, RBX, Q_HEAD)
    a.mov(R9, R10)
    a.shift_left(R10, 5)
    a.lea(R11, RBX, QUEUE)
    a.op("add", R11, R10)
    a.op_n("add", R9, 1)
    a.op_n("and", R9, QUEUE_LENGTH - 1)
    a.store(RBX, Q_HEAD, R9)
    a.load(R9, RBX, Q_COUNT)
    a.op_n("sub", R9, 1)
    a.store(RBX, Q_COUNT, R9)
    a.load(RAX, R11, 0)
    a.load(RCX, R11, 8)
    a.load(RDX, R11, 16)
    a.load(R8, R11, 24)
    a.ret()


def wait(a):
    """rax = wait in ms (-1 forever), rcx = a field: handle messages until that field isn't 0.
    -> rax = 1 it isn't, 0 time ran out, -1 the connection is gone."""
    top, waiting, yes, gone, no_deadline = (a.new_label("wl") for _ in range(5))
    a.label("wl_wait")
    a.store(RBX, W_TIMEOUT, RAX)
    a.store(RBX, W_FIELD, RCX)
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, no_deadline)
    a.call("wl_now")
    a.load(RCX, RBX, W_TIMEOUT)
    a.op("add", RAX, RCX)
    a.store(RBX, W_DEADLINE, RAX)
    a.label(no_deadline)
    a.label(top)
    a.call("wl_dispatch_all")
    a.load(RCX, RBX, W_FIELD)
    field_address(a, RSI, RCX)
    a.load(RAX, RSI)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, yes)
    a.load(RAX, RBX, DEAD)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, gone)
    a.set(RAX, -1)
    a.call("wl_flush")
    a.load(RAX, RBX, DEAD)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, gone)
    a.load(RAX, RBX, W_TIMEOUT)
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, waiting)
    a.call("wl_now")
    a.mov(RCX, RAX)
    a.load(RAX, RBX, W_DEADLINE)
    a.op("sub", RAX, RCX)
    a.set(RCX, 0)
    a.op("cmp", RAX, RCX)
    a.move_if(LESS, RAX, RCX)
    a.label(waiting)
    a.call("wl_read")
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, gone)
    a.jump_if(GREATER, top)
    a.load(RCX, RBX, W_TIMEOUT)                   # nothing in time (or a signal while waiting forever)
    a.op_n("cmp", RCX, 0)
    a.jump_if(LESS, top)
    a.set(RAX, 0)
    a.ret()
    a.label(yes)
    a.set(RAX, 1)
    a.ret()
    a.label(gone)
    a.set(RAX, -1)
    a.ret()


def make_buffers(a):
    """New shared memory and two buffers of W x H (the old ones go)."""
    fresh, failed = a.new_label("wl"), a.new_label("wl")
    a.label("wl_make_buffers")
    a.load(RAX, RBX, POOL)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, fresh)
    request(a, BUF0, 0)                           # wl_buffer.destroy
    request(a, BUF1, 0)
    request(a, POOL, 1)                           # wl_shm_pool.destroy
    for field in (BUF0, BUF1, POOL):
        mark(a, field, K_DEAD)
    a.load(RDI, RBX, POOL_ADDR)
    a.load(RSI, RBX, POOL_SIZE)
    a.set(RAX, 11)                                # munmap
    a.syscall()
    a.label(fresh)
    for logical, real in ((LOG_W, W), (LOG_H, H)):       # real = logical x scale / 120, rounded
        a.load(RAX, RBX, logical)
        a.load(RCX, RBX, SCALE)
        a.multiply(RAX, RCX)
        a.op_n("add", RAX, 60)
        a.set(RDX, 0)
        a.set(RCX, 120)
        a.divide(RCX)
        clamp(a, RAX, 1, MAX_SIDE)
        a.store(RBX, real, RAX)
    fractional, told = a.new_label("wl"), a.new_label("wl")
    a.load(RAX, RBX, SCALE_MODE)
    a.op_n("cmp", RAX, SCALE_FRACTIONAL)
    a.jump_if(EQUAL, fractional)
    a.op_n("cmp", RAX, SCALE_WHOLE)
    a.jump_if(NOT_EQUAL, told)
    a.load(RAX, RBX, SCALE)                       # whole scale: wl_surface.set_buffer_scale
    a.set(RDX, 0)
    a.set(RCX, 120)
    a.divide(RCX)
    a.store(RBX, BUFSCALE, RAX)
    request(a, SURFACE, 8, ("f", BUFSCALE))
    a.jump(told)
    a.label(fractional)                           # fractional: the viewport shows it at the logical size
    request(a, VIEWPORT, 2, ("f", LOG_W), ("f", LOG_H))
    a.label(told)
    a.load(RAX, RBX, W)
    a.shift_left(RAX, 2)
    a.store(RBX, STRIDE, RAX)
    a.load(RCX, RBX, H)
    a.multiply(RAX, RCX)
    a.store(RBX, BUFSIZE, RAX)
    a.shift_left(RAX, 1)
    a.store(RBX, POOL_SIZE, RAX)
    a.address(RDI, a.text("brick"))               # memfd_create("brick", MFD_CLOEXEC)
    a.op_n("add", RDI, 8)
    a.set(RSI, 1)
    a.set(RAX, 319)
    a.syscall()
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, failed)
    a.store(RBX, T0, RAX)
    a.mov(RDI, RAX)                               # ftruncate
    a.load(RSI, RBX, POOL_SIZE)
    a.set(RAX, 77)
    a.syscall()
    a.op_n("cmp", RAX, 0)
    a.jump_if(LESS, failed)
    a.set(RDI, 0)                                 # mmap(0, size, read | write, MAP_SHARED, fd, 0)
    a.load(RSI, RBX, POOL_SIZE)
    a.set(RDX, 3)
    a.set(R10, 1)
    a.load(R8, RBX, T0)
    a.set(R9, 0)
    a.set(RAX, 9)
    a.syscall()
    a.set(RCX, -4096)
    a.op("cmp", RAX, RCX)
    a.jump_if(ABOVE, failed)
    a.store(RBX, POOL_ADDR, RAX)
    request(a, SHM, 0, ("new", K_POOL, POOL), ("f", POOL_SIZE))           # create_pool, the fd goes along
    a.load(RAX, RBX, T0)
    a.call("wl_flush")
    a.load(RDI, RBX, T0)                          # the compositor has its own copy of the fd now
    a.set(RAX, 3)
    a.syscall()
    request(a, POOL, 0, ("new", K_BUF0, BUF0), ("n", 0), ("f", W), ("f", H), ("f", STRIDE), ("n", 1))
    request(a, POOL, 0, ("new", K_BUF1, BUF1), ("f", BUFSIZE), ("f", W), ("f", H), ("f", STRIDE), ("n", 1))
    store_n(a, FREE0, 1)
    store_n(a, FREE1, 1)
    store_n(a, CUR, 0)
    a.jump("wl_set_target")
    a.label(failed)
    store_n(a, DEAD, 1)
    for field in (POOL, POOL_ADDR, W, H, STRIDE, BUFSIZE):
        store_n(a, field, 0)
    a.jump("wl_set_target")


def set_target(a):
    """gui_target = the current buffer."""
    a.label("wl_set_target")
    a.load(RAX, RBX, CUR)
    a.load(RCX, RBX, BUFSIZE)
    a.multiply(RAX, RCX)
    a.load(RCX, RBX, POOL_ADDR)
    a.op("add", RAX, RCX)
    a.address(RDI, "gui_target")
    a.store(RDI, 0, RAX)
    a.load(RAX, RBX, W)
    a.store(RDI, 8, RAX)
    a.load(RAX, RBX, H)
    a.store(RDI, 16, RAX)
    a.load(RAX, RBX, STRIDE)
    a.store(RDI, 24, RAX)
    a.load(RAX, RBX, SCALE)
    a.address(RDI, "gui_scale")
    a.store(RDI, 0, RAX)
    a.ret()


def to_real(a):
    """rax = a logical coordinate in 24.8 fixed point -> rax = whole real pixels (changes rcx, rdx)."""
    a.label("wl_to_real")
    a.load(RCX, RBX, SCALE)
    a.multiply(RAX, RCX)
    a.sign_extend()
    a.set(RCX, 256 * 120)
    a.divide(RCX, signed=True)
    a.ret()


def scale_to(a):
    """rax = the compositor's preferred scale x 120. A new scale on an open window means new buffers
    and a "resized" event with the new real size."""
    done = a.new_label("wl")
    a.label("wl_scale_to")
    a.op_n("cmp", RAX, 0)
    a.jump_if(NOT_GREATER, done)
    a.load(RCX, RBX, SCALE)
    a.op("cmp", RAX, RCX)
    a.jump_if(EQUAL, done)
    a.store(RBX, SCALE, RAX)
    a.load(RAX, RBX, OPEN)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, done)
    a.call("wl_make_buffers")
    a.load(RCX, RBX, W)
    a.load(RDX, RBX, H)
    a.set(RAX, 5)
    a.set(R8, 0)
    a.jump("wl_push")
    a.label(done)
    a.ret()
