//! Narrow, benchmark-only bindings to nxterm's te engine, built from go/ by build.rs.
use std::ffi::{c_char, c_int, CStr};

/// A cell's color as te stores it: `mode` is nx/cell.ColorMode (0 default,
/// 1 ANSI 16, 2 ANSI 256, 3 true color), `index` the palette entry for the
/// first two, `r` `g` `b` the true color; `valid` is 0 for the default color.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct Color {
    pub r: u8,
    pub g: u8,
    pub b: u8,
    pub valid: u8,
    pub bold: u8,
    pub mode: u8,
    pub index: u8,
}

unsafe extern "C" {
    fn nxvt_new(cols: c_int, rows: c_int) -> usize;
    fn nxvt_free(handle: usize);
    fn nxvt_feed(handle: usize, data: *const c_char, len: usize);
    fn nxvt_resize(handle: usize, cols: c_int, rows: c_int);
    fn nxvt_cursor(handle: usize, col: *mut c_int, row: *mut c_int);
    fn nxvt_snapshot(handle: usize, fg: *mut Color, bg: *mut Color) -> *mut c_char;
    fn nxvt_history(handle: usize) -> *mut c_char;
    fn nxvt_free_string(text: *mut c_char);
}

fn take(text: *mut c_char) -> String {
    let owned = unsafe { CStr::from_ptr(text) }.to_str().expect("te returned invalid UTF-8").to_owned();
    unsafe { nxvt_free_string(text) };
    owned
}

pub struct Terminal {
    handle: usize,
}

impl Terminal {
    pub fn new(cols: usize, rows: usize) -> Self {
        Self { handle: unsafe { nxvt_new(cols as c_int, rows as c_int) } }
    }
    pub fn feed(&mut self, bytes: &[u8]) {
        unsafe { nxvt_feed(self.handle, bytes.as_ptr().cast(), bytes.len()) }
    }
    pub fn resize(&mut self, cols: usize, rows: usize) {
        unsafe { nxvt_resize(self.handle, cols as c_int, rows as c_int) }
    }
    /// (column, row), zero-based.
    pub fn cursor(&self) -> (usize, usize) {
        let (mut col, mut row) = (0, 0);
        unsafe { nxvt_cursor(self.handle, &mut col, &mut row) };
        (col as usize, row as usize)
    }
    /// The visible rows (trailing blanks trimmed) and the top-left cell's foreground and background.
    pub fn snapshot(&self) -> (Vec<String>, Color, Color) {
        let (mut fg, mut bg) = (Color::default(), Color::default());
        let text = take(unsafe { nxvt_snapshot(self.handle, &mut fg, &mut bg) });
        (text.split('\n').map(|row| row.trim_end().to_owned()).collect(), fg, bg)
    }
    /// The scrollback rows, oldest first.
    pub fn history(&self) -> Vec<String> {
        let text = take(unsafe { nxvt_history(self.handle) });
        if text.is_empty() {
            return Vec::new();
        }
        text.split('\n').map(|row| row.trim_end().to_owned()).collect()
    }
}

impl Drop for Terminal {
    fn drop(&mut self) {
        unsafe { nxvt_free(self.handle) }
    }
}
