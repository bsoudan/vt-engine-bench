#!/bin/bash
# macOS: run from a logged-in desktop session with Screen Recording permission
#        (ScreenCaptureKit captures the GPUI window).
# Linux: run from any shell on a machine with a live GNOME Wayland session; the
#        GPUI window opens in that session and scripts/capture_gpui_linux.py
#        observes it through Mutter's screencast API (see README). The fifth
#        subject there is nxgui, the GNOME console of engines/nxterm: a real
#        terminal window fed through an nxterm vtapp's pty (scripts/nxgui_feed.py)
#        with keys injected by GNOME (scripts/nxgui_keys.py).
set -euo pipefail
cd "$(dirname "$0")/.."
root="$PWD"
if [[ -d "$root/.context/zig" ]]; then PATH="$root/.context/zig:$PATH"; fi # bootstrap-zig.sh (macOS)
cargo build --release --features gpui-bench --bin gpui_bench
app_pid=
capture_pid=
extra_pids=()
cleanup() {
  if [[ -n "${capture_bin:-}" ]]; then rm -f "$capture_bin"; fi
  for pid in "$app_pid" "$capture_pid" "${inhibit_pid:-}" "${extra_pids[@]}"; do
    if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; fi
  done
}
trap cleanup EXIT
os="$(uname -s)"
engines=(alacritty ghostty vt100 wezterm)
case "$os" in
  Darwin)
    capture_bin="$(mktemp "${TMPDIR:-/tmp}/vt-capture-gpui.XXXXXX")"
    swiftc -parse-as-library scripts/capture_gpui.swift -o "$capture_bin"
    ;;
  Linux)
    runtime="/run/user/$(id -u)"
    # Started over ssh or inside a sandbox, the shell has none of the desktop
    # session's variables: name its Wayland, D-Bus and PipeWire sockets by path.
    if [[ -z "${WAYLAND_DISPLAY:-}" ]]; then
      for socket in "$runtime"/wayland-*; do
        if [[ -S "$socket" ]]; then export WAYLAND_DISPLAY="$socket"; break; fi
      done
    fi
    if [[ -z "${WAYLAND_DISPLAY:-}" ]]; then
      echo "No Wayland socket in $runtime; is a GNOME Wayland session running? (or set WAYLAND_DISPLAY)" >&2
      exit 1
    fi
    export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=$runtime/bus}"
    export PIPEWIRE_RUNTIME_DIR="${PIPEWIRE_RUNTIME_DIR:-$runtime}"
    # Nothing here counts as user input, so GNOME would idle-lock the session
    # mid-benchmark and hide the window: hold an idle inhibitor until done.
    python3 - <<'PY' &
import signal
from gi.repository import Gio, GLib
bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
bus.call_sync("org.gnome.SessionManager", "/org/gnome/SessionManager", "org.gnome.SessionManager", "Inhibit",
              GLib.Variant("(susu)", ("vt-engine-bench", 0, "GPUI window-capture benchmark", 8)),
              None, Gio.DBusCallFlags.NONE, 5000, None)
signal.pause()
PY
    inhibit_pid=$!
    # nxgui and nxterm from the pinned submodule; the gtk tag links GTK 4.
    nx_bin="$root/target/nx/bin"
    mkdir -p "$nx_bin"
    (cd engines/nxterm && go build -tags gtk -o "$nx_bin/nxgui" ./cmd/nxgui && go build -o "$nx_bin/nxterm" ./cmd/nxterm)
    engines+=(nxgui)
    ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac
output="$(mktemp -d "$root/results/gpui-$(date +%F)-XXXXXX")"
printf 'Raw GPUI results: %s\n' "$output"
# A run whose capture did not keep up is invalid (summarize_gpui.py says so)
# and is repeated up to this many times; every attempt's files are kept.
attempts="${GPUI_BENCH_ATTEMPTS:-3}"
fail() { cat "$1"; echo "$2" >&2; return 1; }
run_once() {
  local engine="$1" prefix="$2"
  if [[ "$os" == Darwin ]]; then
    "$root/target/release/gpui_bench" "$engine" "$prefix-events.csv" "$prefix-ready" >"$prefix-app.log" 2>&1 &
    app_pid=$!
    "$capture_bin" "$app_pid" "$prefix-events.csv" "$prefix-captures.csv" "$prefix-ready" >"$prefix-capture.log" 2>&1 \
      || { fail "$prefix-capture.log" "Capture failed"; return 1; }
  else
    # The collector goes first: it may have to create the monitor the window
    # opens on, and it signals when frames are flowing.
    python3 scripts/capture_gpui_linux.py "$prefix-events.csv" "$prefix-captures.csv" "$prefix-ready" "$prefix-streaming" \
      >"$prefix-capture.log" 2>&1 &
    capture_pid=$!
    while [[ ! -e "$prefix-streaming" ]]; do
      if ! kill -0 "$capture_pid" 2>/dev/null; then
        wait "$capture_pid" || { capture_pid=; fail "$prefix-capture.log" "Capture failed"; return 1; }
      fi
      sleep 0.1
    done
    "$root/target/release/gpui_bench" "$engine" "$prefix-events.csv" "$prefix-ready" >"$prefix-app.log" 2>&1 &
    app_pid=$!
    if ! wait "$capture_pid"; then
      capture_pid=; kill "$app_pid" 2>/dev/null || true; wait "$app_pid" 2>/dev/null || true; app_pid=
      fail "$prefix-capture.log" "Capture failed"; return 1
    fi
    capture_pid=
  fi
  if ! wait "$app_pid"; then app_pid=; fail "$prefix-app.log" "GPUI app failed"; return 1; fi
  app_pid=
  rm -f "$prefix-ready" "$prefix-streaming"
  cat "$prefix-app.log" "$prefix-capture.log"
  python3 scripts/summarize_gpui.py "$prefix-events.csv" "$prefix-captures.csv"
}
# nxgui: the collector first, then an nxterm vtapp running the feeder in a
# private nx runtime dir (its own app registry, away from any nx session of
# the user's), then nxgui showing it, maximized by GNOME's shortcut; the
# feeder paints the marker once the window is big enough, and the key
# injector takes over after the tmux phase.
run_nxgui() {
  local prefix="$1" nx_runtime="$output/nx-runtime"
  rm -rf "$nx_runtime"; mkdir -p -m 0700 "$nx_runtime"
  rm -f "$prefix-keys-ready"
  python3 scripts/capture_gpui_linux.py "$prefix-events.csv" "$prefix-captures.csv" "$prefix-ready" "$prefix-streaming" \
    >"$prefix-capture.log" 2>&1 &
  capture_pid=$!
  while [[ ! -e "$prefix-streaming" ]]; do
    if ! kill -0 "$capture_pid" 2>/dev/null; then
      wait "$capture_pid" || { capture_pid=; fail "$prefix-capture.log" "Capture failed"; return 1; }
    fi
    sleep 0.1
  done
  XDG_RUNTIME_DIR="$nx_runtime" "$nx_bin/nxterm" --session bench -- \
    python3 "$root/scripts/nxgui_feed.py" "$prefix-events.csv" "$prefix-ready" "$prefix-keys-ready" "$prefix-app.log" \
    >>"$prefix-app.log" 2>&1 &
  extra_pids+=($!)
  XDG_RUNTIME_DIR="$nx_runtime" "$nx_bin/nxgui" --font "DejaVu Sans Mono 10" >"$prefix-nxgui.log" 2>&1 &
  local nxgui_pid=$!
  extra_pids+=("$nxgui_pid")
  sleep 2 # the window maps; the shortcut then goes to it as the newest, focused window
  python3 scripts/nxgui_keys.py maximize
  python3 scripts/nxgui_keys.py type "$prefix-keys-ready" "$prefix-keys.csv" >"$prefix-keys.log" 2>&1 &
  local keys_pid=$!
  extra_pids+=("$keys_pid")
  local status=0
  wait "$capture_pid" || status=1
  capture_pid=
  wait "$keys_pid" || true
  kill "$nxgui_pid" 2>/dev/null || true
  wait "$nxgui_pid" 2>/dev/null || true
  rm -f "$prefix-ready" "$prefix-streaming" "$prefix-keys-ready"
  if (( status )); then fail "$prefix-capture.log" "Capture failed"; return 1; fi
  cat "$prefix-app.log" "$prefix-capture.log"
  python3 scripts/summarize_gpui.py "$prefix-events.csv" "$prefix-captures.csv"
}
if (( $# )); then
  engines=("$1")
  if [[ "$1" == nxgui && "$os" != Linux ]]; then echo "nxgui is measured on Linux only" >&2; exit 1; fi
fi
for engine in "${engines[@]}"; do
  for run in 1 2 3; do
    prefix="$output/$engine-$run"
    for (( attempt = 1; attempt <= attempts; attempt++ )); do
      printf '\n%s run %s/3 (attempt %s/%s)\n' "$engine" "$run" "$attempt" "$attempts"
      if [[ "$engine" == nxgui ]]; then
        if run_nxgui "$prefix"; then break; fi
      elif run_once "$engine" "$prefix"; then break; fi
      rm -f "$prefix-ready" "$prefix-streaming" "$prefix-keys-ready"
      for file in events.csv captures.csv app.log capture.log nxgui.log keys.csv keys.log; do
        if [[ -e "$prefix-$file" ]]; then mv "$prefix-$file" "$prefix-attempt$attempt-$file"; fi
      done
      if (( attempt == attempts )); then
        echo "Invalid run after $attempts attempts; no comparison published. Diagnostics: $output" >&2; exit 1
      fi
    done
  done
done
if (( $# == 0 )); then python3 scripts/summarize_gpui.py --aggregate "$output" > "$output/summary.csv"; fi
printf '\nValidated raw data: %s\n' "$output"
