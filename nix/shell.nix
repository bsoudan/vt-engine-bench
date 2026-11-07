# The dev shell: the tools every repository wants, plus the project's own,
# entered inside the sandbox (nix/sandbox.nix). Everything the shell writes
# -- tools, caches, Claude's config -- stays in the checkout, under .local
# and .claude-config.
#
# The project is a Cargo workspace whose Ghostty engine is built from the
# engines/ghostty submodule by `zig build` (run from libghostty-vt-sys's
# build.rs, pointed there by .cargo/config.toml) and wrapped by a small C
# shim compiled with the `cc` crate. The scripts are plain Python 3 and the
# fixture recorder drives a real tmux client.
#
# The GPUI window-capture harness (scripts/run_gpui_bench.sh) has two
# collectors: ScreenCaptureKit on macOS (not provided here) and, on Linux,
# scripts/capture_gpui_linux.py, which records the GNOME session through
# Mutter's org.gnome.Mutter.ScreenCast D-Bus API and reads the PipeWire
# stream with GStreamer. The GPUI app itself needs Wayland, xkbcommon,
# Vulkan, FreeType and Fontconfig on Linux.
{ lib, stdenv, mkShell, bashInteractive, gnumake, jq, git, gh, openssh
, rustc, cargo, rustfmt, clippy, rust-analyzer, rustPlatform
, zig_0_16, pkg-config
, python3, tmux
, libxkbcommon, libxcb, wayland, vulkan-loader, freetype, fontconfig
, gobject-introspection, gst_all_1, pipewire
, name, sandbox }:

let
  # GPUI on Linux dlopens these at run time (libwayland-client, libvulkan,
  # libfontconfig); they are not on any default search path on NixOS.
  gpuiRuntimeLibs = [ wayland vulkan-loader fontconfig freetype libxkbcommon ];
  # The Linux collector: PyGObject for D-Bus (Gio) and GStreamer, whose
  # pipewiresrc element ships with PipeWire itself.
  capturePython = python3.withPackages (ps: [ ps.pygobject3 ]);
  # `out`, not gstreamer's default `bin` output, holds the core elements
  # (capsfilter) and the Gst typelib.
  gstPackages = map (lib.getOutput "out") [ gst_all_1.gstreamer gst_all_1.gst-plugins-base pipewire ];
  giPackages = [ gobject-introspection ] ++ map (lib.getOutput "out") [ gst_all_1.gstreamer gst_all_1.gst-plugins-base ];
in
mkShell {
  name = "${name}-dev";

  packages = [
    bashInteractive # not pkgs.bash: that is the build shell, without readline or completion
    gnumake
    jq

    # Version control
    git
    gh
    openssh

    # Rust: the benchmark, its tests, and the engine crates (Cargo.lock pins them).
    rustc
    cargo
    rustfmt
    clippy
    rust-analyzer

    # Zig 0.16: libghostty-vt-sys shells out to `zig build` in engines/ghostty.
    # This replaces bootstrap-zig.sh, which only knows how to download macOS tarballs.
    zig_0_16
    pkg-config

    # scripts/capture_tmux.py (needs tmux), scripts/summarize_gpui.py (stdlib
    # only) and, on Linux, scripts/capture_gpui_linux.py (PyGObject + GStreamer).
    (if stdenv.isLinux then capturePython else python3)
    tmux

    sandbox
  ] ++ lib.optionals stdenv.isLinux gstPackages;

  # Libraries the project links, found through pkg-config or -L: GPUI's
  # Linux backend links xkbcommon (+ its X11 half), libxcb and FreeType.
  buildInputs = lib.optionals stdenv.isLinux [ libxkbcommon libxcb freetype fontconfig ];

  # rust-analyzer's view of the standard library.
  RUST_SRC_PATH = "${rustPlatform.rustLibSrc}";

  # The `cc` crate compiles the Ghostty shim without -O in debug builds, and
  # glibc warns on every build that _FORTIFY_SOURCE (added by Nix's compiler
  # wrapper) needs optimization. Nothing here ships, so drop the flag.
  hardeningDisable = [ "fortify" ];

  shellHook = ''
    ${lib.optionalString stdenv.isLinux ''
      # GPUI dlopens Wayland, Vulkan and Fontconfig by soname.
      export LD_LIBRARY_PATH="${lib.makeLibraryPath gpuiRuntimeLibs}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
      # GStreamer plugins (pipewiresrc, videoconvert, appsink) and the GI
      # typelibs (Gio, Gst, GstApp, GstVideo) the Linux collector imports.
      export GST_PLUGIN_SYSTEM_PATH_1_0="${lib.makeSearchPath "lib/gstreamer-1.0" gstPackages}"
      export GI_TYPELIB_PATH="${lib.makeSearchPath "lib/girepository-1.0" giPackages}"
      # Mutter's screencast frames arrive over the session's PipeWire
      # socket, which lives in the real runtime dir ($XDG_RUNTIME_DIR moves
      # below), as does the session D-Bus socket DBUS_SESSION_BUS_ADDRESS
      # already names by path.
      export PIPEWIRE_RUNTIME_DIR="''${PIPEWIRE_RUNTIME_DIR:-/run/user/$(id -u)}"
    ''}
    # stdenv exports its build bash as $SHELL -- made without readline
    # or completion, so a bash prompt shows its \[ \] markers as text --
    # and the launching shell's value is gone by now. The sandbox's
    # shell should be the user's own: the login shell the password
    # database records, which is what the launching terminal's $SHELL was.
    export SHELL="$(getent passwd "$(id -un)" | cut -d: -f7)"
    [ -x "$SHELL" ] || export SHELL="$(command -v bash)"
    export CLAUDE_CONFIG_DIR=$PWD/.claude-config
    export PATH="$PWD/.local/bin:$PATH"
    # Language caches, kept in the checkout. The sandbox mounts the system
    # read-only, so cargo's default ~/.cargo registry would be unwritable.
    export CARGO_HOME="$PWD/.local/cargo"
    export PATH="$CARGO_HOME/bin:$PATH"
    # Zig's package cache: build.rs gives `zig build` a per-OUT_DIR
    # --cache-dir but leaves the global (package) cache at its default.
    export ZIG_GLOBAL_CACHE_DIR="$PWD/.local/var/cache/zig"
    mkdir -p "$CARGO_HOME" "$ZIG_GLOBAL_CACHE_DIR"
    export NIX_SHELL=${name}-dev
    # A GUI started here finds the desktop's Wayland socket by name in
    # $XDG_RUNTIME_DIR, which moves below: name it by its path instead --
    # where the socket is, which is not $XDG_RUNTIME_DIR in a shell entered
    # from one that already moved it.
    if [ -n "''${WAYLAND_DISPLAY-}" ] && [ "''${WAYLAND_DISPLAY#/}" = "$WAYLAND_DISPLAY" ]; then
      for d in "$XDG_RUNTIME_DIR" "/run/user/$(id -u)"; do
        if [ -S "$d/$WAYLAND_DISPLAY" ]; then
          export WAYLAND_DISPLAY="$d/$WAYLAND_DISPLAY"
          break
        fi
      done
    fi
    # Moved once: a shell entered from this one keeps it where it is.
    if [ "''${XDG_RUNTIME_DIR#$PWD/.local/}" = "$XDG_RUNTIME_DIR" ]; then
      export XDG_RUNTIME_DIR="$PWD/.local''${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
    fi
    mkdir -p -m 0700 "$XDG_RUNTIME_DIR"

    echo
    echo "entering $NIX_SHELL dev environment"
    # Only the interactive shell is sandboxed: nix develop -c and
    # direnv's print-dev-env run this hook non-interactively, and
    # must get their command, or their environment, back.
    if [[ $- == *i* && -z ''${IN_SANDBOXED_SHELL-} ]]; then
      exec ${name}-sandbox
    fi
  '';
}
