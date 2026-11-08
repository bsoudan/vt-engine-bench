#!/usr/bin/env python3
"""The program an nxterm vtapp runs to put nxgui under the GPUI window-capture benchmark.

It is the counterpart of src/bin/gpui_bench.rs for a real terminal: nxterm
owns the pty this program's stdin and stdout are, nxgui paints the result,
and scripts/capture_gpui_linux.py watches the pixels. Where the GPUI app
paints a color marker beside its viewport, this program paints one in the
terminal's own cells: a BAR_COLUMNS-wide column of marker-colored spaces
down the right edge of the screen, repainted after every update with the
next of the same four colors the GPUI app cycles through.

Phases, as in the GPUI app:

1. The screen is cleared and the bar painted blue; the program then waits
   for the collector's ready file.
2. 80 chunks of the committed tmux recording are written to the pty at
   ~16 ms intervals, each followed by the bar in its next color. Chunks are
   cut at escape-sequence and UTF-8 boundaries (the GPUI engines take
   arbitrary splits; a sequence split around our bar's own escapes would
   corrupt the terminal's parse).
3. The keys-ready file is created; 40 presses of `a` are then expected on
   stdin, which scripts/nxgui_keys.py injects through GNOME; each one writes
   the same "INPUT nnn a" line the GPUI key handler writes, and the bar.
   Anything else on stdin is dropped: the recording carries terminal
   queries (device attributes, cursor position) that the terminal answers
   on the pty.

events.csv has the GPUI app's columns (phase,index,sent_epoch_ms): a tmux
update's time is taken just before its write, an input's when its key byte
arrived here (what the GPUI app's key handler sees), so the input phase
measures from the key reaching the program in the terminal, not from its
injection into GNOME (nxgui_keys.py records those times in keys.csv).

Usage: nxgui_feed.py <events.csv> <ready-file> <keys-ready-file> <log-file> [fixture]
"""
import fcntl
import os
import select
import struct
import sys
import termios
import time
import tty
from pathlib import Path

OUTPUT_FRAMES = 80
INPUT_FRAMES = 40
COLS, ROWS = 120, 40
BAR_COLUMNS = 16
COLORS = [(0x3B, 0x82, 0xF6), (0xEF, 0x44, 0x44), (0x22, 0xC5, 0x5E), (0xEA, 0xB3, 0x08)]
SIZE_TIMEOUT_S = 30.0
READY_TIMEOUT_S = 60.0
KEY_TIMEOUT_S = 10.0


def millis():
    return time.time() * 1000.0


def winsize(fd):
    rows, cols, _, _ = struct.unpack("HHHH", fcntl.ioctl(fd, termios.TIOCGWINSZ, b"\0" * 8))
    return cols, rows


def safe_cuts(data):
    """Byte offsets at which the stream may be cut: outside escape sequences and UTF-8 characters."""
    safe = []
    i, n = 0, len(data)
    while i < n:
        safe.append(i)
        b = data[i]
        if b == 0x1B and i + 1 < n:
            nxt = data[i + 1]
            if nxt == 0x5B:  # CSI: parameters and intermediates, then a final byte 0x40..0x7E
                j = i + 2
                while j < n and not 0x40 <= data[j] <= 0x7E:
                    j += 1
                i = j + 1
            elif nxt == 0x5D or nxt in (0x50, 0x58, 0x5E, 0x5F):  # OSC, DCS, SOS, PM, APC: to BEL or ST
                j = i + 2
                while j < n:
                    if data[j] == 0x07 or (data[j] == 0x1B and j + 1 < n and data[j + 1] == 0x5C):
                        break
                    j += 1
                i = j + (2 if j < n and data[j] == 0x1B else 1)
            elif 0x20 <= nxt <= 0x2F:  # intermediates, then a final byte 0x30..0x7E
                j = i + 2
                while j < n and not 0x30 <= data[j] <= 0x7E:
                    j += 1
                i = j + 1
            else:  # ESC + one byte (ESC 7, ESC 8, ESC M, ESC =, ...)
                i += 2
        elif b >= 0xF0:
            i += 4
        elif b >= 0xE0:
            i += 3
        elif b >= 0xC0:
            i += 2
        else:
            i += 1
    return safe


def chunks(data, count):
    """`count` non-empty chunks of about equal size, each ending at a safe cut."""
    cuts = safe_cuts(data)
    bounds = [0]
    for k in range(1, count):
        target = k * len(data) // count
        at = next((c for c in cuts if c >= target and c > bounds[-1]), None)
        if at is None:
            break
        bounds.append(at)
    bounds.append(len(data))
    assert len(bounds) == count + 1, f"only {len(bounds) - 1} chunks could be cut safely"
    return [data[a:b] for a, b in zip(bounds, bounds[1:])]


def bar(cols, rows, state):
    """The marker bar down the right edge, painted without moving the cursor or changing attributes."""
    r, g, b = COLORS[state % 4]
    left = cols - BAR_COLUMNS + 1
    out = [b"\x1b7"]
    for row in range(1, rows + 1):
        out.append(f"\x1b[{row};{left}H\x1b[48;2;{r};{g};{b}m".encode() + b" " * BAR_COLUMNS)
    out.append(b"\x1b[0m\x1b8")
    return b"".join(out)


def write_all(fd, data):
    view = memoryview(data)
    while view:
        n = os.write(fd, view)
        view = view[n:]


def main(argv):
    if len(argv) not in (5, 6):
        sys.exit(__doc__)
    events_path, ready, keys_ready, log_path = argv[1:5]
    fixture = Path(argv[5]) if len(argv) == 6 else Path(__file__).resolve().parent.parent / "fixtures" / "tmux-redraw.bin"
    log = open(log_path, "a")

    def note(message):
        print(f"{millis():.0f} {message}", file=log, flush=True)

    data = fixture.read_bytes()
    parts = chunks(data, OUTPUT_FRAMES)
    out, inp = sys.stdout.fileno(), sys.stdin.fileno()
    tty.setraw(inp)
    # The window is sized from outside (GNOME's maximize); wait for the grid
    # the recording was made for, so the viewport is the same as GPUI's.
    deadline = time.monotonic() + SIZE_TIMEOUT_S
    cols, rows = winsize(out)
    while (cols < COLS or rows < ROWS) and time.monotonic() < deadline:
        time.sleep(0.05)
        cols, rows = winsize(out)
    note(f"pty {cols}x{rows}" + ("" if cols >= COLS and rows >= ROWS else f" (smaller than {COLS}x{ROWS}; the recording's layout will differ)"))
    state = 0
    write_all(out, b"\x1b[0m\x1b[2J\x1b[H" + bar(cols, rows, state))
    deadline = time.monotonic() + READY_TIMEOUT_S
    while not Path(ready).exists():
        if time.monotonic() > deadline:
            note("window-capture collector did not attach; no benchmark produced")
            sys.exit(1)
        time.sleep(0.1)
    events = []
    for index, chunk in enumerate(parts):
        time.sleep(0.016)
        state += 1
        sent = millis()
        write_all(out, chunk + bar(cols, rows, state))
        events.append(("tmux", index, sent))
    while select.select([inp], [], [], 0.2)[0]:  # the terminal's answers to the recording's queries
        os.read(inp, 4096)
    Path(keys_ready).touch()
    index = 0
    while index < INPUT_FRAMES:
        readable, _, _ = select.select([inp], [], [], KEY_TIMEOUT_S)
        if not readable:
            note(f"no key arrived within {KEY_TIMEOUT_S:.0f}s for input {index}")
            break
        key = os.read(inp, 1)  # one event per key, even when several queued up
        if key != b"a":
            continue
        sent = millis()
        state += 1
        write_all(out, f"\x1b[1;1H\x1b[2KINPUT {index:03} a".encode() + bar(cols, rows, state))
        events.append(("input", index, sent))
        index += 1
    time.sleep(0.5)
    csv = "phase,index,sent_epoch_ms\n" + "".join(f"{phase},{index},{sent:.4f}\n" for phase, index, sent in events)
    Path(events_path).write_text(csv)
    note(f"nxgui: {len(events)} dispatched updates")
    time.sleep(1.5)  # Hold the last frame while the collector finishes.


if __name__ == "__main__":
    main(sys.argv)
