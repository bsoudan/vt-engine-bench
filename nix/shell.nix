# The dev shell: the tools every repository wants, plus the project's own,
# entered inside the sandbox (nix/sandbox.nix). Everything the shell writes
# -- tools, caches, Claude's config -- stays in the checkout, under .local
# and .claude-config.
#
# The project is a Cargo workspace whose Ghostty engine is built from the
# engines/ghostty submodule by `zig build` (run from libghostty-vt-sys's
# build.rs, pointed there by .cargo/config.toml) and wrapped by a small C
# shim compiled with the `cc` crate. The scripts are plain Python 3 and the
# fixture recorder drives a real tmux client. The GPUI window-capture
# harness is macOS-only (ScreenCaptureKit, swiftc) and is not provided.
{ mkShell, bashInteractive, gnumake, jq, git, gh, openssh
, rustc, cargo, rustfmt, clippy, rust-analyzer, rustPlatform
, zig_0_16, pkg-config
, python3, tmux
, name, sandbox }:

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

    # scripts/capture_tmux.py (needs tmux) and scripts/summarize_gpui.py (stdlib only).
    python3
    tmux

    sandbox
  ];

  # Libraries the project links, found through pkg-config:
  buildInputs = [ ];

  # rust-analyzer's view of the standard library.
  RUST_SRC_PATH = "${rustPlatform.rustLibSrc}";

  # The `cc` crate compiles the Ghostty shim without -O in debug builds, and
  # glibc warns on every build that _FORTIFY_SOURCE (added by Nix's compiler
  # wrapper) needs optimization. Nothing here ships, so drop the flag.
  hardeningDisable = [ "fortify" ];

  shellHook = ''
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
