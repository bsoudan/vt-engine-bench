#!/usr/bin/env python3
"""Observe actual pixel changes from the GPUI window through GNOME's compositor.

Linux counterpart of capture_gpui.swift. Mutter (the GNOME Wayland compositor)
exposes org.gnome.Mutter.ScreenCast on the session bus; this reads its PipeWire
video streams with GStreamer in two phases:

1. locate: every logical monitor of the session is streamed whole until one
   frame shows the GPUI window's marker bar (a 96 logical px wide column of
   marker blue). Mutter records a window by id only, and GNOME's introspection
   API withholds ids from ordinary clients, so the window is found by its
   pixels instead. Fullscreen is not an option either: Mutter scans a
   fullscreen window out directly and its stream then starves.
2. sample: the pixel 20 px right and 100 px below the bar's corner is
   classified with the same thresholds as on macOS. Mutter caps a stream's
   frame rate at its source's rate: 60 fps for an area stream, the refresh
   rate for a monitor stream. A 60 fps cap on a 60 Hz monitor skips every
   other paint of an app that updates every 16 ms, so on a monitor faster
   than 60 Hz the monitor stream itself is sampled; otherwise a 160x160
   logical px area at the bar's corner is streamed instead, which is all a
   machine that composites in software can read back per paint.

If the session currently has no monitor (a headless session nobody is attached
to), it asks Mutter for a virtual monitor instead, which exists for as long as
this process keeps its stream open, and samples that stream directly.

Frames are taken as DMA-BUFs and read back through GStreamer's GL plugin in
this process when Mutter offers them: Mutter's own CPU readback of a 1080p
frame per paint stalls the compositor enough to starve the GPUI app of frame
callbacks on a small GPU. Where DMA-BUF does not negotiate (no GPU), the
stream falls back to CPU-side frames.

Timestamps are taken when GStreamer hands over the frame, which includes
compositor and PipeWire delivery, like ScreenCaptureKit's delivery on macOS.

Usage: capture_gpui_linux.py <events.csv> <captures.csv> <ready-file> <streaming-file>
                             [--dump-frames DIR]

  streaming-file  created once every monitor stream is delivering frames;
                  start the GPUI app after it exists.
  ready-file      created when the marker's first (blue) state is sampled; the
                  GPUI app waits for it before timing anything.
  --dump-frames   also write the first frame of each stream as a PPM image,
                  for diagnosing a run where the marker is never found.

Needs PyGObject with Gio, Gst, GstApp and GstVideo typelibs, and GStreamer's
pipewiresrc, videoconvert and appsink elements (nix/shell.nix provides them).
"""
import os
import sys
import threading
import time
from pathlib import Path

import gi

gi.require_version("Gst", "1.0")
gi.require_version("GstApp", "1.0")
gi.require_version("GstVideo", "1.0")
from gi.repository import Gio, GLib, Gst, GstApp, GstVideo  # noqa: E402

SCREEN_CAST = "org.gnome.Mutter.ScreenCast"
SCREEN_CAST_PATH = "/org/gnome/Mutter/ScreenCast"
DISPLAY_CONFIG = "org.gnome.Mutter.DisplayConfig"
DISPLAY_CONFIG_PATH = "/org/gnome/Mutter/DisplayConfig"
CURSOR_HIDDEN = 0
# Size of the virtual monitor requested when the session has none; the
# 1280x800 GPUI window fits.
VIRTUAL_SIZE = (1280, 800)
# Sampled pixel, relative to the marker bar's top-left corner, in physical
# pixels: the same offset ScreenCaptureKit samples in the window on macOS.
SAMPLE_X, SAMPLE_Y = 20, 100
# Logical size of the area streamed around the bar's corner in the sample
# phase; it contains the sampled pixel at any scale >= 1.
AREA = 160
# Marker bar: 96 logical px wide (so 96..384 physical at scales 1..4), at
# least the 40 text rows tall, and uniformly marker blue (0x3b82f6).
BAR_MIN_WIDTH, BAR_MAX_WIDTH, BAR_MIN_HEIGHT = 80, 400, 300
STARTUP_TIMEOUT_S = 60.0
RUN_TIMEOUT_S = 60.0


def classify(r, g, b):
    """Blue, red, green, yellow marker states; same thresholds as the macOS collector."""
    if b > 160 and r < 130:
        return 0
    if r > 170 and g < 125:
        return 1
    if g > 145 and r < 135:
        return 2
    if r > 160 and g > 120 and b < 145:
        return 3
    return None


def marker_blue(data, offset):
    b, g, r = data[offset], data[offset + 1], data[offset + 2]
    return abs(r - 0x3B) < 40 and abs(g - 0x82) < 40 and abs(b - 0xF6) < 40


def locate_bar(data, width, height, stride):
    """(x, y) of the marker bar's top-left corner in a BGRx frame, or None."""
    for y in range(0, height, 8):
        row = y * stride
        for x in range(0, width, 4):
            if not marker_blue(data, row + x * 4):
                continue
            left = x
            while left > 0 and marker_blue(data, row + (left - 1) * 4):
                left -= 1
            right = x
            while right + 1 < width and marker_blue(data, row + (right + 1) * 4):
                right += 1
            run = right - left + 1
            if not BAR_MIN_WIDTH <= run <= BAR_MAX_WIDTH:
                continue
            column = (left + right) // 2
            top = y
            while top > 0 and marker_blue(data, (top - 1) * stride + column * 4):
                top -= 1
            bottom = y
            while bottom + 1 < height and marker_blue(data, (bottom + 1) * stride + column * 4):
                bottom += 1
            if bottom - top + 1 >= BAR_MIN_HEIGHT:
                return left, top
            break  # Something else blue on this row; try the next row.
    return None


class Collector:
    """Same state machine as capture_gpui.swift's Collector."""

    def __init__(self, ready):
        self.ready = ready
        self.lock = threading.Lock()
        self.previous = None
        self.changes = []
        self.frames = 0

    def observe(self, stream, color):
        with self.lock:
            if color is None:
                return
            self.frames += 1
            if self.previous is None:
                if color != 0:
                    return
                self.previous = 0
                Path(self.ready).touch()
                print(f"marker sampled on {stream.describe()}", file=sys.stderr, flush=True)
            elif self.previous % 4 != color:
                nxt = self.previous + 1
                while nxt % 4 != color:
                    nxt += 1
                self.previous = nxt
                self.changes.append((nxt, time.time() * 1000.0))

    def snapshot(self):
        with self.lock:
            return list(self.changes), self.frames


class Stream:
    """One Mutter stream object and the GStreamer pipeline reading its PipeWire node.

    A stream starts in the locate phase (scanning frames for the marker bar)
    unless given a sample point, and switches to sampling when told where.
    """

    def __init__(self, bus, path, label, collector, dump_dir, size=None, sample=None):
        self.bus = bus
        self.path = path
        self.label = label
        self.collector = collector
        self.dump_dir = dump_dir
        self.size = size
        self.sample = sample  # (x, y) physical pixels to classify, or None while locating
        self.found = None  # (x, y) of the bar's corner once located
        self.found_color = None  # the sample point's state in that same frame
        self.node = None
        self.pipeline = None
        self.caps = None
        self.width = self.height = self.stride = None
        self.frames = 0
        self.dumped = False
        self.streaming = threading.Event()
        bus.signal_subscribe(
            SCREEN_CAST, f"{SCREEN_CAST}.Stream", "PipeWireStreamAdded", path, None,
            Gio.DBusSignalFlags.NONE, self._on_node_added, None,
        )

    def describe(self):
        return f"{self.label} (pipewire node {self.node}, {self.width}x{self.height})"

    def _on_node_added(self, _conn, _sender, _path, _iface, _signal, params, _data):
        (self.node,) = params.unpack()
        constraint = f",width={self.size[0]},height={self.size[1]}" if self.size else ""
        # Without max-framerate in the caps PipeWire takes Mutter's default,
        # the top of the range Mutter offers, which is the source's own rate
        # (a monitor's refresh rate; 60 for an area or virtual stream). A
        # fixed value above that fails to negotiate and a GStreamer range
        # ends up at its low end. Below the cap Mutter records at most one
        # frame per interval and queues a follow-up repaint for a paint that
        # comes sooner, which on a 120 Hz display lands two or three vblanks
        # later and loses updates in between: a lower cap (65/1 was tried)
        # captures fewer updates, not more, so the cap is left at the top.
        sink = "appsink name=sink emit-signals=true sync=false max-buffers=16 drop=true"
        self.descriptions = []
        if Gst.ElementFactory.find("glupload") is not None and not os.environ.get("VT_CAPTURE_NO_DMABUF"):
            # GPU frames, read back here rather than by the compositor.
            self.descriptions.append(
                f"pipewiresrc path={self.node} ! video/x-raw(memory:DMABuf){constraint} ! "
                f"glupload ! glcolorconvert ! gldownload ! video/x-raw,format=BGRx ! {sink}")
        self.descriptions.append(
            f"pipewiresrc path={self.node} ! videoconvert ! video/x-raw,format=BGRx{constraint} ! {sink}")
        self._start_pipeline()

    def _start_pipeline(self):
        description = self.descriptions.pop(0)
        self.via = "DMA-BUF via GL" if "glupload" in description else "CPU-side"
        self.pipeline = Gst.parse_launch(description)
        self.pipeline.get_by_name("sink").connect("new-sample", self._on_sample)
        self.pipeline.get_bus().add_signal_watch()
        self.pipeline.get_bus().connect("message::error", self._on_error)
        self.pipeline.set_state(Gst.State.PLAYING)

    def _on_error(self, _bus, message):
        error, debug = message.parse_error()
        if self.descriptions and self.frames == 0:
            print(f"{self.label}: no DMA-BUF stream ({error.message}); reading CPU-side frames instead",
                  file=sys.stderr, flush=True)
            self.pipeline.set_state(Gst.State.NULL)
            self._start_pipeline()
            return
        sys.exit(f"GStreamer error on {self.label}: {error.message} ({debug})")

    def _on_sample(self, sink):
        sample = sink.emit("pull-sample")
        if sample is None:
            return Gst.FlowReturn.OK
        buf = sample.get_buffer()
        caps = sample.get_caps()
        if self.caps is None or not caps.is_equal(self.caps):
            structure = caps.get_structure(0)
            self.caps = caps
            self.width, self.height = structure.get_value("width"), structure.get_value("height")
            self.stride = self.width * 4
            _ok, num, denom = structure.get_fraction("max-framerate")
            print(f"{self.label}: {self.width}x{self.height} {structure.get_value('format')}, "
                  f"up to {num / max(denom, 1):.2f} frames/s, {self.via} frames", file=sys.stderr, flush=True)
        meta = GstVideo.buffer_get_video_meta(buf)
        stride = meta.stride[0] if meta else self.stride
        self.frames += 1
        self.streaming.set()
        if self.dump_dir and not self.dumped:
            self._with_frame(buf, stride, self._dump)
        if self.sample is not None:
            x, y = self.sample
            if x < self.width and y < self.height:
                pixel = buf.extract_dup(y * stride + x * 4, 4)
                self.collector.observe(self, classify(pixel[2], pixel[1], pixel[0]))
        elif self.found is None:
            self._with_frame(buf, stride, self._locate)
        return Gst.FlowReturn.OK

    def _with_frame(self, buf, stride, action):
        ok, info = buf.map(Gst.MapFlags.READ)
        if not ok:
            return
        try:
            action(info.data, stride)
        finally:
            buf.unmap(info)

    def _locate(self, data, stride):
        corner = locate_bar(data, self.width, self.height, stride)
        if corner is not None:
            # Classify the sample point in this very frame too: a window that
            # sits still paints nothing more, so there may be no next frame.
            at = (corner[1] + SAMPLE_Y) * stride + (corner[0] + SAMPLE_X) * 4
            self.found_color = classify(data[at + 2], data[at + 1], data[at])
            self.found = corner
            print(f"marker bar at {corner} on {self.describe()}", file=sys.stderr, flush=True)

    def sample_here(self, x, y):
        """Switch this stream from locating to sampling (x, y), counting the located frame."""
        self.sample = (x, y)
        self.collector.observe(self, self.found_color)

    def _dump(self, data, stride):
        self.dumped = True
        rows = bytearray()
        for y in range(self.height):
            row = data[y * stride:y * stride + self.width * 4]
            rows += bytes(b for px in range(0, len(row), 4) for b in (row[px + 2], row[px + 1], row[px]))
        target = Path(self.dump_dir) / f"{self.label.replace('/', '_').replace(' ', '-')}.ppm"
        target.write_bytes(f"P6\n{self.width} {self.height}\n255\n".encode() + rows)
        print(f"{self.label}: first frame written to {target}", file=sys.stderr, flush=True)

    def stop(self):
        if self.pipeline is not None:
            self.pipeline.set_state(Gst.State.NULL)


def call(bus, path, interface, method, params=None):
    return bus.call_sync(
        SCREEN_CAST if interface.startswith(SCREEN_CAST) else DISPLAY_CONFIG,
        path, interface, method, params, None, Gio.DBusCallFlags.NONE, 10_000, None,
    )


def session_locked(bus):
    try:
        (active,) = bus.call_sync(
            "org.gnome.ScreenSaver", "/org/gnome/ScreenSaver", "org.gnome.ScreenSaver", "GetActive",
            None, None, Gio.DBusCallFlags.NONE, 5_000, None,
        ).unpack()
    except GLib.Error:
        return False
    return bool(active)


def logical_monitors(bus):
    """[(connector, x, y, scale, primary, refresh_hz)] from org.gnome.Mutter.DisplayConfig."""
    _serial, monitors, logical, _props = call(bus, DISPLAY_CONFIG_PATH, DISPLAY_CONFIG, "GetCurrentState").unpack()
    refresh = {}
    for (connector, *_ids), modes, _props in monitors:
        for _mode_id, _w, _h, hz, _scale, _scales, props in modes:
            if props.get("is-current"):
                refresh[connector] = hz
    return [(monitors[0][0], x, y, scale, primary, refresh.get(monitors[0][0], 0.0))
            for x, y, scale, _transform, primary, monitors, _props in logical]


def wait_for(predicate, timeout, what):
    deadline = time.monotonic() + timeout
    context = GLib.MainContext.default()
    while not predicate():
        if time.monotonic() > deadline:
            sys.exit(f"timed out after {timeout:.0f}s waiting for {what}")
        context.iteration(False)
        time.sleep(0.005)


class Session:
    """One org.gnome.Mutter.ScreenCast session; it lives as long as this D-Bus connection."""

    IFACE = f"{SCREEN_CAST}.Session"

    def __init__(self, bus):
        self.bus = bus
        # Animations off so windows map without a fade that blends the marker's color.
        (self.path,) = call(bus, SCREEN_CAST_PATH, SCREEN_CAST, "CreateSession",
                            GLib.Variant("(a{sv})", ({"disable-animations": GLib.Variant("b", True)},))).unpack()
        self.streams = []

    def record(self, method, *args):
        properties = {"cursor-mode": GLib.Variant("u", CURSOR_HIDDEN)}
        signature = {"RecordMonitor": "(sa{sv})", "RecordArea": "(iiiia{sv})", "RecordVirtual": "(a{sv})"}[method]
        (path,) = call(self.bus, self.path, self.IFACE, method, GLib.Variant(signature, (*args, properties))).unpack()
        return path

    def start(self):
        call(self.bus, self.path, self.IFACE, "Start")

    def stop(self):
        for stream in self.streams:
            stream.stop()
        try:
            call(self.bus, self.path, self.IFACE, "Stop")
        except GLib.Error:
            pass  # Mutter closes the session itself when its streams die.


def main(argv):
    dump_dir = None
    if "--dump-frames" in argv:
        at = argv.index("--dump-frames")
        dump_dir = argv[at + 1]
        argv = argv[:at] + argv[at + 2:]
        os.makedirs(dump_dir, exist_ok=True)
    if len(argv) != 5:
        sys.exit(__doc__)
    events, output, ready, streaming = argv[1:5]
    Gst.init(None)
    for element in ("pipewiresrc", "videoconvert", "appsink"):
        if Gst.ElementFactory.find(element) is None:
            sys.exit(f"GStreamer element {element} is not available; check GST_PLUGIN_SYSTEM_PATH_1_0")
    bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
    if session_locked(bus):
        # The lock screen hides every application window, so nothing could be observed.
        sys.exit("the GNOME session is locked; unlock it before running the GPUI benchmark")
    collector = Collector(ready)
    sessions = []
    try:
        locate = Session(bus)
        sessions.append(locate)
        monitors = logical_monitors(bus)
        if monitors:
            for connector, x, y, scale, primary, hz in monitors:
                label = f"monitor {connector} at ({x},{y}) scale {scale:g} {hz:.2f} Hz{' primary' if primary else ''}"
                locate.streams.append(Stream(bus, locate.record("RecordMonitor", connector), label, collector, dump_dir))
        else:
            print("GNOME session has no monitor; recording a Mutter virtual monitor", file=sys.stderr, flush=True)
            label = f"virtual monitor {VIRTUAL_SIZE[0]}x{VIRTUAL_SIZE[1]}"
            locate.streams.append(Stream(bus, locate.record("RecordVirtual"), label, collector, dump_dir, size=VIRTUAL_SIZE))
        locate.start()
        print("locating the window on: " + "; ".join(s.label for s in locate.streams), file=sys.stderr, flush=True)
        wait_for(lambda: all(s.streaming.is_set() for s in locate.streams), STARTUP_TIMEOUT_S, "the first frame of every stream")
        Path(streaming).touch()
        wait_for(lambda: any(s.found for s in locate.streams), STARTUP_TIMEOUT_S,
                 "the GPUI window's marker bar (is the window on a recorded monitor, with nothing covering it?)")
        source = next(s for s in locate.streams if s.found)
        bar_x, bar_y = source.found
        monitor = next((m for m in monitors if m[0] == source.label.split()[1]), None)
        if monitor is not None and monitor[5] > 60.5 and not os.environ.get("VT_CAPTURE_AREA"):
            # The monitor stream records every paint; keep sampling it.
            source.sample_here(bar_x + SAMPLE_X, bar_y + SAMPLE_Y)
        elif monitor is not None:
            # Phase 2: a small area at the bar's corner, in logical coordinates.
            connector, mon_x, mon_y, scale, _primary, _hz = monitor
            area_x, area_y = mon_x + int(bar_x / scale), mon_y + int(bar_y / scale)
            sample = Session(bus)
            sessions.append(sample)
            label = f"{AREA}x{AREA} area at ({area_x},{area_y}) of monitor {connector}"
            # The area's origin may land a pixel inside the bar after the
            # scale rounding; the sample point is far enough in not to care.
            sample.streams.append(Stream(bus, sample.record("RecordArea", area_x, area_y, AREA, AREA), label,
                                         collector, dump_dir, sample=(SAMPLE_X, SAMPLE_Y)))
            sample.start()
            wait_for(lambda: sample.streams[0].streaming.is_set(), STARTUP_TIMEOUT_S, "the first frame of the area stream")
            locate.stop()
        else:
            source.sample_here(bar_x + SAMPLE_X, bar_y + SAMPLE_Y)
        wait_for(lambda: Path(ready).exists(), STARTUP_TIMEOUT_S, "the marker's blue state at the sample point")
        wait_for(lambda: Path(events).exists(), RUN_TIMEOUT_S, f"the GPUI app to write {events}")
        time.sleep(0.3)  # Frames in flight for the last update, as on macOS.
    finally:
        for session in sessions:
            session.stop()
    changes, frames = collector.snapshot()
    csv = "sequence,observed_epoch_ms\n" + "".join(f"{sequence},{t:.4f}\n" for sequence, t in changes)
    Path(output).write_text(csv)
    sampled = next(s for session in sessions for s in session.streams if s.sample is not None)
    print(f"captured {len(changes)} marker changes from {frames} marker frames "
          f"({sampled.frames} PipeWire frames on {sampled.describe()})")


if __name__ == "__main__":
    main(sys.argv)
