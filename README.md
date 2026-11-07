# VT engine benchmark (macOS and Linux)

Two separate measurements: **headless parsing + screen-state** and **a shared GPUI viewport observed through the OS screen-capture path** (ScreenCaptureKit on macOS, Mutter's screencast API on Linux/GNOME). Sources are pinned as git submodules (see `git submodule status` and `Cargo.lock`). Ghostty is pinned to current `main` at [`b699ea7`](https://github.com/ghostty-org/ghostty/commit/b699ea79f4b881421b4b3055abc16a0957d76beb) (1.3.2-dev). Its benchmark-only C shim is compiled against that checkout's headers and links the matching `libghostty-vt` archive; `libghostty-rs` supplies the Zig build integration, **not** its older Rust API. The other engines are pinned source checkouts. Parser-only `vte` is excluded because it has no screen state.

```sh
git submodule update --init --recursive
./bootstrap-zig.sh # Zig 0.16.0 (macOS)
PATH="$PWD/.context/zig:$PATH" cargo test
PATH="$PWD/.context/zig:$PATH" cargo run --release
```

On Linux, `./enter` opens the Nix dev shell (`flake.nix`, `nix/shell.nix`) with Rust, Zig 0.16, the libraries GPUI's Wayland backend links or dlopens (xkbcommon, libxcb, FreeType, Fontconfig, Wayland, the Vulkan loader), and the PyGObject + GStreamer + PipeWire stack the Linux window-capture collector uses; inside it, run `cargo test`, `cargo run --release` and `bash scripts/run_gpui_bench.sh` as they are, without the `PATH` prefix. The shell runs in a read-only sandbox of the system (`nix/sandbox.nix`); the scripts still reach the desktop session because they name its sockets by path. GPUI's dependency tree needs one Linux fix: `crates/xattr` is the `xattr` 0.2.3 release with its removed-from-libc `ENOATTR` alias replaced, substituted through `[patch.crates-io]`.

Three workloads are generated in `src/main.rs`: 15k plain scroll lines, 15k 256-color lines, and 10k cursor-positioned/cleared TUI updates. A fourth is a **real tmux client PTY recording** (`fixtures/tmux-redraw.bin`, 25,641 bytes, SHA-256 `d30383bb7658ae386f6f8e70e7d26bc8f1484b95d1199cbaf81715ad7233b7da`) captured by `python3 scripts/capture_tmux.py`, replayed ten times for timing. Re-capturing changes the fixture; the committed bytes define this run. The capture uses `/bin/sh`, a temporary socket and config, and disables tmux's status line to avoid machine-identifying data.

120×40 cells, 1k scrollback in each engine where configurable, chunks of 8192 bytes. Seven runs per workload/engine, median wall time includes creating a fresh model and dropping it; throughput uses input bytes. No GUI, live PTY during timing, glyph rendering, font shaping, or latency. Run when machine is otherwise idle. Higher throughput here **does not mean smoother or faster terminal UI**. Different protocol feature sets and implementation paths mean this is exploratory, not a compliance-normalized comparison. The Ghostty C shim intentionally exposes only APIs the benchmark tests; do not treat it as a general-purpose Rust binding. `cargo test` checks **all four engines reproduce the exact expected visible text** (the `frame 0150` header and 38 numbered lines) after the same 8192-byte chunking and ten replays used in timing. Additional small-screen fixtures in `src/compat.rs` check the behaviors below with 7-byte chunking (splitting ANSI escapes and UTF-8). These checks do not establish broad VT compliance. The previous recording included a shell prompt after the last frame; it caused a scroll and divergent end state. The capture script now keeps the command running while recording the final frame, removing that ambiguous post-command transition. Replaying ten times also repeats initialization sequences; it is not a continuous tmux session.

## Core compatibility checks

| Check | Alacritty | Ghostty main | vt100 | WezTerm |
|---|:---:|:---:|:---:|:---:|
| Repeated tmux client: exact visible text | ✓ | ✓ | ✓ | ✓ |
| Cursor addressing and line clear | ✓ | ✓ | ✓ | ✓ |
| Truecolor foreground/background and bold | ✓ | ✓ | ✓ | ✓ |
| Indexed color and alternate-screen restore | ✓ | ✓ | ✓ | ✓ |
| Wide Unicode and combining marks | ✓ | ✓ | ✓ | ✓ |
| Preserve text on resize | ✓ | ✓ | ✓ | ✓ |
| Retain and inspect scrollback | ✓ | ✓ | ✓ | ✓ |

Only the **specific fixtures** pass; this is not a complete VT conformance suite. In particular, mouse input, selection, OSC clipboard, hyperlinks, grapheme clustering, error handling, exact cursor shape, rendering, and latency remain untested. The Ghostty column tests the pinned current-main C API through a narrow benchmark shim, not the full macOS app.

## Recorded run (Apple M1 Pro, macOS 15.4.1, release build, 2026-10-06)

| Workload | alacritty_terminal | Ghostty main | vt100 | wezterm-term |
|---|---:|---:|---:|---:|
| plain scroll, ms | 5.98 | **1.34** | 7.93 | 26.24 |
| ANSI scroll, ms | **4.67** | 6.18 | 6.06 | 14.64 |
| TUI redraw, ms | 3.56 | **2.66** | 3.51 | 25.17 |
| tmux client (repeated recording), ms | **2.14** | 6.02 | 8.54 | 138.26 |

[Raw per-run medians](results/2026-10-06-m1-pro-current-ghostty.csv) are included. Values are the **median of three independent process runs**, each reporting the median of seven iterations. This is not a controlled system-wide benchmark. [Previous runs](results/2026-10-06-m1-pro.csv) used Ghostty 1.2.3 and Alacritty's 10k-line default scrollback; **do not compare them directly** to these 1k-line results.

**Do not select a GPUI terminal core on these numbers alone.** All four pass the bounded core fixtures; none is proven the fastest rendered terminal. Ghostty leads plain output and synthetic TUI redraws; Alacritty leads this recorded tmux stream and ANSI output.

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

Each figure is the **median of three independent runs**; ms is lower-is-better. One vt100 run was repeated once (65 of 80 updates captured on the first attempt).

| GPUI metric | Alacritty | Ghostty main | vt100 | WezTerm |
|---|---:|---:|---:|---:|
| Tmux observed updates / 80 | 76 | 79 | 79 | 75 |
| Tmux event→pixel median, ms | 17.98 | 17.48 | 19.22 | 16.99 |
| Tmux event→pixel p95, ms | 30.88 | 28.32 | 26.84 | 25.58 |
| Tmux changed-frame interval median, ms | 16.67 | 16.66 | 16.67 | 16.69 |
| Tmux changed-frame interval p95, ms | 27.87 | 26.28 | 26.62 | 25.37 |
| Synthetic key→pixel median, ms | 10.58 | 9.94 | 13.21 | 10.40 |
| Synthetic key→pixel p95, ms | 14.63 | 13.28 | 17.22 | 14.14 |

[Per-run summary](results/2026-11-07-gpui-alpha-gtx860m.csv) · [Raw event timestamps, captured frames, and logs](results/2026-11-07-gpui-alpha-gtx860m/). The four engines sit within a few ms of each other on every metric, as on macOS; the tmux p95 spread comes from the 120 Hz compositor occasionally coalescing two updates into one paint. The machine had nothing else running on its desktop.
