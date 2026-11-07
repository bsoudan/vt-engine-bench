{
  description = "vt-engine-bench: headless VT engine benchmark (Alacritty, Ghostty, vt100, WezTerm)";

  inputs = {
    # ./enter rewrites this line to follow the system's nixpkgs: keep it one line.
    nixpkgs.url = "github:NixOS/nixpkgs/a033e66ab94a6bedf2c3d58d93a36319d3cbbbaf";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        # The project's name: the dev shell's, and its sandbox's (<name>-sandbox).
        name = "vt-engine-bench";
        pkgs = nixpkgs.legacyPackages.${system};
        sandbox = pkgs.callPackage ./nix/sandbox.nix { inherit name; };
      in
      {
        packages.sandbox = sandbox;

        # Everything in the repo builds and runs here:
        #   cargo test                            # core compatibility fixtures
        #   cargo run --release                   # headless benchmark
        #   bash scripts/run_gpui_bench.sh        # GPUI window capture (needs an
        #                                         # unlocked GNOME Wayland session)
        #   python3 scripts/capture_tmux.py       # re-record the tmux fixture
        #   python3 scripts/summarize_gpui.py ... # summarize recorded GPUI runs
        devShells.default = pkgs.callPackage ./nix/shell.nix { inherit name sandbox; };
      }
    );
}
