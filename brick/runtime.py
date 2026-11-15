"""Built-in bricks: small machine-code routines the program calls.

How they're called: inputs in rax, rcx, rdx; the answer comes back in rax.
A routine may change any register except rsp.

A text is stored as: 8-byte length, the letters, a zero byte.
A list is stored as: 8-byte count, then up to LIST_LIMIT 8-byte items.
New texts come from the heap and are never reused, so they start as zeros.
"""
import screen
from asm import (ABOVE, BELOW, EQUAL, GREATER, LESS, NOT_ABOVE, NOT_EQUAL, NOT_GREATER, NOT_LESS, NOT_NEGATIVE,
                 R8, R9, R10, R11, R12, R13, R14, R15, RAX, RBX, RCX, RDI, RDX, RSI, RSP, XMM0, XMM1)

TEXT_LIMIT = 256          # longest typed answer
LIST_LIMIT = 1000         # most items in a list
LIST_SIZE = 8 + LIST_LIMIT * 8
SYMBOL_LIMIT = 2000       # most symbols the brain can hold (links between them are unlimited)
LIKENESS_NEEDED = 50      # "is like" means at least half alike, until the program changes it (same-kind pairs
                          # with mostly shared links score about 50-70; different kinds 0-25)
HEAP_SIZE = 64 * 1024 * 1024
FILE_LIMIT = 65536        # biggest file we read
BILLION = 1_000_000_000
RESET = "\x1b[0m"           # back to normal colours


def add_all(a):
    a.space["heap"] = HEAP_SIZE
    a.space["heap_end"] = 16      # right after the heap, so its address is where the heap stops
    for brick in [alloc, new_text, write_text, join, number_to_text, decimal_to_text,
                  text_to_number, same_letters, text_equal, match_at, contains, starts_with, ends_with,
                  change_case("upper", ord("a"), -32), change_case("lower", ord("A"), 32),
                  letter, split, letters, reverse_text, reverse_list, read_line, read_number, wait_key, sleep, random, now, elapsed, time_text,
                  push, push_front, remove_first, remove_last, item, list_has, list_to_text, list_copy,
                  file_save, file_load, file_delete]:
        brick(a)
    screen.add_all(a)


def say_fixed(a, s):
    a.address(RAX, a.text(s))
    a.call("write_text")


def start(a):
    """Runs first: note where the environment is, and point the heap at its start."""
    screen.remember_environment(a)
    a.address(RAX, "heap")
    a.address(RCX, "heap_next")
    a.store(RCX, 0, RAX)
    a.space["heap_next"] = 8


def finish(a, code=0):
    """EXIT"""
    a.set(RAX, 60)
    a.set(RDI, code)
    a.syscall()


# ---- memory and texts ----

def alloc(a):
    """rcx = bytes wanted -> rax = where they are."""
    a.label("alloc")
    a.op_n("add", RCX, 7)
    a.op_n("and", RCX, -8)
    a.address(RDX, "heap_next")
    a.load(RAX, RDX)
    a.mov(R8, RAX)
    a.op("add", R8, RCX)
    a.address(R9, "heap_end")
    a.op("cmp", R8, R9)
    a.jump_if(ABOVE, "out_of_memory")
    a.store(RDX, 0, R8)
    a.ret()
    a.label("out_of_memory")
    say_fixed(a, "\nOut of memory: the program made too many texts.\n")
    finish(a, 1)


def new_text(a):
    """rcx = length -> rax = a new, empty text of that length."""
    a.label("new_text")
    a.push(RCX)
    a.op_n("add", RCX, 9)
    a.call("alloc")
    a.pop(RCX)
    a.store(RAX, 0, RCX)
    a.ret()


def write_text(a):
    """rax = text -> shown on screen."""
    a.label("write_text")
    a.lea(RSI, RAX, 8)
    a.load(RDX, RAX)
    a.set(RAX, 1)
    a.set(RDI, 1)
    a.syscall()
    a.ret()


def join(a):
    """rax = first text, rcx = second text -> rax = both, one after the other."""
    a.label("join")
    a.push(RAX)
    a.push(RCX)
    a.load(RDX, RAX)
    a.load(R8, RCX)
    a.op("add", RDX, R8)
    a.mov(RCX, RDX)
    a.call("new_text")
    a.pop(R9)
    a.pop(R8)
    a.lea(RDI, RAX, 8)
    a.lea(RSI, R8, 8)
    a.load(RCX, R8)
    a.copy_bytes()
    a.lea(RSI, R9, 8)
    a.load(RCX, R9)
    a.copy_bytes()
    a.ret()


def number_to_text(a):
    """rax = whole number -> rax = it written as a text."""
    buffer = a.reserve(32, "digits")
    a.label("number_to_text")
    a.mov(R8, RAX)
    a.address(R9, buffer)
    a.op_n("add", R9, 32)
    a.set(R10, 0)
    a.op("test", R8, R8)
    a.jump_if(NOT_LESS, "ntt_digits")
    a.negate(R8)
    a.set(R10, 1)
    a.label("ntt_digits")
    a.set(RCX, 10)
    a.mov(RAX, R8)
    a.set(RDX, 0)
    a.divide(RCX)
    a.mov(R8, RAX)
    a.op_n("add", RDX, ord("0"))
    a.op_n("sub", R9, 1)
    a.store_byte(R9, 0, RDX)
    a.op("test", R8, R8)
    a.jump_if(NOT_EQUAL, "ntt_digits")
    a.op("test", R10, R10)
    a.jump_if(EQUAL, "ntt_copy")
    a.op_n("sub", R9, 1)
    a.set(RDX, ord("-"))
    a.store_byte(R9, 0, RDX)
    a.label("ntt_copy")
    a.address(RCX, buffer)
    a.op_n("add", RCX, 32)
    a.op("sub", RCX, R9)
    a.push(R9)
    a.call("new_text")
    a.pop(RSI)
    a.load(RCX, RAX)
    a.lea(RDI, RAX, 8)
    a.copy_bytes()
    a.ret()


def decimal_to_text(a):
    """rax = decimal, rcx = places after the point (-1: none if it's whole, else 2) -> rax = text."""
    v = a.reserve(48, "decimal_vars")    # 0 number, 8 places, 16 negative?, 24 scale, 32 scaled, 40 text
    half = a.number(0.5)
    a.label("decimal_to_text")
    a.mov(R8, RAX)                       # infinity (from dividing by 0) and "not a number" first
    a.shift_left(R8, 1)
    a.shift_right(R8, 1)
    a.set(R9, 0x7FF0000000000000)
    a.op("cmp", R8, R9)
    a.jump_if(BELOW, "dtt_normal")
    a.jump_if(ABOVE, "dtt_nan")
    a.op("test", RAX, RAX)
    a.address(RAX, a.text("infinity"))
    a.jump_if(NOT_NEGATIVE, "dtt_end")
    a.address(RAX, a.text("minus infinity"))
    a.ret()
    a.label("dtt_nan")
    a.address(RAX, a.text("not a number"))
    a.label("dtt_end")
    a.ret()
    a.label("dtt_normal")
    a.address(R11, v)
    a.mov(R8, RAX)
    a.shift_right(R8, 63)
    a.store(R11, 16, R8)
    a.shift_left(RAX, 1)                 # drop the sign
    a.shift_right(RAX, 1)
    a.store(R11, 0, RAX)
    a.op_n("cmp", RCX, -1)
    a.jump_if(NOT_EQUAL, "dtt_places")
    a.to_xmm(XMM0, RAX)
    a.round_decimal(XMM1, XMM0, 3)
    a.compare_decimals(XMM0, XMM1)
    a.set(RCX, 0)
    a.jump_if(EQUAL, "dtt_places")
    a.set(RCX, 2)
    a.label("dtt_places")
    a.store(R11, 8, RCX)
    a.set(RAX, 1)                        # scale = 10 x 10 x ... (places times)
    a.set(RDX, 10)
    a.label("dtt_scale")
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "dtt_scaled")
    a.multiply(RAX, RDX)
    a.op_n("sub", RCX, 1)
    a.jump("dtt_scale")
    a.label("dtt_scaled")
    a.store(R11, 24, RAX)
    a.whole_to_decimal(XMM1, RAX)        # scaled = number x scale + 0.5, point dropped
    a.load(RAX, R11, 0)
    a.to_xmm(XMM0, RAX)
    a.decimal_maths("mul", XMM0, XMM1)
    a.address(RAX, half)
    a.load(RAX, RAX)
    a.to_xmm(XMM1, RAX)
    a.decimal_maths("add", XMM0, XMM1)
    a.decimal_to_whole(RAX, XMM0)
    a.store(R11, 32, RAX)
    a.load(RCX, R11, 24)                 # whole part and what's after the point
    a.set(RDX, 0)
    a.divide(RCX)
    a.push(RDX)
    a.call("number_to_text")
    a.pop(RDX)
    a.address(R11, v)
    a.store(R11, 40, RAX)
    a.load(R8, R11, 16)                  # put a minus sign in front, unless it rounds to zero
    a.op("test", R8, R8)
    a.jump_if(EQUAL, "dtt_after_sign")
    a.load(R8, R11, 32)
    a.op("test", R8, R8)
    a.jump_if(EQUAL, "dtt_after_sign")
    a.push(RDX)
    a.address(RAX, a.text("-"))
    a.load(RCX, R11, 40)
    a.call("join")
    a.pop(RDX)
    a.address(R11, v)
    a.store(R11, 40, RAX)
    a.label("dtt_after_sign")
    a.load(RCX, R11, 8)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "dtt_done")
    a.load(RAX, R11, 24)                 # scale + rest gives "1" then the digits, zeros kept;
    a.op("add", RAX, RDX)                # the "1" is then swapped for the point
    a.call("number_to_text")
    a.set(RCX, ord("."))
    a.store_byte(RAX, 8, RCX)
    a.mov(RCX, RAX)
    a.address(R11, v)
    a.load(RAX, R11, 40)
    a.jump("join")
    a.label("dtt_done")
    a.load(RAX, R11, 40)
    a.ret()


def text_to_number(a):
    """rax = text -> rax = the whole number at its start (0 if none)."""
    a.label("text_to_number")
    a.load(RCX, RAX)
    a.lea(RSI, RAX, 8)
    a.set(R8, 0)
    a.set(R9, 0)
    a.set(R10, 10)
    a.label("ttn_spaces")
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "ttn_done")
    a.load_byte(RDX, RSI)
    a.op_n("cmp", RDX, ord(" "))
    a.jump_if(NOT_EQUAL, "ttn_sign")
    a.op_n("add", RSI, 1)
    a.op_n("sub", RCX, 1)
    a.jump("ttn_spaces")
    a.label("ttn_sign")
    a.op_n("cmp", RDX, ord("-"))
    a.jump_if(NOT_EQUAL, "ttn_digits")
    a.set(R9, 1)
    a.op_n("add", RSI, 1)
    a.op_n("sub", RCX, 1)
    a.label("ttn_digits")
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "ttn_done")
    a.load_byte(RDX, RSI)
    a.op_n("sub", RDX, ord("0"))
    a.op_n("cmp", RDX, 9)
    a.jump_if(ABOVE, "ttn_done")
    a.multiply(R8, R10)
    a.op("add", R8, RDX)
    a.op_n("add", RSI, 1)
    a.op_n("sub", RCX, 1)
    a.jump("ttn_digits")
    a.label("ttn_done")
    a.mov(RAX, R8)
    a.op("test", R9, R9)
    a.jump_if(EQUAL, "ttn_end")
    a.negate(RAX)
    a.label("ttn_end")
    a.ret()


def same_letters(a):
    """rsi, rdi = where two runs of letters start, rcx = how many -> rax = 1 if they match.
    Capitals don't matter: "YES" matches "yes"."""
    def make_small(reg):
        skip = a.new_label("small")
        a.mov(R10, reg)
        a.op_n("sub", R10, ord("A"))
        a.op_n("cmp", R10, 25)
        a.jump_if(ABOVE, skip)
        a.op_n("add", reg, 32)
        a.label(skip)

    a.label("same_letters")
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "sl_yes")
    a.load_byte(RAX, RSI)
    a.load_byte(RDX, RDI)
    make_small(RAX)
    make_small(RDX)
    a.op("cmp", RAX, RDX)
    a.jump_if(NOT_EQUAL, "sl_no")
    a.op_n("add", RSI, 1)
    a.op_n("add", RDI, 1)
    a.op_n("sub", RCX, 1)
    a.jump("same_letters")
    a.label("sl_yes")
    a.set(RAX, 1)
    a.ret()
    a.label("sl_no")
    a.set(RAX, 0)
    a.ret()


def text_equal(a):
    """rax, rcx = texts -> rax = 1 if they're the same (capitals don't matter), else 0."""
    a.label("text_equal")
    a.load(RDX, RAX)
    a.load(R8, RCX)
    a.op("cmp", RDX, R8)
    a.jump_if(NOT_EQUAL, "te_no")
    a.lea(RSI, RAX, 8)
    a.lea(RDI, RCX, 8)
    a.mov(RCX, RDX)
    a.jump("same_letters")
    a.label("te_no")
    a.set(RAX, 0)
    a.ret()


def match_at(a):
    """rax = text, rcx = smaller text, rdx = position -> rax = 1 if the smaller one is there."""
    a.label("match_at")
    a.op("test", RDX, RDX)
    a.jump_if(LESS, "ma_no")
    a.load(R8, RAX)
    a.load(R9, RCX)
    a.mov(R10, RDX)
    a.op("add", R10, R9)
    a.op("cmp", R10, R8)
    a.jump_if(GREATER, "ma_no")
    a.lea(RSI, RAX, 8)
    a.op("add", RSI, RDX)
    a.lea(RDI, RCX, 8)
    a.mov(RCX, R9)
    a.jump("same_letters")
    a.label("ma_no")
    a.set(RAX, 0)
    a.ret()


def contains(a):
    """rax = text, rcx = smaller text -> rax = 1 if it appears anywhere."""
    v = a.reserve(24, "contains_vars")   # 0 text, 8 smaller text, 16 position
    a.label("contains")
    a.address(R11, v)
    a.store(R11, 0, RAX)
    a.store(R11, 8, RCX)
    a.set(RDX, 0)
    a.label("contains_try")
    a.store(R11, 16, RDX)
    a.load(R8, RAX)
    a.load(R9, RCX)
    a.op("sub", R8, R9)
    a.op("cmp", RDX, R8)
    a.jump_if(GREATER, "contains_no")
    a.call("match_at")
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, "contains_yes")
    a.address(R11, v)
    a.load(RAX, R11, 0)
    a.load(RCX, R11, 8)
    a.load(RDX, R11, 16)
    a.op_n("add", RDX, 1)
    a.jump("contains_try")
    a.label("contains_no")
    a.set(RAX, 0)
    a.label("contains_yes")
    a.ret()


def starts_with(a):
    a.label("starts_with")
    a.set(RDX, 0)
    a.jump("match_at")


def ends_with(a):
    a.label("ends_with")
    a.load(RDX, RAX)
    a.load(R8, RCX)
    a.op("sub", RDX, R8)
    a.jump("match_at")


def change_case(name, first, shift):
    """Make a routine that shifts letters first..first+25 by shift (to capitals or to small)."""
    def brick(a):
        a.label(name)
        a.push(RAX)
        a.load(RCX, RAX)
        a.call("new_text")
        a.pop(RSI)
        a.load(RCX, RSI)
        a.lea(R8, RSI, 8)
        a.lea(R9, RAX, 8)
        a.label(f"{name}_next")
        a.op("test", RCX, RCX)
        a.jump_if(EQUAL, f"{name}_done")
        a.load_byte(RDX, R8)
        a.mov(R10, RDX)
        a.op_n("sub", R10, first)
        a.op_n("cmp", R10, 25)
        a.jump_if(ABOVE, f"{name}_keep")
        a.op_n("add", RDX, shift)
        a.label(f"{name}_keep")
        a.store_byte(R9, 0, RDX)
        a.op_n("add", R8, 1)
        a.op_n("add", R9, 1)
        a.op_n("sub", RCX, 1)
        a.jump(f"{name}_next")
        a.label(f"{name}_done")
        a.ret()
    return brick


def letter(a):
    """rax = text, rcx = N -> rax = its Nth letter (counting from 1), or empty if there isn't one."""
    a.label("letter")
    a.op_n("sub", RCX, 1)
    a.jump_if(LESS, "letter_none")
    a.load(RDX, RAX)
    a.op("cmp", RCX, RDX)
    a.jump_if(NOT_LESS, "letter_none")
    a.push(RAX)
    a.push(RCX)
    a.set(RCX, 1)
    a.call("new_text")
    a.pop(RCX)
    a.pop(RSI)
    a.op("add", RSI, RCX)
    a.load_byte(RDX, RSI, 8)
    a.store_byte(RAX, 8, RDX)
    a.ret()
    a.label("letter_none")
    a.set(RCX, 0)
    a.jump("new_text")


def split(a):
    """rax = text, rcx = list, rdx = the letter to split at, r8 = 1 to turn pieces into numbers.
    The list is emptied, then gets one item per piece (empty pieces are skipped)."""
    v = a.reserve(48, "split_vars")   # 0 text, 8 list, 16 split letter, 24 numbers?, 32 position, 40 piece start
    a.label("split")
    a.address(R11, v)
    a.store(R11, 0, RAX)
    a.store(R11, 8, RCX)
    a.store(R11, 16, RDX)
    a.store(R11, 24, R8)
    a.set(RDX, 0)
    a.store(RCX, 0, RDX)
    a.store(R11, 32, RDX)
    a.store(R11, 40, RDX)
    a.label("split_look")
    a.address(R11, v)
    a.load(RAX, R11, 0)
    a.load(RDX, R11, 32)
    a.load(RCX, RAX)
    a.op("cmp", RDX, RCX)
    a.jump_if(EQUAL, "split_last")
    a.op("add", RAX, RDX)
    a.load_byte(RAX, RAX, 8)
    a.load(RCX, R11, 16)
    a.op("cmp", RAX, RCX)
    a.jump_if(EQUAL, "split_cut")
    a.op_n("add", RDX, 1)
    a.store(R11, 32, RDX)
    a.jump("split_look")
    a.label("split_cut")
    a.call("split_piece")
    a.address(R11, v)
    a.load(RDX, R11, 32)
    a.op_n("add", RDX, 1)
    a.store(R11, 32, RDX)
    a.store(R11, 40, RDX)
    a.jump("split_look")
    a.label("split_last")
    a.call("split_piece")
    a.ret()
    # The piece runs from 'piece start' up to 'position'.
    a.label("split_piece")
    a.address(R11, v)
    a.load(RCX, R11, 32)
    a.load(RDX, R11, 40)
    a.op("sub", RCX, RDX)
    a.jump_if(EQUAL, "split_skip")
    a.call("new_text")
    a.address(R11, v)
    a.load(RCX, RAX)
    a.lea(RDI, RAX, 8)
    a.load(RSI, R11, 0)
    a.load(RDX, R11, 40)
    a.op("add", RSI, RDX)
    a.op_n("add", RSI, 8)
    a.copy_bytes()
    a.load(R8, R11, 24)
    a.op("test", R8, R8)
    a.jump_if(EQUAL, "split_keep_text")
    a.call("text_to_number")
    a.label("split_keep_text")
    a.mov(RCX, RAX)
    a.address(R11, v)
    a.load(RAX, R11, 8)
    a.jump("push")
    a.label("split_skip")
    a.ret()


def letters(a):
    """rax = text, rcx = list -> the list is emptied, then gets each letter as an item."""
    v = a.reserve(24, "letters_vars")   # 0 text, 8 list, 16 position
    a.label("letters")
    a.address(R11, v)
    a.store(R11, 0, RAX)
    a.store(R11, 8, RCX)
    a.set(RDX, 0)
    a.store(RCX, 0, RDX)
    a.set(RDX, 1)
    a.label("letters_next")
    a.store(R11, 16, RDX)
    a.load(RAX, R11, 0)
    a.load(RCX, RAX)
    a.op("cmp", RDX, RCX)
    a.jump_if(GREATER, "letters_done")
    a.mov(RCX, RDX)
    a.call("letter")
    a.mov(RCX, RAX)
    a.address(R11, v)
    a.load(RAX, R11, 8)
    a.call("push")
    a.address(R11, v)
    a.load(RDX, R11, 16)
    a.op_n("add", RDX, 1)
    a.jump("letters_next")
    a.label("letters_done")
    a.ret()


def reverse_text(a):
    """rax = text -> rax = a new text with its letters the other way round.
    A letter made of several bytes (like é) keeps its bytes in order."""
    a.label("reverse_text")
    a.push(RAX)
    a.load(RCX, RAX)
    a.call("new_text")
    a.pop(RSI)
    a.load(R8, RSI)
    a.op_n("add", RSI, 8)                  # the letters
    a.lea(R9, RAX, 8)                      # where they go
    a.op_n("sub", R8, 1)                   # the last byte
    a.label("revt_next")
    a.op_n("cmp", R8, 0)
    a.jump_if(LESS, "revt_done")
    a.mov(R10, R8)
    a.label("revt_start")                    # back to the first byte of this letter
    a.op_n("cmp", R10, 0)
    a.jump_if(EQUAL, "revt_copy")
    a.mov(R11, RSI)
    a.op("add", R11, R10)
    a.load_byte(RDX, R11)
    a.op_n("and", RDX, 0xC0)
    a.op_n("cmp", RDX, 0x80)
    a.jump_if(NOT_EQUAL, "revt_copy")
    a.op_n("sub", R10, 1)
    a.jump("revt_start")
    a.label("revt_copy")
    a.mov(R11, R10)
    a.label("revt_byte")
    a.op("cmp", R11, R8)
    a.jump_if(GREATER, "revt_moved")
    a.mov(RCX, RSI)
    a.op("add", RCX, R11)
    a.load_byte(RDX, RCX)
    a.store_byte(R9, 0, RDX)
    a.op_n("add", R9, 1)
    a.op_n("add", R11, 1)
    a.jump("revt_byte")
    a.label("revt_moved")
    a.mov(R8, R10)
    a.op_n("sub", R8, 1)
    a.jump("revt_next")
    a.label("revt_done")
    a.ret()


def reverse_list(a):
    """rax = list, rcx = space for a list -> rax = that space, holding the items the other way round."""
    a.label("reverse_list")
    a.load(RDX, RAX)
    a.store(RCX, 0, RDX)
    a.set(R8, 0)
    a.label("revl_next")
    a.op("cmp", R8, RDX)
    a.jump_if(NOT_LESS, "revl_done")
    a.mov(R9, RDX)
    a.op("sub", R9, R8)
    a.op_n("sub", R9, 1)
    a.shift_left(R9, 3)
    a.op("add", R9, RAX)
    a.load(R10, R9, 8)
    a.mov(R11, R8)
    a.shift_left(R11, 3)
    a.op("add", R11, RCX)
    a.store(R11, 8, R10)
    a.op_n("add", R8, 1)
    a.jump("revl_next")
    a.label("revl_done")
    a.mov(RAX, RCX)
    a.ret()


# ---- keyboard, clock, chance ----

def read_line(a):
    """-> rax = the text typed before Enter (up to TEXT_LIMIT letters, spaces at the end dropped).
    If the input has ended and nothing was typed, the program ends: there's nothing left to wait for."""
    a.label("read_line")
    a.set(RCX, TEXT_LIMIT)
    a.call("new_text")
    a.push(RAX)
    a.lea(R12, RAX, 8)                 # where the next key goes
    a.mov(R14, R12)                    # where the answer starts
    a.lea(R13, RAX, 8 + TEXT_LIMIT)    # end of the box
    a.label("rl_next")
    a.set(RAX, 0)
    a.set(RDI, 0)
    a.mov(RSI, R12)
    a.set(RDX, 1)
    a.syscall()
    a.op_n("cmp", RAX, 1)
    a.jump_if(NOT_EQUAL, "rl_ended")
    a.load_byte(RDX, R12)
    a.op_n("cmp", RDX, 10)
    a.jump_if(EQUAL, "rl_trim")
    a.op_n("add", R12, 1)
    a.op("cmp", R12, R13)
    a.jump_if(BELOW, "rl_next")
    a.jump("rl_trim")
    a.label("rl_ended")
    a.op("cmp", R12, R14)
    a.jump_if(EQUAL, "input_ended")
    a.label("rl_trim")                 # drop spaces (and Windows line endings) at the end
    a.op("cmp", R12, R14)
    a.jump_if(EQUAL, "rl_done")
    a.load_byte(RDX, R12, -1)
    a.op_n("cmp", RDX, ord(" "))
    a.jump_if(EQUAL, "rl_back")
    a.op_n("cmp", RDX, 13)
    a.jump_if(NOT_EQUAL, "rl_done")
    a.label("rl_back")
    a.op_n("sub", R12, 1)
    a.jump("rl_trim")
    a.label("rl_done")
    a.set(RDX, 0)
    a.store_byte(R12, 0, RDX)          # the Enter key isn't part of the answer
    a.pop(RAX)
    a.mov(RCX, R12)
    a.op("sub", RCX, RAX)
    a.op_n("sub", RCX, 8)
    a.store(RAX, 0, RCX)
    a.ret()
    a.label("input_ended")
    say_fixed(a, RESET + "\n")
    finish(a, 0)


def read_number(a):
    """-> rax = a whole number typed by the user. Asks again until it gets one."""
    a.label("read_number")
    a.call("read_line")
    a.lea(RSI, RAX, 8)
    a.load(RCX, RAX)
    a.label("rn_spaces")
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "rn_again")
    a.load_byte(RDX, RSI)
    a.op_n("add", RSI, 1)
    a.op_n("sub", RCX, 1)
    a.op_n("cmp", RDX, ord(" "))
    a.jump_if(EQUAL, "rn_spaces")
    a.op_n("cmp", RDX, ord("-"))
    a.jump_if(NOT_EQUAL, "rn_digit")
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "rn_again")
    a.load_byte(RDX, RSI)
    a.label("rn_digit")
    a.op_n("sub", RDX, ord("0"))
    a.op_n("cmp", RDX, 9)
    a.jump_if(ABOVE, "rn_again")
    a.jump("text_to_number")
    a.label("rn_again")
    say_fixed(a, "Please type a number: ")
    a.jump("read_number")


def wait_key(a):
    """-> rax = the one key pressed (no Enter needed when it's a real terminal)."""
    saved = a.reserve(64, "terminal_settings")
    keys_now = a.reserve(64, "terminal_keys_now")
    a.label("wait_key")
    a.set(RAX, 16)                     # ioctl: get the terminal settings
    a.set(RDI, 0)
    a.set(RSI, 0x5401)
    a.address(RDX, saved)
    a.syscall()
    a.mov(R14, RAX)                    # 0 means it's a real terminal
    a.op("test", R14, R14)
    a.jump_if(NOT_EQUAL, "wk_read")
    a.address(RSI, saved)
    a.address(RDI, keys_now)
    a.set(RCX, 64)
    a.copy_bytes()
    a.address(RDI, keys_now)
    a.load_byte(RDX, RDI, 12)          # switch off line-at-a-time, echo, and Ctrl+C stopping us
    a.op_n("and", RDX, ~0x0B)          # (we handle Ctrl+C ourselves, after putting the terminal back)
    a.store_byte(RDI, 12, RDX)
    a.set(RDX, 1)
    a.store_byte(RDI, 17 + 6, RDX)     # wait for 1 key
    a.set(RDX, 0)
    a.store_byte(RDI, 17 + 5, RDX)     # with no time limit
    a.set(RAX, 16)                     # ioctl: use the new settings
    a.set(RDI, 0)
    a.set(RSI, 0x5402)
    a.address(RDX, keys_now)
    a.syscall()
    a.label("wk_read")
    a.set(RCX, 1)
    a.call("new_text")
    a.push(RAX)
    a.lea(RSI, RAX, 8)
    a.set(RAX, 0)
    a.set(RDI, 0)
    a.set(RDX, 1)
    a.syscall()
    a.mov(R15, RAX)
    a.op("test", R14, R14)
    a.jump_if(NOT_EQUAL, "wk_done")
    a.set(RAX, 16)                     # ioctl: put the settings back
    a.set(RDI, 0)
    a.set(RSI, 0x5402)
    a.address(RDX, saved)
    a.syscall()
    a.label("wk_done")
    a.pop(RAX)
    a.op_n("cmp", R15, 1)
    a.jump_if(NOT_EQUAL, "input_ended")
    a.load_byte(RDX, RAX, 8)
    a.op_n("cmp", RDX, 3)              # Ctrl+C
    a.jump_if(NOT_EQUAL, "wk_end")
    say_fixed(a, RESET + "\n")
    finish(a, 130)
    a.label("wk_end")
    a.ret()


def sleep(a):
    """rax = nanoseconds to wait."""
    when = a.reserve(16, "sleep_time")
    a.label("sleep")
    a.op("test", RAX, RAX)
    a.jump_if(NOT_GREATER, "sleep_done")
    a.set(RDX, 0)
    a.set(RCX, BILLION)
    a.divide(RCX)
    a.address(RDI, when)
    a.store(RDI, 0, RAX)
    a.store(RDI, 8, RDX)
    a.set(RSI, 0)
    a.set(RAX, 35)
    a.syscall()
    a.label("sleep_done")
    a.ret()


def random(a):
    """rax = lowest, rcx = highest -> rax = a random whole number from lowest to highest."""
    chance = a.reserve(8, "random_bytes")
    a.label("random")
    a.push(RAX)
    a.push(RCX)
    a.set(RAX, 318)                    # getrandom
    a.address(RDI, chance)
    a.set(RSI, 8)
    a.set(RDX, 0)
    a.syscall()
    a.pop(RCX)
    a.pop(R8)
    a.op("sub", RCX, R8)
    a.op_n("add", RCX, 1)
    a.jump_if(NOT_GREATER, "random_lowest")
    a.address(RDI, chance)
    a.load(RAX, RDI)
    a.set(RDX, 0)
    a.divide(RCX)
    a.mov(RAX, RDX)
    a.op("add", RAX, R8)
    a.ret()
    a.label("random_lowest")
    a.mov(RAX, R8)
    a.ret()


def now(a):
    """rdi = which clock (0 time of day, 1 stopwatch) -> rax = nanoseconds."""
    when = a.reserve(16, "clock_reading")
    a.label("now")
    a.set(RAX, 228)                    # CLOCK  B8 E4 00 00 00 ... 48 BE [where] 0F 05
    a.address(RSI, when)
    a.syscall()
    a.address(RSI, when)
    a.load(RAX, RSI)
    a.set(RCX, BILLION)
    a.multiply(RAX, RCX)
    a.load(RCX, RSI, 8)
    a.op("add", RAX, RCX)
    a.ret()


def elapsed(a):
    """rax = a stopwatch start -> rax = seconds since then, as a decimal."""
    billion = a.number(BILLION)
    a.label("elapsed")
    a.push(RAX)
    a.set(RDI, 1)
    a.call("now")
    a.pop(RCX)
    a.op("sub", RAX, RCX)
    a.whole_to_decimal(XMM0, RAX)
    a.address(RAX, billion)
    a.load(RAX, RAX)
    a.to_xmm(XMM1, RAX)
    a.decimal_maths("div", XMM0, XMM1)
    a.from_xmm(RAX, XMM0)
    a.ret()


def time_text(a):
    """-> rax = the time of day as HH:MM:SS (UTC)."""
    def two_digits(offset):
        a.set(RDX, 0)
        a.set(RCX, 10)
        a.divide(RCX)
        a.op_n("add", RAX, ord("0"))
        a.store_byte(R13, offset, RAX)
        a.op_n("add", RDX, ord("0"))
        a.store_byte(R13, offset + 1, RDX)

    a.label("time_text")
    a.set(RDI, 0)
    a.call("now")
    a.set(RDX, 0)
    a.set(RCX, BILLION)
    a.divide(RCX)
    a.set(RDX, 0)
    a.set(RCX, 86400)
    a.divide(RCX)
    a.mov(R12, RDX)                    # seconds since midnight
    a.set(RCX, 8)
    a.call("new_text")
    a.mov(R13, RAX)
    a.set(RCX, ord(":"))
    a.store_byte(R13, 10, RCX)
    a.store_byte(R13, 13, RCX)
    a.mov(RAX, R12)
    a.set(RDX, 0)
    a.set(RCX, 3600)
    a.divide(RCX)
    a.mov(R12, RDX)
    two_digits(8)
    a.mov(RAX, R12)
    a.set(RDX, 0)
    a.set(RCX, 60)
    a.divide(RCX)
    a.mov(R12, RDX)
    two_digits(11)
    a.mov(RAX, R12)
    two_digits(14)
    a.mov(RAX, R13)
    a.ret()


# ---- lists ----

def push(a):
    """rax = list, rcx = item -> item added on the end.
    LOAD COUNT, STORE AT start + count x 8, ADD 1 TO COUNT."""
    a.label("push")
    a.load(RDX, RAX)
    a.op_n("cmp", RDX, LIST_LIMIT)
    a.jump_if(NOT_LESS, "list_full")
    a.mov(R8, RDX)
    a.shift_left(R8, 3)
    a.op("add", R8, RAX)
    a.store(R8, 8, RCX)
    a.op_n("add", RDX, 1)
    a.store(RAX, 0, RDX)
    a.ret()
    a.label("list_full")
    say_fixed(a, f"(That list is full: {LIST_LIMIT:,} items is the most it can hold.)\n")
    a.ret()


def push_front(a):
    """rax = list, rcx = item -> item added at the front."""
    a.label("push_front")
    a.load(RDX, RAX)
    a.op_n("cmp", RDX, LIST_LIMIT)
    a.jump_if(NOT_LESS, "list_full")
    a.mov(R8, RDX)
    a.label("pf_move")
    a.op("test", R8, R8)
    a.jump_if(EQUAL, "pf_put")
    a.mov(R9, R8)
    a.shift_left(R9, 3)
    a.op("add", R9, RAX)
    a.load(R10, R9)
    a.store(R9, 8, R10)
    a.op_n("sub", R8, 1)
    a.jump("pf_move")
    a.label("pf_put")
    a.store(RAX, 8, RCX)
    a.op_n("add", RDX, 1)
    a.store(RAX, 0, RDX)
    a.ret()


def remove_first(a):
    """rax = list, rcx = what to give back if it's empty -> rax = the first item, now removed."""
    a.label("remove_first")
    a.load(RDX, RAX)
    a.op("test", RDX, RDX)
    a.jump_if(EQUAL, "list_empty")
    a.load(R10, RAX, 8)
    a.op_n("sub", RDX, 1)
    a.store(RAX, 0, RDX)
    a.lea(RDI, RAX, 8)
    a.lea(RSI, RAX, 16)
    a.mov(RCX, RDX)
    a.shift_left(RCX, 3)
    a.copy_bytes()
    a.mov(RAX, R10)
    a.ret()
    a.label("list_empty")
    a.mov(RAX, RCX)
    a.ret()


def remove_last(a):
    """rax = list, rcx = what to give back if it's empty -> rax = the last item, now removed."""
    a.label("remove_last")
    a.load(RDX, RAX)
    a.op("test", RDX, RDX)
    a.jump_if(EQUAL, "list_empty")
    a.op_n("sub", RDX, 1)
    a.store(RAX, 0, RDX)
    a.shift_left(RDX, 3)
    a.op("add", RAX, RDX)
    a.load(RAX, RAX, 8)
    a.ret()


def item(a):
    """rax = list, rcx = N, rdx = what to give back if there's no item N -> rax = item N."""
    a.label("item")
    a.op_n("sub", RCX, 1)
    a.jump_if(LESS, "item_none")
    a.load(R8, RAX)
    a.op("cmp", RCX, R8)
    a.jump_if(NOT_LESS, "item_none")
    a.shift_left(RCX, 3)
    a.op("add", RAX, RCX)
    a.load(RAX, RAX, 8)
    a.ret()
    a.label("item_none")
    a.mov(RAX, RDX)
    a.ret()


def list_has(a):
    """rax = list, rcx = thing, rdx = 1 if the items are texts -> rax = 1 if the thing is in the list."""
    v = a.reserve(32, "has_vars")   # 0 list, 8 thing, 16 texts?, 24 position
    a.label("list_has")
    a.address(R11, v)
    a.store(R11, 0, RAX)
    a.store(R11, 8, RCX)
    a.store(R11, 16, RDX)
    a.set(R8, 0)
    a.label("has_next")
    a.address(R11, v)
    a.store(R11, 24, R8)
    a.load(RAX, R11, 0)
    a.load(R9, RAX)
    a.op("cmp", R8, R9)
    a.jump_if(NOT_LESS, "has_no")
    a.shift_left(R8, 3)
    a.op("add", RAX, R8)
    a.load(RAX, RAX, 8)
    a.load(RCX, R11, 8)
    a.load(RDX, R11, 16)
    a.op("test", RDX, RDX)
    a.jump_if(EQUAL, "has_numbers")
    a.call("text_equal")
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, "has_yes")
    a.jump("has_again")
    a.label("has_numbers")
    a.op("cmp", RAX, RCX)
    a.jump_if(EQUAL, "has_yes")
    a.label("has_again")
    a.address(R11, v)
    a.load(R8, R11, 24)
    a.op_n("add", R8, 1)
    a.jump("has_next")
    a.label("has_yes")
    a.set(RAX, 1)
    a.ret()
    a.label("has_no")
    a.set(RAX, 0)
    a.ret()


def list_to_text(a):
    """rax = list, rcx = kind (0 whole numbers, 1 decimals, 2 texts), rdx = text to put between
    -> rax = all the items as one text."""
    v = a.reserve(40, "ltt_vars")   # 0 list, 8 kind, 16 between, 24 position, 32 result
    a.label("list_to_text")
    a.address(R11, v)
    a.store(R11, 0, RAX)
    a.store(R11, 8, RCX)
    a.store(R11, 16, RDX)
    a.address(RAX, a.text(""))
    a.store(R11, 32, RAX)
    a.set(R8, 0)
    a.label("ltt_next")
    a.address(R11, v)
    a.store(R11, 24, R8)
    a.load(RAX, R11, 0)
    a.load(R9, RAX)
    a.op("cmp", R8, R9)
    a.jump_if(NOT_LESS, "ltt_done")
    a.op("test", R8, R8)
    a.jump_if(EQUAL, "ltt_item")
    a.load(RAX, R11, 32)
    a.load(RCX, R11, 16)
    a.call("join")
    a.address(R11, v)
    a.store(R11, 32, RAX)
    a.label("ltt_item")
    a.load(RAX, R11, 0)
    a.load(R8, R11, 24)
    a.shift_left(R8, 3)
    a.op("add", RAX, R8)
    a.load(RAX, RAX, 8)
    a.load(RCX, R11, 8)
    a.op_n("cmp", RCX, 1)
    a.jump_if(ABOVE, "ltt_join")
    a.jump_if(EQUAL, "ltt_decimal")
    a.call("number_to_text")
    a.jump("ltt_join")
    a.label("ltt_decimal")
    a.set(RCX, -1)
    a.call("decimal_to_text")
    a.label("ltt_join")
    a.mov(RCX, RAX)
    a.address(R11, v)
    a.load(RAX, R11, 32)
    a.call("join")
    a.address(R11, v)
    a.store(R11, 32, RAX)
    a.load(R8, R11, 24)
    a.op_n("add", R8, 1)
    a.jump("ltt_next")
    a.label("ltt_done")
    a.load(RAX, R11, 32)
    a.ret()


def list_copy(a):
    """rax = from list, rcx = to list -> the to list becomes the same as the from list."""
    a.label("list_copy")
    a.mov(RSI, RAX)
    a.mov(RDI, RCX)
    a.set(RCX, LIST_SIZE)
    a.copy_bytes()
    a.ret()


# ---- files (in the folder the program runs from) ----

def file_save(a):
    """rax = file name, rcx = text, rdx = how to open (replace or add to the end) -> 1 if it worked."""
    a.label("file_save")
    a.push(RCX)
    a.lea(RDI, RAX, 8)                 # OPEN  B8 02 00 00 00 ... BA A4 01 00 00 0F 05
    a.mov(RSI, RDX)
    a.set(RDX, 0o644)
    a.set(RAX, 2)
    a.syscall()
    a.pop(RCX)
    a.op("test", RAX, RAX)
    a.jump_if(LESS, "fs_failed")
    a.mov(R12, RAX)
    a.lea(RSI, RCX, 8)                 # WRITE the text
    a.load(RDX, RCX)
    a.set(RAX, 1)
    a.mov(RDI, R12)
    a.syscall()
    a.address(RSI, a.text("\n"))       # and end the line
    a.op_n("add", RSI, 8)
    a.set(RDX, 1)
    a.set(RAX, 1)
    a.mov(RDI, R12)
    a.syscall()
    a.set(RAX, 3)                      # CLOSE  B8 03 00 00 00 ... 0F 05
    a.mov(RDI, R12)
    a.syscall()
    a.set(RAX, 1)
    a.ret()
    a.label("fs_failed")
    say_fixed(a, "(Couldn't save that file.)\n")
    a.set(RAX, 0)
    a.ret()


def file_load(a):
    """rax = file name -> rax = everything in it as a text (last line ending dropped), or 0 if there's no such file."""
    a.label("file_load")
    a.lea(RDI, RAX, 8)
    a.set(RSI, 0)
    a.set(RDX, 0)
    a.set(RAX, 2)
    a.syscall()
    a.op("test", RAX, RAX)
    a.jump_if(LESS, "fl_missing")
    a.mov(R12, RAX)
    a.set(RCX, FILE_LIMIT)
    a.call("new_text")
    a.mov(R13, RAX)
    a.set(R14, 0)                      # bytes read so far
    a.label("fl_read")
    a.set(RDX, FILE_LIMIT)
    a.op("sub", RDX, R14)
    a.jump_if(EQUAL, "fl_close")
    a.lea(RSI, R13, 8)
    a.op("add", RSI, R14)
    a.mov(RDI, R12)
    a.set(RAX, 0)
    a.syscall()
    a.op("test", RAX, RAX)
    a.jump_if(NOT_GREATER, "fl_close")
    a.op("add", R14, RAX)
    a.jump("fl_read")
    a.label("fl_close")
    a.set(RAX, 3)
    a.mov(RDI, R12)
    a.syscall()
    a.op("test", R14, R14)
    a.jump_if(EQUAL, "fl_size")
    a.mov(R8, R13)
    a.op("add", R8, R14)
    a.load_byte(RDX, R8, 7)
    a.op_n("cmp", RDX, 10)
    a.jump_if(NOT_EQUAL, "fl_size")
    a.op_n("sub", R14, 1)
    a.set(RDX, 0)
    a.store_byte(R8, 7, RDX)
    a.label("fl_size")
    a.store(R13, 0, R14)
    a.mov(RAX, R13)
    a.ret()
    a.label("fl_missing")
    a.set(RAX, 0)
    a.ret()


def file_delete(a):
    """rax = file name -> asks "are you sure?" first, then deletes it."""
    name = a.reserve(8, "delete_name")
    a.label("file_delete")
    a.address(R11, name)
    a.store(R11, 0, RAX)
    say_fixed(a, "Are you sure you want to delete ")
    a.address(R11, name)
    a.load(RAX, R11)
    a.call("write_text")
    say_fixed(a, "? (yes/no) ")
    a.call("read_line")
    a.load_byte(RDX, RAX, 8)
    a.op_n("or", RDX, 0x20)            # Y or y
    a.op_n("cmp", RDX, ord("y"))
    a.jump_if(NOT_EQUAL, "fd_keep")
    a.address(R11, name)
    a.load(RAX, R11)
    a.lea(RDI, RAX, 8)                 # DELETE  B8 57 00 00 00 ... 0F 05
    a.set(RAX, 87)
    a.syscall()
    a.op("test", RAX, RAX)
    a.jump_if(LESS, "fd_missing")
    say_fixed(a, "Deleted.\n")
    a.ret()
    a.label("fd_missing")
    say_fixed(a, "(There was no such file.)\n")
    a.ret()
    a.label("fd_keep")
    say_fixed(a, "OK, I kept it.\n")
    a.ret()


# ---- the brain: symbols with weighted links, compared by how alike their links are ----
#
# Only programs that use the brain words get these routines (see add_brain).
# A symbol is a text; capitals don't matter ("Apple" and "apple" are one symbol). The links live in a
# table of strengths, SYMBOL_LIMIT x SYMBOL_LIMIT whole numbers, in the empty writable area, so it costs
# nothing in the program file. Links go both ways: the strength from a to b is always the one from b to a.

def add_brain(a):
    a.space["brain_count"] = 8                                  # how many symbols so far
    a.space["brain_needed"] = 8                                 # the likeness "is like" needs, minus LIKENESS_NEEDED
    a.space["brain_symbols"] = SYMBOL_LIMIT * 8                 # each symbol's text
    a.space["brain_weights"] = SYMBOL_LIMIT * SYMBOL_LIMIT * 8  # how strongly each one links to each other one
    a.space["brain_breadth"] = SYMBOL_LIMIT * 8                 # how many different symbols each one links to
    a.space["brain_words"] = SYMBOL_LIMIT * 8                   # the symbols of the words being learned
    for brick in [brain_find, brain_add, brain_bump, brain_link, brain_unlink, brain_forget, brain_score,
                  brain_likeness, brain_closest, brain_links, brain_learn, brain_save, brain_load]:
        brick(a)


def keep(a, *regs):
    for reg in regs:
        a.push(reg)


def give_back(a, *regs):
    for reg in reversed(regs):
        a.pop(reg)


KEPT = (R12, R13, R14, R15, RBX)


def brain_cell(a, out, i, j, scratch):
    """out = where the strength of the link from symbol i to symbol j is kept."""
    a.set(out, SYMBOL_LIMIT)
    a.multiply(out, i)
    a.op("add", out, j)
    a.shift_left(out, 3)
    a.address(scratch, "brain_weights")
    a.op("add", out, scratch)


def brain_symbol(a, out, i, scratch):
    """out = the text of symbol i."""
    a.mov(scratch, i)
    a.shift_left(scratch, 3)
    a.address(out, "brain_symbols")
    a.op("add", out, scratch)
    a.load(out, out)


def brain_count(a, out):
    a.address(out, "brain_count")
    a.load(out, out)


def brain_find(a):
    """rax = text -> rax = its symbol number, or -1 if the brain hasn't met it. r12-r15 and rbx are kept."""
    a.label("brain_find")
    keep(a, R12, R13)
    a.mov(R12, RAX)
    a.set(R13, 0)
    a.label("bf_next")
    brain_count(a, RCX)
    a.op("cmp", R13, RCX)
    a.jump_if(NOT_LESS, "bf_none")
    brain_symbol(a, RCX, R13, RDX)
    a.mov(RAX, R12)
    a.call("text_equal")
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, "bf_found")
    a.op_n("add", R13, 1)
    a.jump("bf_next")
    a.label("bf_found")
    a.mov(RAX, R13)
    give_back(a, R12, R13)
    a.ret()
    a.label("bf_none")
    a.set(RAX, -1)
    give_back(a, R12, R13)
    a.ret()


def brain_add(a):
    """rax = text -> rax = its symbol number, made if it's new; -1 for an empty text.
    When the brain is full the program stops with a message. r12-r15 and rbx are kept."""
    a.label("brain_add")
    a.load(RCX, RAX)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "ba_empty")
    a.push(RAX)
    a.call("brain_find")
    a.pop(RCX)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_NEGATIVE, "ba_done")
    a.address(RDX, "brain_count")
    a.load(R8, RDX)
    a.op_n("cmp", R8, SYMBOL_LIMIT)
    a.jump_if(NOT_LESS, "brain_full")
    a.mov(R9, R8)
    a.shift_left(R9, 3)
    a.address(R10, "brain_symbols")
    a.op("add", R9, R10)
    a.store(R9, 0, RCX)
    a.mov(R9, R8)
    a.op_n("add", R9, 1)
    a.store(RDX, 0, R9)
    a.mov(RAX, R8)
    a.label("ba_done")
    a.ret()
    a.label("ba_empty")
    a.set(RAX, -1)
    a.ret()
    a.label("brain_full")
    say_fixed(a, f"{RESET}\n(The brain is full: {SYMBOL_LIMIT:,} symbols is the most it can hold.)\n")
    finish(a, 1)


def brain_bump(a):
    """rax, rcx = two different symbol numbers, rdx = how much stronger their link gets (0: no link at all).
    Keeps each symbol's breadth up to date. Only changes rax, rcx, rdx, r8, r9, r10, r11."""
    def breadth(reg, change):
        a.address(R11, "brain_breadth")
        a.mov(R9, reg)
        a.shift_left(R9, 3)
        a.op("add", R11, R9)
        a.load(R9, R11)
        a.op_n("add", R9, change)
        a.store(R11, 0, R9)

    a.label("brain_bump")
    brain_cell(a, R8, RAX, RCX, R9)
    brain_cell(a, R10, RCX, RAX, R9)
    a.load(R9, R8)                       # how strong it was
    a.op("test", RDX, RDX)
    a.jump_if(EQUAL, "bb_clear")
    a.mov(R11, R9)
    a.op("add", R9, RDX)
    a.store(R8, 0, R9)
    a.store(R10, 0, R9)
    a.op("test", R11, R11)
    a.jump_if(NOT_EQUAL, "bb_done")
    breadth(RAX, 1)                      # a new link: both are a little broader
    breadth(RCX, 1)
    a.label("bb_done")
    a.ret()
    a.label("bb_clear")
    a.op("test", R9, R9)
    a.jump_if(EQUAL, "bb_done")
    a.set(R9, 0)
    a.store(R8, 0, R9)
    a.store(R10, 0, R9)
    breadth(RAX, -1)
    breadth(RCX, -1)
    a.ret()


def brain_link(a):
    """rax, rcx = two texts, rdx = how much stronger -> they're linked, both ways (new symbols are made).
    An empty text, or a text linked to itself, does nothing."""
    a.label("brain_link")
    a.push(RDX)
    a.push(RCX)
    a.call("brain_add")
    a.pop(RCX)
    a.push(RAX)
    a.mov(RAX, RCX)
    a.call("brain_add")
    a.pop(RCX)
    a.pop(RDX)
    a.label("bl_numbers")
    a.op("test", RAX, RAX)
    a.jump_if(LESS, "bl_none")
    a.op("test", RCX, RCX)
    a.jump_if(LESS, "bl_none")
    a.op("cmp", RAX, RCX)
    a.jump_if(EQUAL, "bl_none")
    a.jump("brain_bump")
    a.label("bl_none")
    a.ret()


def brain_unlink(a):
    """rax, rcx = two texts -> the link between them is gone (if there was one)."""
    a.label("brain_unlink")
    a.push(RCX)
    a.call("brain_find")
    a.pop(RCX)
    a.push(RAX)
    a.mov(RAX, RCX)
    a.call("brain_find")
    a.pop(RCX)
    a.set(RDX, 0)
    a.jump("bl_numbers")


def brain_forget(a):
    """rax = text -> every link it has is gone."""
    a.label("brain_forget")
    keep(a, R12, R13)
    a.call("brain_find")
    a.op("test", RAX, RAX)
    a.jump_if(LESS, "bfo_done")
    a.mov(R12, RAX)
    a.set(R13, 0)
    a.label("bfo_next")
    brain_count(a, RCX)
    a.op("cmp", R13, RCX)
    a.jump_if(NOT_LESS, "bfo_done")
    a.mov(RAX, R12)
    a.mov(RCX, R13)
    a.set(RDX, 0)
    a.op("cmp", RAX, RCX)
    a.jump_if(EQUAL, "bfo_skip")
    a.call("brain_bump")
    a.label("bfo_skip")
    a.op_n("add", R13, 1)
    a.jump("bfo_next")
    a.label("bfo_done")
    give_back(a, R12, R13)
    a.ret()


SCALE = 1 << 20           # fixed point for the shares in brain_score, when the exact one would be too big
SCALE_LIMIT = 1 << 24     # the exact scale: the lcm of the breadths involved, if it's at most this
SHARE_LIMIT = 1 << 30     # totals are halved (with their parts) until they're below this, so products fit


def brain_score(a):
    """rax, rcx = two different symbol numbers a, b -> rax = how alike they are, 0 to 100.
    Every other symbol s (not a or b) linked to either counts in proportion to w(a,s) / (1 + floor(log2
    breadth'(s))): a link to a broad symbol (linked to many) says less than a link to a specific one.
    breadth'(s) is 1 + how many symbols s links to other than a and b, so a link only a and b share is fully
    specific (divisor 1); breadth' 2-3 divides by 2, 4-7 by 3, and so on.
    With p_a(s) = w(a,s) / (1 + floor(log2 breadth'(s)))
    and P_a their total: 100 x sum of min(p_a(s) x P_b, p_b(s) x P_a) / sum of max(...), rounded down;
    0 if either total is 0. In whole numbers: p is kept times the lcm of the divisors involved, so it's exact
    (the divisors are at most 11, so the lcm is at most 27,720; times 2^20 if it were ever over 2^24); each side is halved until its total is below 2^30, so every product
    stays below 2^60 and the sums below 2^62; the x 100 is done in 128 bits. Exact while each side's total
    p stays below 2^30 / scale. (A link strength must stay below 2^39.) Keeps r12-r15 and rbx."""
    v = a.reserve(72, "brain_score_vars")   # 0 Pa, 8 Pb, 16 a's halvings, 24 b's, 32 the smaller ones, 40 the bigger,
                                            # 48 row a, 56 row b, 64 the scale
    a.label("brain_score")
    keep(a, *KEPT)
    a.address(R14, v)
    a.mov(R10, RAX)
    a.mov(R11, RCX)
    a.set(R8, SYMBOL_LIMIT * 8)
    a.address(R9, "brain_weights")
    a.mov(RDX, RAX)
    a.multiply(RDX, R8)
    a.op("add", RDX, R9)
    a.store(R14, 48, RDX)                # a's row of strengths
    a.mov(RDX, RCX)
    a.multiply(RDX, R8)
    a.op("add", RDX, R9)
    a.store(R14, 56, RDX)                # b's row
    a.set(RDX, 0)
    for at in (0, 8, 32, 40):
        a.store(R14, at, RDX)
    a.set(RDX, 1)
    a.store(R14, 64, RDX)
    brain_count(a, R13)

    def each(name, body):
        """Go through every other symbol s linked to a or b: r9, rbx = w(a,s), w(b,s);
        r15 = 1 + floor(log2 breadth'(s))."""
        a.load(RSI, R14, 48)
        a.load(RDI, R14, 56)
        a.address(R12, "brain_breadth")
        a.set(R8, 0)
        a.label(f"{name}_next")
        a.op("cmp", R8, R13)
        a.jump_if(NOT_LESS, f"{name}_end")
        a.op("cmp", R8, R10)
        a.jump_if(EQUAL, f"{name}_skip")
        a.op("cmp", R8, R11)
        a.jump_if(EQUAL, f"{name}_skip")
        a.load(R9, RSI)
        a.load(RBX, RDI)
        a.mov(RAX, R9)
        a.op("or", RAX, RBX)
        a.jump_if(EQUAL, f"{name}_skip")
        a.load(R15, R12)                 # breadth'(s) = 1 + the symbols s links to other than a and b
        a.op_n("add", R15, 1)
        for w in (R9, RBX):
            a.op("test", w, w)
            a.jump_if(EQUAL, f"{name}_{w}_not")
            a.op_n("sub", R15, 1)
            a.label(f"{name}_{w}_not")
        a.mov(RAX, R15)                  # the divisor: 1 + floor(log2 breadth'(s))
        a.set(R15, 1)
        a.label(f"{name}_log")
        a.op_n("cmp", RAX, 1)
        a.jump_if(NOT_ABOVE, f"{name}_logged")
        a.shift_right(RAX, 1)
        a.op_n("add", R15, 1)
        a.jump(f"{name}_log")
        a.label(f"{name}_logged")
        body()
        a.label(f"{name}_skip")
        a.op_n("add", RSI, 8)
        a.op_n("add", RDI, 8)
        a.op_n("add", R12, 8)
        a.op_n("add", R8, 1)
        a.jump(f"{name}_next")
        a.label(f"{name}_end")

    def share(weight):
        """rax = weight x the scale / the divisor for s."""
        a.mov(RAX, weight)
        a.load(RDX, R14, 64)
        a.multiply(RAX, RDX)
        a.set(RDX, 0)
        a.divide(R15)

    def add_to(at, reg):
        a.load(RDX, R14, at)
        a.op("add", RDX, reg)
        a.store(R14, at, RDX)

    def scale():
        """The scale so far = lcm(scale, the divisor for s); 0 once it's over SCALE_LIMIT."""
        a.load(RAX, R14, 64)
        a.op("test", RAX, RAX)
        a.jump_if(EQUAL, "bsl_done")
        a.mov(RCX, R15)
        a.label("bsl_gcd")
        a.op("test", RCX, RCX)
        a.jump_if(EQUAL, "bsl_gcd_done")
        a.set(RDX, 0)
        a.divide(RCX)
        a.mov(RAX, RCX)
        a.mov(RCX, RDX)
        a.jump("bsl_gcd")
        a.label("bsl_gcd_done")
        a.mov(RCX, RAX)
        a.load(RAX, R14, 64)
        a.set(RDX, 0)
        a.divide(RCX)
        a.multiply(RAX, R15)
        a.op_n("cmp", RAX, SCALE_LIMIT)
        a.jump_if(NOT_ABOVE, "bsl_keep")
        a.set(RAX, 0)
        a.label("bsl_keep")
        a.store(R14, 64, RAX)
        a.label("bsl_done")

    def totals():
        share(R9)
        add_to(0, RAX)
        share(RBX)
        add_to(8, RAX)

    def parts():
        share(R9)                        # p_a(s), halved like P_a, times P_b
        a.load(RCX, R14, 16)
        a.shift_right_cl(RAX)
        a.load(RDX, R14, 8)
        a.multiply(RAX, RDX)
        a.mov(R9, RAX)
        share(RBX)                       # p_b(s), halved like P_b, times P_a
        a.load(RCX, R14, 24)
        a.shift_right_cl(RAX)
        a.load(RDX, R14, 0)
        a.multiply(RAX, RDX)
        a.mov(RBX, RAX)
        a.op("cmp", R9, RBX)
        a.jump_if(ABOVE, "bs_swap")
        add_to(32, R9)
        add_to(40, RBX)
        a.jump("bs_skip")
        a.label("bs_swap")
        add_to(32, RBX)
        add_to(40, R9)

    each("bsl", scale)
    a.load(RAX, R14, 64)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, "bs_scaled")
    a.set(RAX, SCALE)
    a.store(R14, 64, RAX)
    a.label("bs_scaled")
    each("bst", totals)
    for total, halvings in ((0, 16), (8, 24)):
        a.load(RAX, R14, total)
        a.op("test", RAX, RAX)
        a.jump_if(EQUAL, "bs_zero")
        a.set(RCX, 0)
        top, done = a.new_label("bs_halve"), a.new_label("bs_halved")
        a.label(top)
        a.op_n("cmp", RAX, SHARE_LIMIT)
        a.jump_if(BELOW, done)
        a.shift_right(RAX, 1)
        a.op_n("add", RCX, 1)
        a.jump(top)
        a.label(done)
        a.store(R14, total, RAX)
        a.store(R14, halvings, RCX)
    each("bs", parts)
    a.load(RAX, R14, 32)
    a.load(RCX, R14, 40)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "bs_zero")
    a.set(R9, 100)
    a.multiply_wide(R9)
    a.divide(RCX)
    give_back(a, *KEPT)
    a.ret()
    a.label("bs_zero")
    a.set(RAX, 0)
    give_back(a, *KEPT)
    a.ret()


def brain_likeness(a):
    """rax, rcx = two texts -> rax = how alike they are, 0 to 100. The same text is 100;
    a text the brain hasn't met is 0."""
    a.label("brain_likeness")
    keep(a, R12, R13)
    a.mov(R12, RAX)
    a.mov(R13, RCX)
    a.call("text_equal")
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, "bli_same")
    a.mov(RAX, R12)
    a.call("brain_find")
    a.mov(R12, RAX)
    a.mov(RAX, R13)
    a.call("brain_find")
    a.mov(RCX, RAX)
    a.mov(RAX, R12)
    give_back(a, R12, R13)
    a.op("test", RAX, RAX)
    a.jump_if(LESS, "bli_zero")
    a.op("test", RCX, RCX)
    a.jump_if(LESS, "bli_zero")
    a.jump("brain_score")
    a.label("bli_zero")
    a.set(RAX, 0)
    a.ret()
    a.label("bli_same")
    a.set(RAX, 100)
    give_back(a, R12, R13)
    a.ret()


def brain_closest(a):
    """rax = text -> rax = the symbol most like it (not itself), or an empty text if none is at all alike.
    On a tie, the one the brain met first."""
    a.label("brain_closest")
    keep(a, *KEPT)
    a.call("brain_find")
    a.mov(R12, RAX)
    a.set(R14, 0)                        # the best likeness so far
    a.set(R15, -1)                       # and which symbol it was
    a.op("test", R12, R12)
    a.jump_if(LESS, "bc_done")
    a.set(R13, 0)
    a.label("bc_next")
    brain_count(a, RCX)
    a.op("cmp", R13, RCX)
    a.jump_if(NOT_LESS, "bc_done")
    a.op("cmp", R13, R12)
    a.jump_if(EQUAL, "bc_skip")
    a.mov(RAX, R12)
    a.mov(RCX, R13)
    a.call("brain_score")
    a.op("cmp", RAX, R14)
    a.jump_if(NOT_GREATER, "bc_skip")
    a.mov(R14, RAX)
    a.mov(R15, R13)
    a.label("bc_skip")
    a.op_n("add", R13, 1)
    a.jump("bc_next")
    a.label("bc_done")
    a.address(RAX, a.text(""))
    a.op("test", R15, R15)
    a.jump_if(LESS, "bc_end")
    brain_symbol(a, RAX, R15, RCX)
    a.label("bc_end")
    give_back(a, *KEPT)
    a.ret()


def brain_links(a):
    """rax = text, rcx = list -> the list is emptied, then gets every symbol linked to the text,
    in the order the brain met them."""
    a.label("brain_links")
    keep(a, *KEPT)
    a.mov(R13, RCX)
    a.set(RDX, 0)
    a.store(R13, 0, RDX)
    a.call("brain_find")
    a.mov(R12, RAX)
    a.op("test", R12, R12)
    a.jump_if(LESS, "blk_done")
    a.set(R14, 0)
    a.label("blk_next")
    brain_count(a, RCX)
    a.op("cmp", R14, RCX)
    a.jump_if(NOT_LESS, "blk_done")
    brain_cell(a, R8, R12, R14, R9)
    a.load(R8, R8)
    a.op("test", R8, R8)
    a.jump_if(EQUAL, "blk_skip")
    brain_symbol(a, RCX, R14, RDX)
    a.mov(RAX, R13)
    a.call("push")
    a.label("blk_skip")
    a.op_n("add", R14, 1)
    a.jump("blk_next")
    a.label("blk_done")
    give_back(a, *KEPT)
    a.ret()


def word_letter(a, reg, yes):
    """Jump to yes if the byte in reg belongs in a word: a letter, a digit, or part of a letter like é."""
    a.op_n("cmp", reg, 0x80)
    a.jump_if(NOT_LESS, yes)
    a.mov(R10, reg)
    a.op_n("sub", R10, ord("0"))
    a.op_n("cmp", R10, 9)
    a.jump_if(NOT_ABOVE, yes)
    a.mov(R10, reg)
    a.op_n("or", R10, 0x20)
    a.op_n("sub", R10, ord("a"))
    a.op_n("cmp", R10, 25)
    a.jump_if(NOT_ABOVE, yes)


def brain_learn(a):
    """rax = text, rcx = 0 -> every two different words in it are linked (one stronger each time).
    rcx = a symbol instead -> that symbol is linked to each different word in the text.
    Words are runs of letters and digits ("don't" is one word), in small letters; the rest is skipped.
    When learning (rcx = 0), each word is also linked to its frame, the words around it: in "a cherry is",
    cherry is linked to the symbol "a _ is" (the first word gets "_ cherry", the last one "sweet _").
    A frame is a symbol like any other: its links are the words that filled it."""
    to = a.reserve(8, "brain_learn_to")
    frames = a.reserve(16, "brain_frames")     # 0 the word before the last one, 8 the last one
    a.label("brain_learn")
    keep(a, *KEPT)
    a.address(RDX, to)
    a.store(RDX, 0, RCX)
    a.address(RDX, frames)
    a.set(RCX, 0)
    a.store(RDX, 0, RCX)
    a.store(RDX, 8, RCX)
    a.mov(R12, RAX)
    a.set(R13, 0)                        # where we are in the text
    a.set(R14, 0)                        # how many different words so far
    a.label("bln_look")
    a.load(RCX, R12)
    a.op("cmp", R13, RCX)
    a.jump_if(NOT_LESS, "bln_pairs")
    a.mov(RSI, R12)
    a.op("add", RSI, R13)
    a.load_byte(RDX, RSI, 8)
    word_letter(a, RDX, "bln_word")
    a.op_n("add", R13, 1)
    a.jump("bln_look")
    a.label("bln_word")
    a.mov(R15, R13)                      # the word starts here
    a.label("bln_more")
    a.op_n("add", R13, 1)
    a.load(RCX, R12)
    a.op("cmp", R13, RCX)
    a.jump_if(NOT_LESS, "bln_cut")
    a.mov(RSI, R12)
    a.op("add", RSI, R13)
    a.load_byte(RDX, RSI, 8)
    word_letter(a, RDX, "bln_more")
    a.op_n("cmp", RDX, ord("'"))         # an apostrophe with a letter after it stays in the word
    a.jump_if(NOT_EQUAL, "bln_cut")
    a.mov(RAX, R13)
    a.op_n("add", RAX, 1)
    a.op("cmp", RAX, RCX)
    a.jump_if(NOT_LESS, "bln_cut")
    a.load_byte(RDX, RSI, 9)
    word_letter(a, RDX, "bln_more")
    a.label("bln_cut")
    a.mov(RCX, R13)
    a.op("sub", RCX, R15)
    a.call("new_text")
    a.mov(RCX, R13)
    a.op("sub", RCX, R15)
    a.mov(RSI, R12)
    a.op("add", RSI, R15)
    a.op_n("add", RSI, 8)
    a.lea(RDI, RAX, 8)
    a.label("bln_copy")                  # copied in small letters
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "bln_copied")
    a.load_byte(RDX, RSI)
    a.mov(R10, RDX)
    a.op_n("sub", R10, ord("A"))
    a.op_n("cmp", R10, 25)
    a.jump_if(ABOVE, "bln_small")
    a.op_n("add", RDX, 32)
    a.label("bln_small")
    a.store_byte(RDI, 0, RDX)
    a.op_n("add", RSI, 1)
    a.op_n("add", RDI, 1)
    a.op_n("sub", RCX, 1)
    a.jump("bln_copy")
    a.label("bln_copied")
    a.address(RDX, to)                   # the word before this one gets its frame: "a _ is"
    a.load(RDX, RDX)
    a.op("test", RDX, RDX)
    a.jump_if(NOT_EQUAL, "bln_add")
    a.address(R8, frames)
    a.load(RCX, R8, 8)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, "bln_shift")
    a.push(RAX)
    a.load(RAX, R8, 0)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, "bln_lone")
    a.address(RCX, a.text(" _ "))
    a.call("join")
    a.jump("bln_framed")
    a.label("bln_lone")
    a.address(RAX, a.text("_ "))
    a.label("bln_framed")
    a.load(RCX, RSP, 0)
    a.call("join")
    a.mov(RCX, RAX)
    a.address(R8, frames)
    a.load(RAX, R8, 8)
    a.set(RDX, 1)
    a.call("brain_link")
    a.pop(RAX)
    a.label("bln_shift")
    a.address(R8, frames)
    a.load(RCX, R8, 8)
    a.store(R8, 0, RCX)
    a.store(R8, 8, RAX)
    a.label("bln_add")
    a.call("brain_add")
    a.set(R8, 0)                         # met already in this text? Then it's not counted again
    a.address(R9, "brain_words")
    a.label("bln_seen")
    a.op("cmp", R8, R14)
    a.jump_if(NOT_LESS, "bln_new")
    a.load(R10, R9)
    a.op("cmp", R10, RAX)
    a.jump_if(EQUAL, "bln_look")
    a.op_n("add", R9, 8)
    a.op_n("add", R8, 1)
    a.jump("bln_seen")
    a.label("bln_new")
    a.store(R9, 0, RAX)
    a.op_n("add", R14, 1)
    a.jump("bln_look")
    a.label("bln_pairs")                 # the last word's frame: "sweet _"
    a.address(RAX, to)
    a.load(RAX, RAX)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, "bln_framed_all")
    a.address(R8, frames)
    a.load(RAX, R8, 0)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, "bln_framed_all")
    a.address(RCX, a.text(" _"))
    a.call("join")
    a.mov(RCX, RAX)
    a.address(R8, frames)
    a.load(RAX, R8, 8)
    a.set(RDX, 1)
    a.call("brain_link")
    a.label("bln_framed_all")            # now link every two of them, or each to the symbol
    a.address(RAX, to)
    a.load(RAX, RAX)
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, "bln_every")
    a.call("brain_add")
    a.mov(R13, RAX)
    a.op("test", R13, R13)
    a.jump_if(LESS, "bln_done")
    a.set(R12, 0)
    a.label("bln_each")
    a.op("cmp", R12, R14)
    a.jump_if(NOT_LESS, "bln_done")
    a.address(R9, "brain_words")
    a.mov(R10, R12)
    a.shift_left(R10, 3)
    a.op("add", R10, R9)
    a.load(RCX, R10)
    a.op_n("add", R12, 1)
    a.op("cmp", RCX, R13)
    a.jump_if(EQUAL, "bln_each")
    a.mov(RAX, R13)
    a.set(RDX, 1)
    a.call("brain_bump")
    a.jump("bln_each")
    a.label("bln_every")
    a.set(R12, 0)
    a.label("bln_first")
    a.op("cmp", R12, R14)
    a.jump_if(NOT_LESS, "bln_done")
    a.mov(R13, R12)
    a.label("bln_second")
    a.op_n("add", R13, 1)
    a.op("cmp", R13, R14)
    a.jump_if(NOT_LESS, "bln_next_first")
    a.address(R9, "brain_words")
    a.mov(R10, R12)
    a.shift_left(R10, 3)
    a.op("add", R10, R9)
    a.load(RAX, R10)
    a.mov(R10, R13)
    a.shift_left(R10, 3)
    a.op("add", R10, R9)
    a.load(RCX, R10)
    a.set(RDX, 1)
    a.call("brain_bump")
    a.jump("bln_second")
    a.label("bln_next_first")
    a.op_n("add", R12, 1)
    a.jump("bln_first")
    a.label("bln_done")
    give_back(a, *KEPT)
    a.ret()


def brain_save(a):
    """rax = file name -> every link saved in it, one per line: the two symbols and the strength,
    with a tab between."""
    v = a.reserve(8, "brain_save_how")
    a.label("brain_save")
    keep(a, *KEPT)
    a.mov(R15, RAX)
    a.address(RBX, v)
    a.set(RCX, 0o1101)                   # the first line replaces what was in the file
    a.store(RBX, 0, RCX)
    a.set(R13, 0)
    a.label("bsv_first")
    brain_count(a, RCX)
    a.op("cmp", R13, RCX)
    a.jump_if(NOT_LESS, "bsv_end")
    a.mov(R14, R13)
    a.label("bsv_second")
    a.op_n("add", R14, 1)
    brain_count(a, RCX)
    a.op("cmp", R14, RCX)
    a.jump_if(NOT_LESS, "bsv_next_first")
    brain_cell(a, R12, R13, R14, R9)
    a.load(R12, R12)
    a.op("test", R12, R12)
    a.jump_if(EQUAL, "bsv_second")
    brain_symbol(a, RAX, R13, RCX)
    a.address(RCX, a.text("\t"))
    a.call("join")
    brain_symbol(a, RCX, R14, RDX)
    a.call("join")
    a.address(RCX, a.text("\t"))
    a.call("join")
    a.push(RAX)
    a.mov(RAX, R12)
    a.call("number_to_text")
    a.mov(RCX, RAX)
    a.pop(RAX)
    a.call("join")
    a.mov(RCX, RAX)
    a.mov(RAX, R15)
    a.address(RBX, v)
    a.load(RDX, RBX)
    a.call("file_save")
    a.address(RBX, v)
    a.set(RCX, 0o2101)                   # the rest go on the end
    a.store(RBX, 0, RCX)
    a.jump("bsv_second")
    a.label("bsv_next_first")
    a.op_n("add", R13, 1)
    a.jump("bsv_first")
    a.label("bsv_end")
    a.address(RBX, v)                    # no links: the file is emptied
    a.load(RDX, RBX)
    a.op_n("cmp", RDX, 0o1101)
    a.jump_if(NOT_EQUAL, "bsv_done")
    a.mov(RAX, R15)
    a.address(RCX, a.text(""))
    a.call("file_save")
    a.label("bsv_done")
    give_back(a, *KEPT)
    a.ret()


def brain_load(a):
    """rax = file name -> every link in it (as brain_save writes them) is added to the brain.
    Lines that don't fit are skipped. If there's no such file, nothing changes."""
    def field_to(stop, end_label):
        """Step r13 on to the next tab or line end (or the end of the text); r14 = where the field started."""
        top = a.new_label("bld_field")
        a.mov(R14, R13)
        a.label(top)
        a.load(RCX, R12)
        a.op("cmp", R13, RCX)
        a.jump_if(NOT_LESS, end_label)
        a.mov(RSI, R12)
        a.op("add", RSI, R13)
        a.load_byte(RDX, RSI, 8)
        a.op_n("cmp", RDX, stop)
        a.jump_if(EQUAL, end_label)
        a.op_n("cmp", RDX, 10)
        a.jump_if(EQUAL, end_label)
        a.op_n("add", R13, 1)
        a.jump(top)

    def field_text():
        """rax = the field from r14 up to r13, as a new text."""
        a.mov(RCX, R13)
        a.op("sub", RCX, R14)
        a.call("new_text")
        a.load(RCX, RAX)
        a.mov(RSI, R12)
        a.op("add", RSI, R14)
        a.op_n("add", RSI, 8)
        a.lea(RDI, RAX, 8)
        a.copy_bytes()

    def at_tab(otherwise):
        """Go to otherwise unless r13 is at a tab; then step past it."""
        a.load(RCX, R12)
        a.op("cmp", R13, RCX)
        a.jump_if(NOT_LESS, otherwise)
        a.mov(RSI, R12)
        a.op("add", RSI, R13)
        a.load_byte(RDX, RSI, 8)
        a.op_n("cmp", RDX, 9)
        a.jump_if(NOT_EQUAL, otherwise)
        a.op_n("add", R13, 1)

    a.label("brain_load")
    keep(a, *KEPT)
    a.call("file_load")
    a.op("test", RAX, RAX)
    a.jump_if(EQUAL, "bld_done")
    a.mov(R12, RAX)
    a.set(R13, 0)
    a.label("bld_line")
    a.load(RCX, R12)
    a.op("cmp", R13, RCX)
    a.jump_if(NOT_LESS, "bld_done")
    field_to(9, "bld_a")
    a.label("bld_a")
    at_tab("bld_skip")
    a.op_n("sub", R13, 1)
    field_text()
    a.op_n("add", R13, 1)
    a.mov(R15, RAX)                      # the first symbol
    field_to(9, "bld_b")
    a.label("bld_b")
    at_tab("bld_skip")
    a.op_n("sub", R13, 1)
    field_text()
    a.op_n("add", R13, 1)
    a.mov(RBX, RAX)                      # the second symbol
    field_to(10, "bld_n")
    a.label("bld_n")
    field_text()
    a.call("text_to_number")
    a.mov(RDX, RAX)
    a.op("test", RDX, RDX)
    a.jump_if(NOT_GREATER, "bld_skip")
    a.mov(RAX, R15)
    a.mov(RCX, RBX)
    a.call("brain_link")
    a.label("bld_skip")                  # on to the start of the next line
    a.load(RCX, R12)
    a.op("cmp", R13, RCX)
    a.jump_if(NOT_LESS, "bld_done")
    a.mov(RSI, R12)
    a.op("add", RSI, R13)
    a.load_byte(RDX, RSI, 8)
    a.op_n("add", R13, 1)
    a.op_n("cmp", RDX, 10)
    a.jump_if(NOT_EQUAL, "bld_skip")
    a.jump("bld_line")
    a.label("bld_done")
    give_back(a, *KEPT)
    a.ret()
