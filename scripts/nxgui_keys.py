#!/usr/bin/env python3
"""Inject keys into the GNOME session through Mutter's remote-desktop API.

The GPUI app dispatches its synthetic keys inside itself; nxgui is a GTK
window, so its keys come from GNOME: org.gnome.Mutter.RemoteDesktop takes
evdev keycodes and delivers them to the focused window, which is nxgui's
when it was the last one opened.

Usage: nxgui_keys.py maximize
           Super+Up: GNOME's maximize shortcut, for the window to reach the
           120x40 grid the recording was made for.
       nxgui_keys.py type <keys-ready-file> <keys.csv> [count] [interval-ms]
           Waits for the feeder's keys-ready file, then presses `a` count
           times (40) at the interval (40 ms), writing each injection's time
           to keys.csv (index,injected_epoch_ms).
"""
import sys
import time
from pathlib import Path

from gi.repository import Gio, GLib

REMOTE_DESKTOP = "org.gnome.Mutter.RemoteDesktop"
KEY_A, KEY_UP, KEY_LEFTMETA = 30, 103, 125  # linux/input-event-codes.h
READY_TIMEOUT_S = 120.0


class Keyboard:
    def __init__(self):
        self.bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
        (self.session,) = self.call("/org/gnome/Mutter/RemoteDesktop", REMOTE_DESKTOP, "CreateSession").unpack()
        self.call(self.session, f"{REMOTE_DESKTOP}.Session", "Start")

    def call(self, path, interface, method, params=None):
        return self.bus.call_sync(REMOTE_DESKTOP, path, interface, method, params, None, Gio.DBusCallFlags.NONE, 5_000, None)

    def key(self, keycode, pressed):
        self.call(self.session, f"{REMOTE_DESKTOP}.Session", "NotifyKeyboardKeycode", GLib.Variant("(ub)", (keycode, pressed)))

    def tap(self, *keycodes):
        for code in keycodes:
            self.key(code, True)
        for code in reversed(keycodes):
            self.key(code, False)

    def close(self):
        try:
            self.call(self.session, f"{REMOTE_DESKTOP}.Session", "Stop")
        except GLib.Error:
            pass


def main(argv):
    if len(argv) >= 2 and argv[1] == "maximize" and len(argv) == 2:
        keyboard = Keyboard()
        keyboard.tap(KEY_LEFTMETA, KEY_UP)
        keyboard.close()
        return
    if len(argv) >= 4 and argv[1] == "type":
        keys_ready, output = argv[2], argv[3]
        count = int(argv[4]) if len(argv) > 4 else 40
        interval = (float(argv[5]) if len(argv) > 5 else 40.0) / 1000.0
        deadline = time.monotonic() + READY_TIMEOUT_S
        while not Path(keys_ready).exists():
            if time.monotonic() > deadline:
                sys.exit("the feeder never asked for keys")
            time.sleep(0.05)
        keyboard = Keyboard()
        times = []
        for index in range(count):
            time.sleep(interval)
            times.append((index, time.time() * 1000.0))
            keyboard.tap(KEY_A)
        keyboard.close()
        Path(output).write_text("index,injected_epoch_ms\n" + "".join(f"{i},{t:.4f}\n" for i, t in times))
        return
    sys.exit(__doc__)


if __name__ == "__main__":
    main(sys.argv)
