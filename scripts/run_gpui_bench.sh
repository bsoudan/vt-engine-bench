#!/bin/bash
# macOS: run from a logged-in desktop session with Screen Recording permission
#        (ScreenCaptureKit captures the GPUI window).
# Linux: run from any shell on a machine with a live GNOME Wayland session; the
#        GPUI window opens fullscreen in that session and scripts/capture_gpui_linux.py
#        observes it through Mutter's screencast API (see README).
set -euo pipefail
cd "$(dirname "$0")/.."
root="$PWD"
if [[ -d "$root/.context/zig" ]]; then PATH="$root/.context/zig:$PATH"; fi # bootstrap-zig.sh (macOS)
cargo build --release --features gpui-bench --bin gpui_bench
app_pid=
capture_pid=
cleanup() {
  if [[ -n "${capture_bin:-}" ]]; then rm -f "$capture_bin"; fi
  for pid in "$app_pid" "$capture_pid" "${inhibit_pid:-}"; do
    if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; fi
  done
}
trap cleanup EXIT
os="$(uname -s)"
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
engines=(alacritty ghostty vt100 wezterm)
if (( $# )); then engines=("$1"); fi
for engine in "${engines[@]}"; do
  for run in 1 2 3; do
    prefix="$output/$engine-$run"
    for (( attempt = 1; attempt <= attempts; attempt++ )); do
      printf '\n%s run %s/3 (attempt %s/%s)\n' "$engine" "$run" "$attempt" "$attempts"
      if run_once "$engine" "$prefix"; then break; fi
      rm -f "$prefix-ready" "$prefix-streaming"
      for file in events.csv captures.csv app.log capture.log; do
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
