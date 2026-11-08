#[cfg(test)]
mod compat;

use std::hint::black_box;
use std::sync::Arc;
use std::time::Instant;

use alacritty_terminal::event::VoidListener;
use alacritty_terminal::term::test::TermSize;
use alacritty_terminal::term::{Config, Term};
use alacritty_terminal::vte::ansi::Processor;
use wezterm_term::{Terminal as WezTerm, TerminalConfiguration, TerminalSize};
#[derive(Debug)]
struct BenchConfig;
impl TerminalConfiguration for BenchConfig {
    fn scrollback_size(&self) -> usize {
        1000
    }
    fn color_palette(&self) -> wezterm_term::color::ColorPalette {
        Default::default()
    }
}

const COLS: usize = 120;
const ROWS: usize = 40;
const ROUNDS: usize = 7;

fn alacritty_config() -> Config {
    Config {
        scrolling_history: 1000,
        ..Config::default()
    }
}

fn workloads() -> Vec<(&'static str, Vec<u8>)> {
    let mut plain = Vec::new();
    let mut styled = Vec::new();
    let mut redraw = Vec::new();
    for i in 0..15_000 {
        plain.extend_from_slice(
            format!("{i:05} The quick brown fox jumps over the lazy dog.\r\n").as_bytes(),
        );
        styled.extend_from_slice(
            format!("\x1b[38;5;{}m{i:05} colored output\x1b[0m\r\n", i % 256).as_bytes(),
        );
    }
    for i in 0..10_000 {
        redraw.extend_from_slice(
            format!(
                "\x1b[{};1H\x1b[2K\x1b[3{}mframe {i:05} αβγ",
                i % ROWS + 1,
                i % 8
            )
            .as_bytes(),
        );
    }
    let tmux = include_bytes!("../fixtures/tmux-redraw.bin").repeat(10);
    vec![
        ("plain-scroll", plain),
        ("ansi-scroll", styled),
        ("tui-redraw", redraw),
        ("tmux-client", tmux),
    ]
}

/// Times `run` ROUNDS times, prints the median, and returns it in ms.
fn time(name: &str, workload: &str, bytes: &[u8], mut run: impl FnMut(&[u8])) -> f64 {
    let mut samples = Vec::new();
    for _ in 0..ROUNDS {
        let start = Instant::now();
        run(black_box(bytes));
        samples.push(start.elapsed().as_secs_f64() * 1000.0);
    }
    samples.sort_by(f64::total_cmp);
    let median = samples[ROUNDS / 2];
    println!(
        "{workload:14} {name:20} {median:>9.2} ms   {:>8.1} MiB/s",
        bytes.len() as f64 / median / 1048.576
    );
    median
}

/// `--csv PATH --run N` appends each median as `run,workload,engine,median_ms`
/// to PATH (with a header when the file is new), the format of results/*.csv;
/// scripts/summarize_headless.py turns three such runs into the README table.
fn csv_target() -> Option<(std::path::PathBuf, u32)> {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let value = |flag: &str| {
        args.iter()
            .position(|a| a == flag)
            .and_then(|i| args.get(i + 1))
    };
    match (value("--csv"), value("--run")) {
        (None, None) => None,
        (Some(path), Some(run)) => Some((path.into(), run.parse().expect("--run takes a number"))),
        _ => panic!("usage: vt-engine-bench [--csv PATH --run N]"),
    }
}

#[cfg(test)]
#[allow(clippy::items_after_test_module)]
mod tests {
    #[test]
    fn workloads_cover_scroll_color_and_cursor_redraw() {
        let inputs = super::workloads();
        assert_eq!(inputs.len(), 4);
        assert!(inputs[0].1.windows(2).any(|bytes| bytes == b"\r\n"));
        assert!(inputs[1].1.windows(5).any(|bytes| bytes == b"\x1b[38;"));
        assert!(inputs[2].1.windows(4).any(|bytes| bytes == b"\x1b[2K"));
        assert!(inputs[3].1.windows(5).any(|bytes| bytes == b"frame"));
    }

    #[test]
    fn all_engines_render_tmux_output() {
        use super::*;
        let bytes = workloads().pop().unwrap().1;
        let expected: Vec<String> = std::iter::once("frame 0150".to_string())
            .chain((1..=38).map(|n| n.to_string()))
            .collect();
        let normalize = |rows: Vec<String>| -> Vec<String> {
            let mut rows: Vec<_> = rows
                .into_iter()
                .map(|row| row.trim_end().to_string())
                .collect();
            while rows.last().is_some_and(String::is_empty) {
                rows.pop();
            }
            rows
        };

        let mut alacritty = Term::new(alacritty_config(), &TermSize::new(COLS, ROWS), VoidListener);
        let mut parser: Processor = Processor::new();
        for chunk in bytes.chunks(8192) {
            parser.advance(&mut alacritty, chunk);
        }
        use alacritty_terminal::index::Line;
        let alacritty_rows = normalize(
            (0..ROWS)
                .map(|row| {
                    (&alacritty.grid()[Line(row as i32)])
                        .into_iter()
                        .map(|cell| cell.c)
                        .collect()
                })
                .collect(),
        );
        assert_eq!(alacritty_rows, expected, "alacritty viewport");

        let mut ghostty = ghostty_vt::Terminal::new(COLS as u16, ROWS as u16);
        for chunk in bytes.chunks(8192) {
            ghostty.feed(chunk);
        }
        let ghostty_rows = normalize(ghostty.snapshot().0);
        assert_eq!(ghostty_rows, expected, "ghostty viewport");

        let mut vt100 = vt100::Parser::new(ROWS as u16, COLS as u16, 1000);
        for chunk in bytes.chunks(8192) {
            vt100.process(chunk);
        }
        let vt100_rows = normalize(vt100.screen().rows(0, COLS as u16).collect());
        assert_eq!(vt100_rows, expected, "vt100 viewport");

        let size = TerminalSize {
            rows: ROWS,
            cols: COLS,
            pixel_width: COLS * 8,
            pixel_height: ROWS * 16,
            dpi: 96,
        };
        let mut wezterm = WezTerm::new(
            size,
            Arc::new(BenchConfig),
            "bench",
            "0",
            Box::new(std::io::sink()),
        );
        for chunk in bytes.chunks(8192) {
            wezterm.advance_bytes(chunk);
        }
        let wezterm_rows = normalize(
            wezterm
                .screen()
                .lines_in_phys_range(0..ROWS)
                .iter()
                .map(|row| row.as_str().to_string())
                .collect(),
        );
        assert_eq!(wezterm_rows, expected, "wezterm viewport");

        let mut nxterm = nxterm_vt::Terminal::new(COLS, ROWS);
        for chunk in bytes.chunks(8192) {
            nxterm.feed(chunk);
        }
        let nxterm_rows = normalize(nxterm.snapshot().0);
        assert_eq!(nxterm_rows, expected, "nxterm viewport");
    }
}

fn main() {
    let csv = csv_target();
    let mut rows = Vec::new();
    println!(
        "Headless parse + screen-state; {} runs, median; {}x{}; no GUI or PTY",
        ROUNDS, COLS, ROWS
    );
    for (label, bytes) in workloads() {
        println!("{} bytes: {}", label, bytes.len());
        let mut record = |engine: &str, median: f64| rows.push((label, engine.to_owned(), median));
        let median = time("alacritty_terminal", label, &bytes, |bytes| {
            let mut term = Term::new(alacritty_config(), &TermSize::new(COLS, ROWS), VoidListener);
            let mut parser: Processor = Processor::new();
            for chunk in bytes.chunks(8192) {
                parser.advance(&mut term, chunk);
            }
            black_box(term);
        });
        record("alacritty_terminal", median);
        let median = time("ghostty-vt-main", label, &bytes, |bytes| {
            let mut term = ghostty_vt::Terminal::new(COLS as u16, ROWS as u16);
            for chunk in bytes.chunks(8192) {
                term.feed(chunk);
            }
            black_box(term);
        });
        record("ghostty-vt-main", median);
        let median = time("vt100", label, &bytes, |bytes| {
            let mut parser = vt100::Parser::new(ROWS as u16, COLS as u16, 1000);
            for chunk in bytes.chunks(8192) {
                parser.process(chunk);
            }
            black_box(parser);
        });
        record("vt100", median);
        let median = time("wezterm-term", label, &bytes, |bytes| {
            let size = TerminalSize {
                rows: ROWS,
                cols: COLS,
                pixel_width: COLS * 8,
                pixel_height: ROWS * 16,
                dpi: 96,
            };
            let mut term = WezTerm::new(
                size,
                Arc::new(BenchConfig),
                "bench",
                "0",
                Box::new(std::io::sink()),
            );
            for chunk in bytes.chunks(8192) {
                term.advance_bytes(chunk);
            }
            black_box(term);
        });
        record("wezterm-term", median);
        let median = time("nxterm-te", label, &bytes, |bytes| {
            let mut term = nxterm_vt::Terminal::new(COLS, ROWS);
            for chunk in bytes.chunks(8192) {
                term.feed(chunk);
            }
            black_box(term);
        });
        record("nxterm-te", median);
    }
    if let Some((path, run)) = csv {
        use std::io::Write;
        let fresh = std::fs::metadata(&path)
            .map(|m| m.len() == 0)
            .unwrap_or(true);
        let mut file = std::fs::OpenOptions::new()
            .append(true)
            .create(true)
            .open(&path)
            .expect("open --csv file");
        if fresh {
            writeln!(file, "run,workload,engine,median_ms").unwrap();
        }
        for (workload, engine, median) in rows {
            writeln!(file, "{run},{workload},{engine},{median:.2}").unwrap();
        }
        println!("appended run {run} to {}", path.display());
    }
}
