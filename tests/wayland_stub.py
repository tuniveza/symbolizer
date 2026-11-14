"""A tiny fake Wayland compositor, for testing brick's own Wayland client without touching the desktop.

Pure Python standard library. It listens on a socket in a fresh temporary XDG_RUNTIME_DIR, serves one
client, keeps every committed frame, and can fake the pointer, the keyboard, resizes, pings and close.

    with Stub() as stub:                       # Stub(size=(w, h)): the compositor picks the window size
        process = subprocess.Popen([program], env=stub.env)
        stub.wait_frames(1, 5)
        stub.pixel(10, 10)                     # -> 0xRRGGBB of the last frame
        stub.click(20, 30)                     # button 1 = left
        stub.type_text("Hi!")
        stub.save_png("/tmp/frame.png")

Everything the client asks for is noted in `stub.log` (and printed to stderr with Stub(log=True) or
WL_STUB_LOG=1). Mistakes the client makes (attaching a buffer the compositor still holds, attaching
before the first configure is acknowledged, unknown objects) go into `stub.errors`.
"""
import array
import mmap
import os
import shutil
import socket
import struct
import tempfile
import threading
import time
import zlib

GLOBALS = [("wl_compositor", 6), ("wl_shm", 2), ("xdg_wm_base", 6), ("wl_seat", 9), ("wp_viewporter", 1),
           ("wp_fractional_scale_manager_v1", 1)]
BUTTONS = {1: 0x110, 2: 0x111, 3: 0x112}

# characters -> (evdev key code, shift), US layout
KEYS = {}
for _first, _plain, _shifted in [(2, "1234567890-=", "!@#$%^&*()_+"), (16, "qwertyuiop[]", "QWERTYUIOP{}"),
                                 (30, "asdfghjkl;'`", 'ASDFGHJKL:"~'), (43, "\\zxcvbnm,./", "|ZXCVBNM<>?")]:
    for _n, (_p, _s) in enumerate(zip(_plain, _shifted)):
        KEYS[_p], KEYS[_s] = (_first + _n, False), (_first + _n, True)
KEYS.update({" ": (57, False), "\n": (28, False), "\t": (15, False), "\b": (14, False), "\x1b": (1, False)})
NAMED_KEYS = {"escape": 1, "backspace": 14, "tab": 15, "enter": 28, "left": 105, "right": 106, "up": 103,
              "down": 108, "home": 102, "end": 107, "delete": 111, "shift": 42, "ctrl": 29}


def fixed(value):
    return int(round(value * 256))


def string(s):
    raw = s.encode() + b"\0"
    return struct.pack("<I", len(raw)) + raw + b"\0" * (-len(raw) % 4)


def wl_array(raw):
    return struct.pack("<I", len(raw)) + raw + b"\0" * (-len(raw) % 4)


class Args:
    """Reads the arguments of one request."""

    def __init__(self, body, fds):
        self.body, self.at, self.fds = body, 0, fds

    def u32(self):
        value, = struct.unpack_from("<I", self.body, self.at)
        self.at += 4
        return value

    def i32(self):
        value, = struct.unpack_from("<i", self.body, self.at)
        self.at += 4
        return value

    def string(self):
        length = self.u32()
        raw = self.body[self.at:self.at + length]
        self.at += (length + 3) & ~3
        return raw[:-1].decode(errors="replace") if length else None

    def array(self):
        length = self.u32()
        raw = self.body[self.at:self.at + length]
        self.at += (length + 3) & ~3
        return raw

    def fd(self):
        if not self.fds:
            raise ValueError("a request needs an fd but none arrived")
        return self.fds.pop(0)


class Obj:
    def __init__(self, interface, version=1, **data):
        self.interface, self.version, self.data = interface, version, data


class Stub:
    def __init__(self, size=None, log=False, release="next", offer=None, seat=True, scale=120, fractional=True,
                 compositor_version=6):
        """size: (w, h) the compositor's configure asks for; None = 0 x 0, the client chooses.
        release: when buffers come back: "next" (when another buffer is committed, like most compositors),
        "now", "never", or a number of seconds after the next commit.
        offer: the globals to announce (default GLOBALS), as (name, version) pairs.
        scale: the screen's scale x 120 (192 = 1.6). With fractional=True it is told through
        wp_fractional_scale_v1 (with wp_viewporter); otherwise, with a wl_compositor of version 6, as a
        whole number through wl_surface.preferred_buffer_scale (rounded up, like compositors do); older
        compositors can't tell it at all."""
        self.size, self.release = size, release
        self.offer = list(offer if offer is not None else GLOBALS)
        if not seat:
            self.offer = [g for g in self.offer if g[0] != "wl_seat"]
        if not fractional:
            self.offer = [g for g in self.offer if not g[0].startswith("wp_")]
        self.offer = [(n, compositor_version if n == "wl_compositor" else v) for n, v in self.offer]
        self.scale = scale
        self.fractional_scale = None          # the client's wp_fractional_scale_v1
        self.viewport = None
        self.destination = None               # wp_viewport.set_destination (w, h)
        self.buffer_scale = 1                 # wl_surface.set_buffer_scale
        self.print_log = log or os.environ.get("WL_STUB_LOG") == "1"
        self.dir = tempfile.mkdtemp(prefix="wlstub-")
        self.display = "wayland-stub"
        self.path = os.path.join(self.dir, self.display)
        self.env = {"XDG_RUNTIME_DIR": self.dir, "WAYLAND_DISPLAY": self.display,
                    "PATH": os.environ.get("PATH", "/usr/bin:/bin")}
        self.log, self.errors, self.frames, self.frame_buffers = [], [], [], []
        self.title = self.app_id = self.min_size = self.max_size = None
        self.versions, self.pongs, self.acks, self.configures = {}, [], [], []
        self.objects = {1: Obj("wl_display")}
        self.serial = 100
        self.held = set()                 # buffers the compositor holds (not released)
        self.surface = self.xdg_surface = self.toplevel = self.pointer = self.keyboard = None
        self.entered = self.keyboard_entered = False
        self.mods = self.locked = 0
        self.acked = False
        self.connected = threading.Event()
        self.disconnected = threading.Event()
        self.changed = threading.Condition()
        self.send_lock = threading.Lock()
        self.client = self.listener = self.thread = None
        self.running = False

    # ---- running ----

    def start(self):
        self.listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.listener.bind(self.path)
        self.listener.listen(1)
        self.listener.settimeout(0.2)
        self.running = True
        self.thread = threading.Thread(target=self._serve, daemon=True)
        self.thread.start()
        return self

    def stop(self):
        self.running = False
        for s in (self.client, self.listener):
            if s:
                try:
                    s.close()
                except OSError:
                    pass
        if self.thread:
            self.thread.join(2)
        shutil.rmtree(self.dir, ignore_errors=True)

    def __enter__(self):
        return self.start()

    def __exit__(self, *problem):
        self.stop()

    def note(self, line):
        self.log.append(line)
        if self.print_log:
            import sys
            print("[wl stub]", line, file=sys.stderr)

    def _serve(self):
        while self.running:
            try:
                self.client, _ = self.listener.accept()
                break
            except socket.timeout:
                continue
            except OSError:
                return
        if not self.running:
            return
        self.connected.set()
        buf, fds = b"", []
        while self.running:
            try:
                data, ancillary, _, _ = self.client.recvmsg(65536, socket.CMSG_SPACE(64 * 4))
            except OSError:
                break
            for level, kind, raw in ancillary:
                if level == socket.SOL_SOCKET and kind == socket.SCM_RIGHTS:
                    got = array.array("i")
                    got.frombytes(raw[:len(raw) - len(raw) % 4])
                    fds.extend(got)
            if not data:
                break
            buf += data
            while len(buf) >= 8:
                obj, word = struct.unpack_from("<II", buf)
                size, opcode = word >> 16, word & 0xFFFF
                if size < 8 or len(buf) < size:
                    if size < 8:
                        self.errors.append(f"message of size {size}")
                        buf = b""
                    break
                body, buf = buf[8:size], buf[size:]
                try:
                    self._request(obj, opcode, Args(body, fds))
                except Exception as problem:            # keep serving; the test sees the error
                    self.errors.append(f"while handling {obj}.{opcode}: {type(problem).__name__}: {problem}")
                with self.changed:
                    self.changed.notify_all()
        self.disconnected.set()
        with self.changed:
            self.changed.notify_all()

    # ---- sending events ----

    def next_serial(self):
        self.serial += 1
        return self.serial

    @staticmethod
    def message(obj, opcode, payload=b""):
        """The bytes of one event (for tests that send odd pieces with send_raw)."""
        return struct.pack("<II", obj, (8 + len(payload)) << 16 | opcode) + payload

    def send_raw(self, raw):
        with self.send_lock:
            self.client.sendall(raw)

    def send(self, obj, opcode, payload=b"", fds=()):
        message = self.message(obj, opcode, payload)
        with self.send_lock:
            if not self.client:
                raise RuntimeError("no client connected")
            if fds:
                self.client.sendmsg([message], [(socket.SOL_SOCKET, socket.SCM_RIGHTS, array.array("i", fds))])
            else:
                self.client.sendall(message)

    def _event(self, obj, opcode, fmt="", *values, tail=b"", fds=()):
        self.send(obj, opcode, struct.pack("<" + fmt, *values) + tail, fds)

    # ---- requests from the client ----

    def _request(self, obj_id, opcode, args):
        obj = self.objects.get(obj_id)
        if obj is None:
            self.errors.append(f"request {opcode} to unknown object {obj_id}")
            self.note(f"? {obj_id}.{opcode}")
            return
        handler = getattr(self, f"_{obj.interface}_{opcode}", None)
        self.note(f"{obj.interface}#{obj_id}.{opcode} {args.body.hex()}")
        if handler:
            handler(obj_id, obj, args)

    def _new(self, args, interface, version=1, **data):
        new_id = args.u32()
        if new_id in self.objects:
            self.errors.append(f"id {new_id} is still in use ({self.objects[new_id].interface})")
        self.objects[new_id] = Obj(interface, version, **data)
        return new_id

    def _destroy(self, obj_id):
        self.objects.pop(obj_id, None)
        self._event(1, 1, "I", obj_id)          # wl_display.delete_id

    # wl_display
    def _wl_display_0(self, _, __, args):                     # sync
        callback = self._new(args, "wl_callback")
        self._event(callback, 0, "I", self.next_serial())
        self._destroy(callback)

    def _wl_display_1(self, _, __, args):                     # get_registry
        registry = self._new(args, "wl_registry")
        for name, (interface, version) in enumerate(self.offer, 1):
            self._event(registry, 0, "I", name, tail=string(interface) + struct.pack("<I", version))

    # wl_registry.bind
    def _wl_registry_0(self, _, __, args):
        name = args.u32()
        interface, version = args.string(), args.u32()
        offered = dict(self.offer)
        if interface not in offered or version > offered[interface] or version < 1:
            self.errors.append(f"bad bind {interface} v{version}")
        new_id = self._new(args, interface, version)
        self.versions[interface] = version
        if interface == "wl_shm":
            for format_ in (0, 1):
                self._event(new_id, 0, "I", format_)
        elif interface == "wl_seat":
            self._event(new_id, 0, "I", 3)                    # pointer + keyboard
            if version >= 2:
                self._event(new_id, 1, tail=string("stub-seat"))

    # wl_compositor
    def _wl_compositor_0(self, _, __, args):
        self.surface = self._new(args, "wl_surface", pending=None, frames=[])
        self._tell_whole_scale()

    def _tell_whole_scale(self):
        if self.surface and self.versions.get("wl_compositor", 0) >= 6:
            self._event(self.surface, 2, "i", -(-self.scale // 120))

    def set_scale(self, scale):
        """The window moved to a screen with another scale (x 120)."""
        self.scale = scale
        if self.fractional_scale:
            self._event(self.fractional_scale, 0, "I", scale)
        self._tell_whole_scale()

    # wp_viewporter, wp_viewport
    def _wp_viewporter_1(self, _, __, args):
        self.viewport = self._new(args, "wp_viewport")
        args.u32()

    def _wp_viewport_1(self, _, __, args):
        self.errors.append("set_source isn't expected")

    def _wp_viewport_2(self, _, __, args):
        self.destination = (args.i32(), args.i32())

    # wp_fractional_scale_manager_v1
    def _wp_fractional_scale_manager_v1_1(self, _, __, args):
        self.fractional_scale = self._new(args, "wp_fractional_scale_v1")
        args.u32()
        self._event(self.fractional_scale, 0, "I", self.scale)

    def _wl_surface_8(self, _, __, args):                     # set_buffer_scale
        self.buffer_scale = args.i32()

    def _wl_compositor_1(self, _, __, args):
        self._new(args, "wl_region")

    def _wl_region_0(self, obj_id, _, __):
        self._destroy(obj_id)

    # wl_surface
    def _wl_surface_0(self, obj_id, _, __):
        self._destroy(obj_id)

    def _wl_surface_1(self, _, obj, args):                    # attach
        buffer = args.u32()
        obj.data["pending"] = buffer or None
        obj.data["attached"] = True

    def _wl_surface_2(self, _, obj, args):                    # damage
        obj.data.setdefault("damage", []).append(tuple(args.i32() for _ in range(4)))

    def _wl_surface_9(self, _, obj, args):                    # damage_buffer
        obj.data.setdefault("damage", []).append(tuple(args.i32() for _ in range(4)))

    def _wl_surface_3(self, _, obj, args):                    # frame
        obj.data["frames"].append(self._new(args, "wl_callback"))

    def _wl_surface_6(self, obj_id, obj, _):                  # commit
        if obj.data.pop("attached", False) and obj.data["pending"]:
            self._show(obj.data["pending"])
        elif self.xdg_surface and not self.configures:
            self._configure(*(self.size or (0, 0)))
        for callback in obj.data["frames"]:
            self._event(callback, 0, "I", int(time.time() * 1000) & 0xFFFFFFFF)
            self._destroy(callback)
        obj.data["frames"] = []

    def _show(self, buffer_id):
        if not self.acked:
            self.errors.append("a buffer was attached before the first configure was acknowledged")
        if buffer_id in self.held:
            self.errors.append(f"buffer {buffer_id} was attached again before it was released")
        buffer = self.objects.get(buffer_id)
        if buffer is None or buffer.interface != "wl_buffer":
            self.errors.append(f"attached {buffer_id}, which is not a buffer")
            return
        d = buffer.data
        if self.viewport is None and (d["width"] % self.buffer_scale or d["height"] % self.buffer_scale):
            self.errors.append(f"buffer {d['width']}x{d['height']} doesn't divide by scale {self.buffer_scale}")
        pool = self.objects[d["pool"]].data
        raw = pool["map"][d["offset"]:d["offset"] + d["stride"] * d["height"]]
        if d["stride"] != d["width"] * 4:
            raw = b"".join(raw[y * d["stride"]:y * d["stride"] + d["width"] * 4] for y in range(d["height"]))
        with self.changed:
            self.frames.append((d["width"], d["height"], bytes(raw)))
            self.frame_buffers.append(buffer_id)
        if self.release == "now":
            self._release(buffer_id)
        elif self.release == "never":
            self.held.add(buffer_id)
        else:
            older = list(self.held)
            self.held.add(buffer_id)
            for old in older:
                if isinstance(self.release, (int, float)) and not isinstance(self.release, bool):
                    threading.Timer(self.release, self._release, (old,)).start()
                else:
                    self._release(old)

    def _release(self, buffer_id):
        if buffer_id in self.held and buffer_id in self.objects:
            self.held.discard(buffer_id)
            try:
                self._event(buffer_id, 0)
            except OSError:
                pass

    # wl_shm
    def _wl_shm_0(self, _, __, args):                         # create_pool
        new_id = args.u32()
        fd, size = args.fd(), args.i32()
        self.objects[new_id] = Obj("wl_shm_pool", fd=fd, size=size, map=mmap.mmap(fd, size, mmap.MAP_SHARED,
                                                                                  mmap.PROT_READ))

    def _wl_shm_pool_0(self, pool_id, _, args):               # create_buffer
        new_id = args.u32()
        offset, width, height, stride, format_ = (args.i32() for _ in range(5))
        if format_ not in (0, 1):
            self.errors.append(f"buffer format {format_}")
        pool = self.objects[pool_id].data
        if offset + stride * height > pool["size"] or stride < width * 4:
            self.errors.append(f"buffer {width}x{height} stride {stride} at {offset} does not fit")
        self.objects[new_id] = Obj("wl_buffer", pool=pool_id, offset=offset, width=width, height=height,
                                   stride=stride, format=format_)

    def _wl_shm_pool_1(self, pool_id, _, __):                 # destroy (the buffers keep the memory)
        self._destroy(pool_id)

    def _wl_shm_pool_2(self, _, obj, args):                   # resize
        size = args.i32()
        obj.data["map"] = mmap.mmap(obj.data["fd"], size, mmap.MAP_SHARED, mmap.PROT_READ)
        obj.data["size"] = size

    def _wl_buffer_0(self, buffer_id, _, __):                 # destroy
        self.held.discard(buffer_id)
        self._destroy(buffer_id)

    # xdg_wm_base
    def _xdg_wm_base_2(self, _, __, args):                    # get_xdg_surface
        self.xdg_surface = self._new(args, "xdg_surface")
        args.u32()

    def _xdg_wm_base_3(self, _, __, args):                    # pong
        self.pongs.append(args.u32())

    def _xdg_surface_1(self, _, __, args):                    # get_toplevel
        self.toplevel = self._new(args, "xdg_toplevel")

    def _xdg_surface_4(self, _, __, args):                    # ack_configure
        serial = args.u32()
        self.acks.append(serial)
        if self.configures and serial == self.configures[-1][0]:
            self.acked = True
        elif not self.configures or serial not in [c[0] for c in self.configures]:
            self.errors.append(f"ack_configure with serial {serial}, which was never sent")

    def _xdg_toplevel_2(self, _, __, args):
        self.title = args.string()

    def _xdg_toplevel_3(self, _, __, args):
        self.app_id = args.string()

    def _xdg_toplevel_7(self, _, __, args):
        self.max_size = (args.i32(), args.i32())

    def _xdg_toplevel_8(self, _, __, args):
        self.min_size = (args.i32(), args.i32())

    def _xdg_toplevel_0(self, obj_id, _, __):
        self._destroy(obj_id)

    def _xdg_surface_0(self, obj_id, _, __):
        self._destroy(obj_id)

    # wl_seat
    def _wl_seat_0(self, _, __, args):
        self.pointer = self._new(args, "wl_pointer", self.versions.get("wl_seat", 1))

    def _wl_seat_1(self, _, __, args):
        self.keyboard = self._new(args, "wl_keyboard", self.versions.get("wl_seat", 1))
        keymap = b"xkb_keymap { (a fake keymap from the stub) };\0"
        fd = os.memfd_create("stub-keymap")
        os.write(fd, keymap)
        self._event(self.keyboard, 0, "III", 1, 0, len(keymap), fds=[fd])   # the fd travels out of band
        os.close(fd)
        if self.objects[self.keyboard].version >= 4:
            self._event(self.keyboard, 5, "ii", 25, 600)

    # ---- what a test can do ----

    def wait_until(self, check, timeout=5.0):
        end = time.time() + timeout
        with self.changed:
            while not check():
                left = end - time.time()
                if left <= 0:
                    return False
                self.changed.wait(min(left, 0.05))
        return True

    def wait_frames(self, n, timeout=5.0):
        if not self.wait_until(lambda: len(self.frames) >= n, timeout):
            raise TimeoutError(f"waited for {n} frames, got {len(self.frames)}")
        return self.frames[n - 1]

    def wait_input(self, timeout=5.0):
        """Until the client has its pointer and keyboard."""
        if not self.wait_until(lambda: self.pointer and self.keyboard, timeout):
            raise TimeoutError("the client never asked for a pointer and a keyboard")

    def wait_closed(self, timeout=5.0):
        return self.disconnected.wait(timeout)

    def pixel(self, x, y, frame=-1):
        width, height, raw = self.frames[frame]
        value, = struct.unpack_from("<I", raw, (y * width + x) * 4)
        return value & 0xFFFFFF

    def save_png(self, path, frame=-1):
        width, height, raw = self.frames[frame]
        rows = bytearray()
        for y in range(height):
            rows.append(0)
            line = raw[y * width * 4:(y + 1) * width * 4]
            for x in range(width):
                b, g, r = line[x * 4:x * 4 + 3]
                rows += bytes((r, g, b))

        def chunk(kind, data):
            return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

        with open(path, "wb") as f:
            f.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
                    + chunk(b"IDAT", zlib.compress(bytes(rows))) + chunk(b"IEND", b""))

    def _pointer_frame(self):
        if self.objects[self.pointer].version >= 5:
            self._event(self.pointer, 5)

    def _ms(self):
        return int(time.monotonic() * 1000) & 0xFFFFFFFF

    def move(self, x, y):
        if not self.entered:
            self._event(self.pointer, 0, "IIii", self.next_serial(), self.surface, fixed(x), fixed(y))
            self.entered = True
        else:
            self._event(self.pointer, 2, "Iii", self._ms(), fixed(x), fixed(y))
        self._pointer_frame()

    def leave(self):
        self._event(self.pointer, 1, "II", self.next_serial(), self.surface)
        self.entered = False
        self._pointer_frame()

    def button(self, button=1, pressed=True):
        self._event(self.pointer, 3, "IIII", self.next_serial(), self._ms(), BUTTONS.get(button, button),
                    1 if pressed else 0)
        self._pointer_frame()

    def click(self, x, y, button=1):
        self.move(x, y)
        self.button(button, True)
        self.button(button, False)

    def scroll(self, amount):
        """amount in pixels, + = down."""
        self._event(self.pointer, 4, "IIi", self._ms(), 0, fixed(amount))
        self._pointer_frame()

    def modifiers(self, mods=None, locked=None):
        """mods: shift 1, ctrl 4. locked: caps lock 2."""
        self.mods = self.mods if mods is None else mods
        self.locked = self.locked if locked is None else locked
        self._event(self.keyboard, 4, "IIIII", self.next_serial(), self.mods, 0, self.locked, 0)

    def key(self, code_or_char, ctrl=False):
        """A key press and release: a character (US layout, shift added when needed), a name like "left",
        or an evdev key code."""
        if not self.keyboard_entered:
            self._event(self.keyboard, 1, "II", self.next_serial(), self.surface, tail=wl_array(b""))
            self.keyboard_entered = True
            self.modifiers(0, self.locked)
        shift = False
        if isinstance(code_or_char, int):
            code = code_or_char
        elif code_or_char in NAMED_KEYS:
            code = NAMED_KEYS[code_or_char]
        else:
            code, shift = KEYS[code_or_char]
        mods = (1 if shift else 0) | (4 if ctrl else 0)
        if mods != self.mods:
            self.modifiers(mods)
        self._event(self.keyboard, 3, "IIII", self.next_serial(), self._ms(), code, 1)
        self._event(self.keyboard, 3, "IIII", self.next_serial(), self._ms(), code, 0)
        if mods:
            self.modifiers(0)

    def type_text(self, s):
        for c in s:
            self.key(c)

    def resize(self, w, h):
        self._configure(w, h)

    def _configure(self, w, h):
        serial = self.next_serial()
        self.configures.append((serial, w, h))
        self._event(self.toplevel, 0, "ii", w, h, tail=wl_array(b""))
        self._event(self.xdg_surface, 0, "I", serial)

    def close(self):
        """The person clicked the window's close button."""
        self._event(self.toplevel, 1)

    def ping(self):
        serial = self.next_serial()
        wm = next(i for i, o in self.objects.items() if o.interface == "xdg_wm_base")
        self._event(wm, 0, "I", serial)
        return serial

    def disconnect(self):
        """The compositor goes away (as if it crashed)."""
        self.client.shutdown(socket.SHUT_RDWR)

    def protocol_error(self, message, code=0):
        self._event(1, 0, "II", 1, code, tail=string(message))
