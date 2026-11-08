# nx-te: nxterm's `te` engine, vendored

This is the Go package `te` of [nxterm](https://github.com/nxterm/nxterm), the
terminal emulator behind its vtapps, with the packages it imports: `cell` (the
cell model, deltas and the differ), `wire` and `nxlink` (the byte codec `cell`
uses). It was copied from nxterm commit `3b920eab` (the `v0.5.1` tag) with the
`te` and `cell` tests and `te/testdata`; the other two packages come without
their tests. The module keeps nxterm's module path `nx`, so the sources and
their imports read exactly as upstream's and a change here applies upstream
with `git apply`.

The benchmark's Go shim, `crates/nxterm-vt/go`, builds this copy (its `go.mod`
has `replace nx => ../../../engines/nx-te`) rather than the `engines/nxterm`
submodule, which is a private repository and no longer needed to build or run
the benchmark. Commits after the vendoring change `te` and `cell` for
throughput; each says what it changes and why. Run the engine's own tests from
this directory with `go test ./...`.
