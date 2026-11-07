xattr
=====

[![Build Status](https://travis-ci.org/Stebalien/xattr.svg?branch=master)](https://travis-ci.org/Stebalien/xattr)
[![Build Status](https://api.cirrus-ci.com/github/Stebalien/xattr.svg)](https://cirrus-ci.com/github/Stebalien/xattr)

A small library for setting, getting, and listing extended attributes.

Supported Platforms: Android, Linux, MacOS, FreeBSD, and NetBSD.

API Documentation: https://stebalien.github.com/xattr/xattr/

Unsupported Platforms
--------------------------

This library includes no-op support for unsupported platforms. That is, it will
build on *all* platforms but always fail on unsupported platforms.

1. You can turn this off by disabling the default `unsupported` feature. If you
   do so, this library will fail to compile on unsupported platforms.
2. Alternatively, you can detect unsupported platforms at runtime by checking
   the `xattr::SUPPORTED_PLATFORM` boolean.

---

**vt-engine-bench vendored copy.** This is the unmodified `xattr` 0.2.3
crates.io release (MIT/Apache-2.0, see `LICENSE-*`) except for
`src/sys/mod.rs`, where the Linux/Android `ENOATTR` constant now reads
`libc::ENODATA` (the same value) because libc 0.2.178+ removed the alias.
Cargo's `[patch.crates-io]` in the root manifest substitutes it so that
GPUI's `http_client` dependency builds on Linux. Nothing in the benchmark
calls it.
