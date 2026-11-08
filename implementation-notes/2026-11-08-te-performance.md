# Making te fast: what Ghostty's architecture buys, applied to nxterm's engine

Written 2026-11-08 from the `worktree-te-vendored` branch of vt-engine-bench, where
nxterm's `te` package (v0.5.1, commit `3b920eab`) was vendored as `engines/nx-te`
and tuned in 24 commits, each with te's 766 tests, a race run and the bench's
`cargo test` green. This note is the record to redo the work in nxterm itself:
what to change, in what order, what each change is worth, how to prove it, and
what pulls against `doc/VALUES.md`. Numbers are on alpha (Intel i3-4130T,
NixOS), fed from Go without cgo, medians of seven; the bench's own figures
through the C shim are in the README.

## Where it landed

| Workload | te v0.5.1 | te after | Ghostty main | Alacritty |
|---|---:|---:|---:|---:|
| plain scroll, ms | 453 | 5.3 | 3.2 | 13.0 |
| 256-color scroll, ms | 260 | 10.4 | 20.0 | 9.3 |
| TUI redraw, ms | 108 | 9.8 | 4.2 | 6.1 |
| tmux recording, ms | 243 | 7.0 | 7.4 | 3.6 |

With deltas attached, a cut after every 8 KB chunk applied to a shadow screen
(feed · cut · apply, ms for the whole workload):

| Workload (cuts) | te v0.5.1 | te after |
|---|---:|---:|
| plain scroll (96) | 496 · 17 · 80 | 11 · 11 · 5 |
| 256-color scroll (67) | 287 · 10 · 74 | 16 · 5.6 · 5.4 |
| TUI redraw (42) | 111 · 4.6 · 0.3 | 10 · 1.5 · 0.3 |
| tmux (32) | 248 · 2.8 · 0.1 | 7.2 · 0.6 · 0.06 |

## How the work was done, and should be again

- **Measure before guessing.** The first claim made in this session, that a
  batched ASCII print path was "most of the hundredfold gap", was wrong: it was
  the largest single term on the scrolls and irrelevant on the tmux recording,
  whose cost was a map lookup per erased cell. Every change after that began
  with a CPU profile of the specific workload and ended with a measurement of
  all four. The harness is thirty lines of Go: feed the workload in 8 KB
  chunks, seven runs, report the median, write a pprof profile of forty more.
  In the Nix shell set `GOPATH`, `GOMODCACHE` and `GOCACHE` to a writable
  directory first; `~/go` is read-only there.
- **One commit per change, with its number in the message.** Each commit says
  what it changes, why the profile pointed there, and what it did to the
  workloads. That is what made it possible to say afterwards which changes
  carried the weight and which were noise.
- **A fast path needs a slow path it must match, and a test that checks.**
  `TestDrawASCIIMatchesGeneral` draws random mixes of ASCII, combining marks,
  wide glyphs, emoji, flags, Hangul, a Devanagari conjunct and a prepend
  through the fast path and through the general path alone (`asciiFastPath`
  off) and wants the same cells, cursor, history and last-drawn text. It was
  checked to fail on a planted bug before it was trusted. The general path is
  the definition; a fast path is an optimization of it, never a second
  definition. This is also how Ghostty tests `printSliceFast` against `print`.
- **Keep the existing suites as the gate.** The esctest2 and pyte corpora and
  the delta corpus test (every screen made in tests is rebuilt from its
  deltas and compared) caught the two real bugs of the session: a grapheme
  table whose index 0 collided with "no text", and tests that wrote into a
  copy after `Buffer` became `Rows()`.

## The changes, in the order to make them

Each has the workload it helps, the profile evidence, and anything that pulls
against a value. The first group is local to te and safe to take as it is; the
second changes te's storage and has companions it cannot ship without.

### Group one: the engine as it is

1. **`cell.WidthPolicy.Width` answers printable ASCII without uniseg.** One
   column, never a cluster. A quarter of the plain scroll's time and two thirds
   of its allocations were `Pieces` slices and property-table walks for ASCII.
2. **An ASCII fast path in `Draw`.** Runs of printable ASCII, when the active
   charset is the identity and insert mode is off, are written a byte a cell
   from a static table of one-byte strings, a row at a time: work out how many
   fit from the cursor, write them in one pass, move the cursor once. A run
   ends one byte early before a non-ASCII byte so a combining mark still joins
   its base. Everything else goes through the per-cluster general path. Plain
   scroll 351 to 127 ms on its own.
3. **Recycle rows on a scroll.** `Index` gave the departing row to the history
   without a copy (the screen replaces its pointer and never writes it again;
   `Resize` reslices past the rows it hands over) and refilled the row the
   history evicted as the new bottom row. 127 to 39 ms; allocation 236 MiB to
   10 MiB per run. The `Differ` already recycled its model's rows this way.
   Caveat while cells are still `cell.Cell`: a row from `ScrollbackRows()` is
   overwritten once evicted, so a caller holds none past the next scroll; the
   view cores read within one cut and are fine.
4. **Compute the erased cell once per erase.** `bceAttr` → `defaultAttr` →
   `isModeSet` was a map lookup per cell, 4800 per full-screen erase, 85% of
   the tmux recording once drawing was cheap. Hoist the blank out of the loop.
5. **Read CSI parameters into a reused byte buffer**, not a string grown a
   digit at a time, and keep a parameter's string only when it has a colon;
   note the colon as it is read so `expandSGRParams` returns at once without
   one. Five of six allocations on the 256-color scroll.
6. **Hand `Draw` the bytes of a text run**, not runes re-encoded: the run is a
   span of the input. Later, hand it a zero-copy view of the input (see 12).
7. **Step grapheme clusters only.** `uniseg.FirstGraphemeClusterInString`
   instead of the `Graphemes` iterator, whose `Next` also works out word,
   sentence and line breaks nothing reads. A quarter of the TUI redraw.
8. **Keep the hot modes as booleans beside the `Mode` map.** DECLRMM, DECOM,
   DECAWM, IRM, DECSCNM and LNM are read per cell, per row or per line; a map
   lookup each. `modesChanged` refreshes them at the five places the map is
   written; `TestHotModesFollowMap` drives every write path. This is tracked
   state, which the explicit-control-flow value dislikes; the test is the
   mitigation. Nothing outside te writes `Mode` (vtapp, vtappview and iface
   only read it).
9. **SGR in place, as a switch.** No queue copy, no map of strings like
   `"+bold"`, sub-parameters read from after their attribute exactly as the
   queue consumed them. Half the 256-color scroll's allocations.
10. **Segment once.** `Draw` itself walks clusters and resumes an ASCII run
    wherever one begins, instead of segmenting to find run boundaries and
    then segmenting again inside the general path. Whether the charset is the
    identity is remembered by the table (`charsetIdentity`); `translate`
    returns its input unchanged when it is. TUI redraw 26 to 16 ms.
11. **A standalone character is drawn without uniseg.** A character that
    extends nothing before it and takes nothing after (probed once by
    segmenting it beside an ASCII letter each way, remembered per rune), not a
    regional indicator or Hangul, followed by another such or by ASCII, is a
    cluster on its own. Greek, Cyrillic and CJK text skips the state machine.
    TUI redraw 12 to 10 ms. The exclusions are hand-listed and tied to
    uniseg's rules; the differential test's pieces cover them.
12. **`Draw` receives a view of the input, good for the call only.** Documented
    on `EventHandler.Draw`; the two places te kept the text clone it (the
    grapheme table on first sight; the last glyph drawn, for REP, kept as a
    cell). Thousands of allocations per run to about a thousand. The only
    other `EventHandler` implementations are test doubles.

### Group two: the cell

13. **Store a cell in eight bytes.** The screen's rows become `[][]gcell`: a
    codepoint, or with a flag an index into a per-screen table of
    multi-codepoint clusters; a 16-bit index into a per-screen table of
    distinct `Attr`s; wrap and empty flags. `cell.Cell` stays the type at the
    edges: `Rows()` (was `Buffer`, now a copy), `ScrollbackRows()`, snapshots,
    `Fill`. Tables are shared by the alternate screen and the history and are
    compacted to what those still hold when the style table fills
    (`Screen.moreRows` lists every row set that indexes them). A scrolled row
    is 960 bytes instead of 7.7 KB, a full-screen erase 38 KB instead of
    307 KB, and no cell holds a pointer. Plain scroll 36 to 16 ms, tmux 35 to
    24, on its own; it also made every later change possible.
14. **Hints the tables make free.** An erase reads no cell until some style in
    the table protects one (`guarded`, set by `styleID`, recomputed by
    compaction), then copies the blank row in; a draw works out its style's
    index once and writes it into each cell; the last `Attr` asked about is
    kept with its index; the width of each character is remembered. These
    are Ghostty's per-row `styled` and per-cell `style_id` at the grain a
    table gives. Tmux 14 to 9 ms.
15. **The delta path native to the cell.** This is the one that decides
    whether 13 is worth it for nxterm. Left as it was, the model compared
    `cell.Cell` rows and so unpacked the whole screen on every cut and every
    apply, and the scroll hook unpacked a row per scroll: tmux's apply went
    from 0.1 ms to 10 ms and plain scroll's feed from 5 ms to 47. Made native
    (`gdiff.go`): the model holds gcells sharing the screen's tables; a cut
    compares eight bytes a cell and unpacks only the cells of each span for
    the runs the wire carries (two gcells are equal exactly when their cells
    are, each text and attribute being in the tables once); the hook
    compares gcell rows; a shadow applies scrolls by moving row slices and
    packs spans as it writes them. `cell` exports the builders and checks
    (`export.go`: `Table`, `Runs`, `Check`, `Each`, `Entry`, `Scroll.Check`,
    `IsUnseen`) so the delta's logic stays in one place; `cell.Differ` is
    untouched for the one-plane frontend. Result: a cut a third of the
    original's cost, an apply a sixteenth on scrolls, the hooked feed twice
    the unhooked rather than nine times.

### Before nxterm can take group two

- **Row accessors for the views.** `cores/vtappview/cut.go`, `vtappview.go`
  and `predict.go` index `screen.Buffer[line-live]` and call
  `ScrollbackRows()` per cut; `cores/vtapp/cut.go` slices `ScrollbackRows()`
  for a `Fill`. With copies, `ScrollbackRows()` unpacks the whole history on
  every paint. te needs `Row(y)` and `HistoryRow(seq)` that unpack one row on
  demand, and the views need to use them.
- **A wider style index.** A screen plus a thousand-line history is about
  125k cells; a truecolor image from chafa or timg gives nearly every cell its
  own attribute, so a scrolled image can exceed 65,535 styles, and when
  compaction cannot help the colours are lost. Use 24 bits (text u32, style
  u24, flags u8 still fits eight bytes), or scope the table per block of rows
  as Ghostty scopes per page.
- **Grapheme compaction.** The grapheme table only shrinks when the style
  table fills; give it its own trigger.
- **`nxtest` and the shim** use `LinesCells()` and `Rows()`, which are copies;
  fine for tests, but anything that reads cells at paint rate should go
  through the row accessors above.

## What did not work

- **Copying a blank row template instead of filling a loop**, while cells were
  64 bytes: no gain. The erase and refill were bandwidth-bound by the cell
  size, which is what pointed at the cell change. The same copy did pay once
  cells were eight bytes (item 14).
- **SIMD-within-a-register scanning for printable runs** (eight bytes a step
  through a 64-bit word): flat on plain scroll, worse on tmux whose runs are
  three characters long. The scan was already under 5% of the time; Ghostty's
  SIMD pays because it feeds a batched cell fill behind a compiler that
  removes bounds checks, which Go source cannot imitate.
- **A table-driven parser was not attempted.** Parsing is about 28% of the
  TUI redraw, 25% of the 256-color scroll and 15% of tmux; halving it would
  be worth 1.4, 1.3 and 0.5 ms, optimistic for Go, against rewriting the
  1,200-line parser with te's quirks (the deferred `'` final, the 8-bit
  control toggle, UTF-8 mode refusing C1, charset designations, CAN/SUB
  aborts, ESC-not-ST recovery in OSC and DCS). Worth doing to learn the
  parser's cost, not for throughput.

## What Ghostty's design teaches, in nxterm's terms

- **Make the hot path cheap by making the cell small**, and move everything
  variable-length out of it into tables the cell indexes. Equality of cells
  becomes equality of integers, erasing becomes a memory move, and the
  collector has nothing to trace.
- **Derive hints from structure you already maintain** rather than tracking
  state beside it. `guarded` comes from the style table's contents and can
  never be a false negative; the hot-mode booleans had to be synced and
  tested. Prefer the first kind.
- **A fast path is a prefix the general path would have consumed anyway.**
  Ghostty's `printSlice` fills what it can and returns how many codepoints
  it took; `print` handles the next one; then the fast path tries again. te's
  `Draw` now has the same shape. The general path stays the definition.
- **Change records fit the hop they cross.** Ghostty's renderer shares memory
  with its terminal and needs only a dirty bit per row; nx's shadows cannot
  see the screen and need a delta. The lesson for nx is not to adopt dirty
  bits but to make the delta's producer and consumer work in the storage
  form, so that neither side converts the whole screen to talk to the other.
- **The remaining gap is the language.** Two times on plain scroll and the TUI
  redraw is spread across cell writes, row moves, width and style lookups and
  per-byte parsing, each a little slower in Go than in Zig with SIMD and a
  table-driven machine. There is no single function left to fix.
