# VT engine benchmark (macOS and Linux)

Two separate measurements: **headless parsing + screen-state** and **a shared GPUI viewport observed through the OS screen-capture path** (ScreenCaptureKit on macOS, Mutter's screencast API on Linux/GNOME); on Linux the second also measures **nxgui**, a real GNOME terminal window, the same way. Sources are pinned as git submodules (see `git submodule status` and `Cargo.lock`), except nxterm's engine, the Go package `te`, which is a fifth headless engine vendored in [`engines/nx-te`](engines/nx-te/README.md) from nxterm `v0.5.1` and changed there for throughput, commit by commit: `crates/nxterm-vt` builds that copy as a C archive with `go build -buildmode=c-archive` (Go on `PATH`; the Nix shell provides it, on macOS `brew install go`) behind a shim as narrow as Ghostty's. `engines/nxterm` itself is a private repository reached over ssh; only the nxgui window-capture run below needs it, so `git submodule update` may skip it. Ghostty is pinned to current `main` at [`b699ea7`](https://github.com/ghostty-org/ghostty/commit/b699ea79f4b881421b4b3055abc16a0957d76beb) (1.3.2-dev). Its benchmark-only C shim is compiled against that checkout's headers and links the matching `libghostty-vt` archive; `libghostty-rs` supplies the Zig build integration, **not** its older Rust API. The other engines are pinned source checkouts. Parser-only `vte` is excluded because it has no screen state.

```sh
git submodule update --init --recursive
./bootstrap-zig.sh # Zig 0.16.0 (macOS)
PATH="$PWD/.context/zig:$PATH" cargo test
PATH="$PWD/.context/zig:$PATH" cargo run --release
```

On Linux, `./enter` opens the Nix dev shell (`flake.nix`, `nix/shell.nix`) with Rust, Zig 0.16, the libraries GPUI's Wayland backend links or dlopens (xkbcommon, libxcb, FreeType, Fontconfig, Wayland, the Vulkan loader), and the PyGObject + GStreamer + PipeWire stack the Linux window-capture collector uses; inside it, run `cargo test`, `cargo run --release` and `bash scripts/run_gpui_bench.sh` as they are, without the `PATH` prefix. The shell runs in a read-only sandbox of the system (`nix/sandbox.nix`); the scripts still reach the desktop session because they name its sockets by path. GPUI's dependency tree needs one Linux fix: `crates/xattr` is the `xattr` 0.2.3 release with its removed-from-libc `ENOATTR` alias replaced, substituted through `[patch.crates-io]`.

`cargo run --release -- --csv FILE --run N` also appends the run's medians to FILE (`run,workload,engine,median_ms`); `python3 scripts/summarize_headless.py FILE` prints the median-of-runs table below from three such runs. Three workloads are generated in `src/main.rs`: 15k plain scroll lines, 15k 256-color lines, and 10k cursor-positioned/cleared TUI updates. A fourth is a **real tmux client PTY recording** (`fixtures/tmux-redraw.bin`, 25,641 bytes, SHA-256 `d30383bb7658ae386f6f8e70e7d26bc8f1484b95d1199cbaf81715ad7233b7da`) captured by `python3 scripts/capture_tmux.py`, replayed ten times for timing. Re-capturing changes the fixture; the committed bytes define this run. The capture uses `/bin/sh`, a temporary socket and config, and disables tmux's status line to avoid machine-identifying data.

120×40 cells, 1k scrollback in each engine where configurable, chunks of 8192 bytes. Seven runs per workload/engine, median wall time includes creating a fresh model and dropping it; throughput uses input bytes. No GUI, live PTY during timing, glyph rendering, font shaping, or latency. Run when machine is otherwise idle. Higher throughput here **does not mean smoother or faster terminal UI**. Different protocol feature sets and implementation paths mean this is exploratory, not a compliance-normalized comparison. The Ghostty C shim intentionally exposes only APIs the benchmark tests; do not treat it as a general-purpose Rust binding. `cargo test` checks **all four engines reproduce the exact expected visible text** (the `frame 0150` header and 38 numbered lines) after the same 8192-byte chunking and ten replays used in timing. Additional small-screen fixtures in `src/compat.rs` check the behaviors below with 7-byte chunking (splitting ANSI escapes and UTF-8). These checks do not establish broad VT compliance. The previous recording included a shell prompt after the last frame; it caused a scroll and divergent end state. The capture script now keeps the command running while recording the final frame, removing that ambiguous post-command transition. Replaying ten times also repeats initialization sequences; it is not a continuous tmux session.

## Core compatibility checks

| Check | Alacritty | Ghostty main | vt100 | WezTerm | nxterm te |
|---|:---:|:---:|:---:|:---:|:---:|
| Repeated tmux client: exact visible text | ✓ | ✓ | ✓ | ✓ | ✓ |
| Cursor addressing and line clear | ✓ | ✓ | ✓ | ✓ | ✓ |
| Truecolor foreground/background and bold | ✓ | ✓ | ✓ | ✓ | ✓ |
| Indexed color and alternate-screen restore | ✓ | ✓ | ✓ | ✓ | ✓ |
| Wide Unicode and combining marks | ✓ | ✓ | ✓ | ✓ | ✓ |
| Preserve text on resize | ✓ | ✓ | ✓ | ✓ | ✓ |
| Retain and inspect scrollback | ✓ | ✓ | ✓ | ✓ | ✓ |

Only the **specific fixtures** pass; this is not a complete VT conformance suite. In particular, mouse input, selection, OSC clipboard, hyperlinks, grapheme clustering, error handling, exact cursor shape, rendering, and latency remain untested. The Ghostty column tests the pinned current-main C API through a narrow benchmark shim, not the full macOS app; the nxterm column tests its `te` package the same way, through `crates/nxterm-vt`, not nxgui or nxterm.

## Recorded run (Apple M1 Pro, macOS 15.4.1, release build, 2026-10-06)

| Workload | alacritty_terminal | Ghostty main | vt100 | wezterm-term |
|---|---:|---:|---:|---:|
| plain scroll, ms | 5.98 | **1.34** | 7.93 | 26.24 |
| ANSI scroll, ms | **4.67** | 6.18 | 6.06 | 14.64 |
| TUI redraw, ms | 3.56 | **2.66** | 3.51 | 25.17 |
| tmux client (repeated recording), ms | **2.14** | 6.02 | 8.54 | 138.26 |

[Raw per-run medians](results/2026-10-06-m1-pro-current-ghostty.csv) are included. Values are the **median of three independent process runs**, each reporting the median of seven iterations. This is not a controlled system-wide benchmark. [Previous runs](results/2026-10-06-m1-pro.csv) used Ghostty 1.2.3 and Alacritty's 10k-line default scrollback; **do not compare them directly** to these 1k-line results.

**Do not select a GPUI terminal core on these numbers alone.** All four pass the bounded core fixtures; none is proven the fastest rendered terminal. Ghostty leads plain output and synthetic TUI redraws; Alacritty leads this recorded tmux stream and ANSI output.

### Recorded run (Intel i3-4130T, NixOS, GNOME desktop idle, release build, 2026-11-07)

Three runs of `cargo run --release -- --csv results/2026-11-07-headless-alpha.csv --run N`, the table by `python3 scripts/summarize_headless.py` on that [CSV](results/2026-11-07-headless-alpha.csv): the median of the three per-run medians. The fifth column is nxterm's `te` engine (v0.5.1) through `crates/nxterm-vt`. A 2.9 GHz Haswell core is about twice as slow as the M1 Pro above on every engine; compare columns within a row, not with the macOS table.

| Workload | alacritty_terminal | Ghostty main | vt100 | wezterm-term | nxterm te |
|---|---:|---:|---:|---:|---:|
| plain scroll, ms | 12.94 | **3.20** | 18.75 | 46.17 | 453.33 |
| ANSI scroll, ms | **9.30** | 19.95 | 15.25 | 25.14 | 260.42 |
| TUI redraw, ms | 6.04 | **4.21** | 7.24 | 44.75 | 108.22 |
| tmux client (repeated recording), ms | **3.58** | 7.41 | 22.94 | 230.05 | 243.39 |

nxterm's engine passes the same fixtures but parses at about 1.7 MiB/s on the plain scroll, ten to a hundred times slower than the Rust and Zig engines. That is the engine, not the shim: fed from Go directly, without cgo, the same workload takes 437 ms with a 1000-line scrollback and 380 ms with none. It is a pure-Go emulator written for a multiplexer that cuts a pty's output into frames at most every 16 ms, where this throughput is far above what a pty delivers; the comparison is of parse cost, as for the others.

### Recorded run with the vendored, tuned te (same machine, 2026-11-08)

The same three runs ([CSV](results/2026-11-08-headless-alpha-nx-te.csv)) after the `engines/nx-te` commits: an ASCII fast path in `Draw` that skips grapheme segmentation and makes no string per cell, rows recycled between the screen and the history instead of allocated and copied, the erased cell worked out once per erase, CSI parameters read into a reused byte buffer, text runs handed to `Draw` as the bytes they are, grapheme clusters stepped without word and line break state, and the five modes the hot paths read kept as booleans beside the map. Each commit keeps te's 766 tests and `cargo test` green; `TestDrawASCIIMatchesGeneral` draws random mixed text through the fast path and the general path and wants the same screen. Profiles of this state are bound by the cell: `cell.Cell` is 64 bytes (a Go string plus inline attributes), so a scrolled line fills 7.7 KB and a full-screen erase writes 307 KB, which is six tenths of the plain scroll and of the tmux recording. Closing more of the gap to Ghostty's 8-byte cells means changing nxterm's shared cell model, not te alone.

| Workload | alacritty_terminal | Ghostty main | vt100 | wezterm-term | nxterm te (nx-te) |
|---|---:|---:|---:|---:|---:|
| plain scroll, ms | 12.99 | **3.17** | 18.71 | 45.92 | 36.06 |
| ANSI scroll, ms | **9.22** | 19.92 | 15.37 | 25.19 | 31.47 |
| TUI redraw, ms | 6.04 | **4.22** | 7.09 | 44.92 | 25.03 |
| tmux client (repeated recording), ms | **3.60** | 7.49 | 22.86 | 231.20 | 35.26 |

### Recorded run with eight-byte cells (same machine, 2026-11-08)

The same three runs ([CSV](results/2026-11-08-headless-alpha-nx-te-cells.csv)) after the next twelve `engines/nx-te` commits, which take Ghostty's cell layout: the screen stores each cell as eight bytes (a codepoint or an index into a per-screen table of multi-codepoint clusters, an index into a per-screen table of distinct attributes, and the wrap and empty flags), and `cell.Cell` is translated at the edges, so the tests, snapshots and deltas see no change. The tables carry the hints Ghostty keeps per row: no erase reads a cell until some style on the screen protects one, a draw works out its style's index once, the width of each character is remembered, and a character that is a grapheme cluster on its own (most non-ASCII text) is drawn without the segmenter. The remaining commits draw ASCII a row at a time, hand `Draw` a view of the input rather than a copy, and trim the CSI and SGR paths. Allocation per run falls to about a thousand objects.

| Workload | alacritty_terminal | Ghostty main | vt100 | wezterm-term | nxterm te (nx-te) |
|---|---:|---:|---:|---:|---:|
| plain scroll, ms | 12.97 | **3.17** | 18.90 | 46.42 | 6.12 |
| ANSI scroll, ms | **9.31** | 19.97 | 15.32 | 25.35 | 10.45 |
| TUI redraw, ms | 6.06 | **4.24** | 7.13 | 45.19 | 10.00 |
| tmux client (repeated recording), ms | **3.58** | 7.42 | 22.77 | 229.66 | 7.34 |

te now leads Ghostty on the 256-color scroll and matches it on the tmux recording, and trails it by two times on plain scroll and the TUI redraw; it is within two times of Alacritty everywhere but the tmux recording. What remains in the profiles is spread thin: hashing a 40-byte attribute to find its style index when the color changes every line, a byte-at-a-time CSI state machine against Ghostty's table-driven one and SIMD scan, and the memory moves of scrolling itself.

## GPUI window capture (Apple M1 Pro, 2026-10-08)

Run `bash scripts/run_gpui_bench.sh` from a logged-in macOS desktop with Screen Recording permission (Linux: see below). It builds a **single shared GPUI text renderer** for all four engines, launches each in a visible window, and uses ScreenCaptureKit to observe a color marker changing alongside the rendered 120×40 tmux viewport. It feeds the committed tmux recording in 80 chunks at ~16 ms intervals, then dispatches 40 synthetic GPUI key events at ~40 ms intervals. The app checks that all four engines end with the same expected tmux viewport; a run is rejected if capture loses too many updates or timestamps do not align. No PTY, text attributes, cursor painting, hardware keyboard, or glyph-by-glyph screen validation is included in this rendering harness.

Each figure below is the **median of three independent runs**; ms is lower-is-better. `event→pixel` times from feeding bytes (or the synthetic key handler) until ScreenCaptureKit delivers a frame containing the new marker; it includes capture delivery and is **not physical key-to-photon latency**. Capture intervals time *changed observed frames*, not GPUI's internal presentation callback; ScreenCaptureKit itself may drop frames. Do not compare these times to the headless table.

| GPUI metric | Alacritty | Ghostty main | vt100 | WezTerm |
|---|---:|---:|---:|---:|
| Tmux observed updates / 80 | 77 | 77 | 77 | 77 |
| Tmux event→pixel median, ms | 38.72 | 34.32 | 23.26 | 26.30 |
| Tmux event→pixel p95, ms | 46.84 | 41.82 | 30.96 | 45.16 |
| Tmux changed-frame interval median, ms | 17.22 | 17.39 | 17.32 | 17.26 |
| Tmux changed-frame interval p95, ms | 19.96 | 19.24 | 19.58 | 22.46 |
| Synthetic key→pixel median, ms | 20.54 | 18.44 | 20.72 | 20.31 |
| Synthetic key→pixel p95, ms | 26.49 | 25.09 | 24.75 | 25.70 |

[Per-run summary](results/2026-10-08-gpui-m1-pro.csv) · [Raw event timestamps, captured frames, and logs](results/2026-10-08-gpui-m1-pro/). Regenerate the summary with `python3 scripts/summarize_gpui.py --aggregate results/2026-10-08-gpui-m1-pro`. No single engine wins every meaningful metric; the synthetic key medians differ by only a few ms. These results benchmark **this common simplified renderer**, not a finished GPUI terminal or Ghostty/Alacritty's native renderer. Real tmux-with-styles and hardware input-to-photon tests remain open.

### GPUI window capture on Linux (GNOME)

The same `bash scripts/run_gpui_bench.sh` runs on Linux against a running, **unlocked** GNOME Wayland session on the same machine. It can be started from any shell, including over ssh: it finds the session's Wayland, D-Bus and PipeWire sockets under `/run/user/$UID` when the usual variables are unset, and holds a GNOME idle inhibitor so the session cannot lock itself mid-benchmark. `scripts/capture_gpui_linux.py` stands in for the ScreenCaptureKit collector. It asks Mutter (`org.gnome.Mutter.ScreenCast`) for a PipeWire video stream of every logical monitor, finds the window by its 96 px marker bar in the frames (Mutter records a window by an id that GNOME's introspection API withholds from ordinary clients), then samples the pixel 20 px right and 100 px below the bar's corner with the same four-color thresholds. The window is an ordinary 1280×800 window, not fullscreen: Mutter scans a fullscreen window out directly and its screencast stream then starves. On a monitor above 60 Hz the collector keeps sampling the monitor stream, which Mutter records at the refresh rate; on a 60 Hz monitor it switches to a 160×160 area stream at the bar's corner, because a full-frame CPU readback per paint is what starves the stream on a machine that composites in software. Frames are taken as DMA-BUFs and read back through GStreamer's GL plugin in the collector's own process when Mutter offers them, with a CPU-side fallback. A session with no monitor at all (headless, nobody attached) gets a 1280×800 Mutter virtual monitor for the duration of each run. The collector refuses to run while the session is locked, and `--dump-frames DIR` saves each stream's first frame as a PPM when the marker is never found. The text font is DejaVu Sans Mono instead of Menlo (`VT_BENCH_FONT` overrides either).

A run whose capture did not keep up (the same `summarize_gpui.py` checks as on macOS) is repeated, up to `GPUI_BENCH_ATTEMPTS` times (default 3); every attempt's files stay in the results directory, renamed `<engine>-<run>-attempt<n>-*`. `VT_CAPTURE_AREA=1` forces the area stream and `VT_CAPTURE_NO_DMABUF=1` the CPU-side frames, for machines where the defaults misbehave.

**The display must refresh faster than 60 Hz for a valid run.** Mutter records a stream at most once per stream interval, and its cap is the source's rate: the monitor's refresh rate for a monitor stream, 60 for an area, window or virtual stream. The app sends an update every 16 ms, and a 60 fps cap on a 60 Hz display skips every other paint (the capture interval alternates 17 and 33 ms); on a 120 Hz display an area stream settles at one frame per three vblanks. Only a monitor stream on a faster display records every update. A 4K TV mode at 30 Hz, the GNOME default for the TV below, cannot work at all; the GNOME display settings or `org.gnome.Mutter.DisplayConfig.ApplyMonitorsConfig` switch it for the run. On a machine without a GPU (software compositing, a remote-desktop client reading back every frame) the capture also fell to roughly 40–45 paints per second with a 60 Hz virtual monitor, so valid runs there are a matter of luck.

Timestamps are taken when GStreamer hands over the frame, so `event→pixel` includes compositor, PipeWire and GL readback delivery just as it includes ScreenCaptureKit delivery on macOS, and it is still not physical key-to-photon latency. **Linux numbers are not comparable with the macOS table above**: different compositor, GPU, refresh rate, font and window size.

#### Recorded run (Intel i3-4130T, GeForce GTX 860M, NVIDIA 580.178, GNOME Shell 50.5 on NixOS, LG TV over HDMI at 1920×1080 119.88 Hz, 2026-11-07)

Each figure is the **median of three independent runs**; ms is lower-is-better. One vt100 run was repeated once (65 of 80 updates captured on the first attempt). The nxgui column (see below) was measured in a separate invocation the same day with the same settings; its key→pixel is from the key reaching the program in the terminal, and its observed-update counts are its own repaint rate, not capture loss.

| GPUI metric | Alacritty | Ghostty main | vt100 | WezTerm | nxgui 0.5.1 |
|---|---:|---:|---:|---:|---:|
| Tmux observed updates / 80 | 76 | 79 | 79 | 75 | 57 |
| Tmux event→pixel median, ms | 17.98 | 17.48 | 19.22 | 16.99 | 49.81 |
| Tmux event→pixel p95, ms | 30.88 | 28.32 | 26.84 | 25.58 | 66.07 |
| Tmux changed-frame interval median, ms | 16.67 | 16.66 | 16.67 | 16.69 | 24.50 |
| Tmux changed-frame interval p95, ms | 27.87 | 26.28 | 26.62 | 25.37 | 31.34 |
| Synthetic key→pixel median, ms | 10.58 | 9.94 | 13.21 | 10.40 | 56.81 |
| Synthetic key→pixel p95, ms | 14.63 | 13.28 | 17.22 | 14.14 | 67.00 |

[Per-run summary](results/2026-11-07-gpui-alpha-gtx860m.csv) · [Raw event timestamps, captured frames, and logs](results/2026-11-07-gpui-alpha-gtx860m/). The four engines sit within a few ms of each other on every metric, as on macOS; the tmux p95 spread comes from the 120 Hz compositor occasionally coalescing two updates into one paint. The machine had nothing else running on its desktop.

#### nxgui: a real GNOME terminal window under the same capture

[nxterm](https://github.com/nxterm/nxterm) (`engines/nxterm`, pinned to the `v0.5.1` tag) is a Go terminal multiplexer: an `nxterm` process owns a program's pty and terminal state (its `te` engine), and `nxgui`, a GTK 4 window, paints what it is sent. On Linux the run script measures it as a fifth subject, as the terminal a user would run rather than an engine inside the shared GPUI renderer. It builds `nxgui` and `nxterm` from the submodule with `go build -tags gtk` (Go, GTK 4 and libadwaita come from the Nix shell), then for each run:

- starts the collector, then an `nxterm` vtapp (session `bench`, in a private `XDG_RUNTIME_DIR` under the results directory, so it meets no nx session of the user's) whose program is `scripts/nxgui_feed.py`, then `nxgui -font "DejaVu Sans Mono 10"`, and maximizes the window with GNOME's own Super+Up shortcut, injected through Mutter's remote-desktop API by `scripts/nxgui_keys.py`, because the window opens at 100×30 cells and the recording was made for 120×40;
- the feeder waits for a pty of at least 120×40, clears the screen and paints the marker in the terminal's own cells: a 16-column bar of marker-blue spaces down the right edge, which the collector locates and samples exactly as it does the GPUI window's bar. It then writes the recording to the pty in 80 chunks at 16 ms intervals, each followed by the bar in the next color, with chunks cut at escape-sequence and UTF-8 boundaries so the bar's own escapes never land inside a split sequence; the recording's lines may overwrite part of the bar for a frame, which the classifier ignores;
- for the key phase `nxgui_keys.py` presses `a` 40 times at 40 ms intervals through GNOME, so the key crosses GNOME Shell, GTK, nxgui, nxterm and the pty; the feeder answers each key byte with the same `INPUT nnn a` line the GPUI key handler writes, plus the bar.

`events.csv` keeps the GPUI app's meaning: a tmux update's time is taken just before its write to the pty, an input's when its key byte reaches the feeder (the GPUI app's handler sees the key at that point too), so `key→pixel` is the terminal's output path in both cases. The injection times are kept beside it in `<run>-keys.csv`; the difference is the input path, which the GPUI app does not have: in the run above a key took 12.1 ms median (23.1 ms p95) from Mutter to the feeder, 69.5 ms median (80.9 ms p95) from Mutter to the pixel. What differs from the GPUI subjects: the bytes pass through a pty and a separate process before the renderer; the viewport is nxgui's own, with its header bar, in a maximized window on the same monitor (237×61 cells here); and nxgui's final viewport is not checked against the expected text (the recording's layout is the same 120×40, the bar aside, when the window reached that size, which the feeder notes in `<run>-app.log`).

One check differs for nxgui. The GPUI app renders every update, so fewer than 70 of 80 observed tmux updates means the capture fell behind and the run is rejected. nxgui repaints at its own pace (every ~25 ms here, three vblanks of the 120 Hz display), so updates sent 16 ms apart are coalesced on its side and about 57 of 80 ever reach the screen; `summarize_gpui.py` reports that count as a result for nxgui instead of enforcing the minimum, while its other checks (every marker state in order, first and last observed, plausible latencies) still apply. Its event→pixel figures are therefore for the updates that were painted.
