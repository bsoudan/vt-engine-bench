//! Builds go/ as a C archive of nxterm's te engine (engines/nxterm) with the
//! Go toolchain on PATH, and links it. The Nix shell provides Go on Linux;
//! on macOS, `brew install go`.
use std::env;
use std::path::PathBuf;
use std::process::Command;

fn main() {
    let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let out = PathBuf::from(env::var("OUT_DIR").unwrap());
    let archive = out.join("libnxvt.a");
    let status = Command::new("go")
        .args(["build", "-buildmode=c-archive", "-o"])
        .arg(&archive)
        .arg(".")
        .current_dir(manifest.join("go"))
        .env("CGO_ENABLED", "1")
        .status()
        .unwrap_or_else(|err| panic!("running `go build` for nxterm's te engine (is Go on PATH?): {err}"));
    assert!(status.success(), "`go build -buildmode=c-archive` for nxterm's te engine failed");
    println!("cargo:rustc-link-search=native={}", out.display());
    println!("cargo:rustc-link-lib=static=nxvt");
    match env::var("CARGO_CFG_TARGET_OS").as_deref() {
        Ok("macos") => {
            println!("cargo:rustc-link-lib=framework=CoreFoundation");
            println!("cargo:rustc-link-lib=framework=Security");
        }
        _ => println!("cargo:rustc-link-lib=pthread"),
    }
    println!("cargo:rerun-if-changed=go");
    println!("cargo:rerun-if-changed=../../engines/nxterm/te");
    println!("cargo:rerun-if-changed=../../engines/nxterm/cell");
}
