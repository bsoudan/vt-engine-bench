package te

import (
	"strings"
	"testing"

	"nx/cell"
)

// TestScrollbackNotPollutedByAltOrRegion verifies that scrollback (and the
// TotalAdded sequence counter the client sync protocol keys off) only accrues
// from full-screen scrolling on the primary buffer — not from alt-screen
// scrolling (less/vim) or DECSTBM scroll-region scrolling (status-line apps).
func TestScrollbackNotPollutedByAltOrRegion(t *testing.T) {
	t.Parallel()
	t.Run("alt screen", func(t *testing.T) {
		screen := NewScrollbackScreen(20, 5, 100)
		stream := NewStream(screen)
		stream.Feed("\x1b[?1049h") // enter alt screen
		for i := 0; i < 10; i++ {
			stream.Feed("line\r\n")
		}
		if got := screen.TotalAdded(); got != 0 {
			t.Errorf("alt-screen scrolling added %d lines to scrollback, want 0", got)
		}
	})

	t.Run("scroll region", func(t *testing.T) {
		screen := NewScrollbackScreen(20, 5, 100)
		stream := NewStream(screen)
		stream.Feed("\x1b[2;4r") // DECSTBM: region rows 2..4 (1-based), top margin > 0
		stream.Feed("\x1b[4;1H") // cursor to region bottom
		for i := 0; i < 10; i++ {
			stream.Feed("x\r\n")
		}
		if got := screen.TotalAdded(); got != 0 {
			t.Errorf("region scrolling added %d lines to scrollback, want 0", got)
		}
	})

	t.Run("full screen still accrues", func(t *testing.T) {
		screen := NewScrollbackScreen(20, 5, 100)
		stream := NewStream(screen)
		for i := 0; i < 10; i++ {
			stream.Feed("line\r\n")
		}
		if screen.TotalAdded() == 0 {
			t.Error("full-screen scrolling added nothing to scrollback")
		}
	})
}

// TestAProgramDoesNotResizeTheScreen: CSI 8;rows;cols t (XTWINOPS resize)
// leaves the screen the size its pty is. A pseudoconsole sends one after each
// resize it is given, late: two resizes in quick succession, and the first's
// came after the second was made, growing a vtapp's screen back to rows its
// pty no longer had, its last line and cursor below what its view shows.
func TestAProgramDoesNotResizeTheScreen(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(80, 21, 100)
	stream := NewStream(screen)
	stream.Feed("one\r\ntwo")
	stream.Feed("\x1b[8;23;80t")
	stream.Feed("\x1b[8;5;40t")
	if screen.Lines != 21 || screen.Columns != 80 {
		t.Fatalf("the program's resize was done: %dx%d, want 80x21", screen.Columns, screen.Lines)
	}
	if screen.Cursor.Row != 1 || screen.Cursor.Col != 3 {
		t.Fatalf("the cursor moved to %d,%d; want 1,3", screen.Cursor.Row, screen.Cursor.Col)
	}
}

// TestResizeShrinkNoCrash reproduces a client crash: Screen.Resize, which a
// program's CSI 8;rows;cols t reached when it was done (not the
// scrollback-aware ScrollbackScreen.Resize), shrank the screen without
// truncating the cell buffer, leaving len(Buffer) > Lines so LinesCells
// panicked with index out of range. The invariant must hold in
// Screen.Resize itself.
func TestResizeShrinkNoCrash(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(80, 24, 100)
	for i := 0; i < 24; i++ {
		screen.Draw(strings.Repeat("x", 80))
		if i != 23 {
			screen.LineFeed()
		}
	}

	screen.Screen.Resize(5, 80)

	if screen.Lines != 5 {
		t.Fatalf("Lines = %d, want 5", screen.Lines)
	}
	if len(screen.Buffer) != screen.Lines {
		t.Fatalf("len(Buffer) = %d, want %d (Lines) — buffer not truncated on shrink", len(screen.Buffer), screen.Lines)
	}
	if screen.Cursor.Row >= screen.Lines {
		t.Fatalf("cursor row %d out of range for %d lines", screen.Cursor.Row, screen.Lines)
	}

	cells := screen.LinesCells()
	if len(cells) != screen.Lines {
		t.Fatalf("LinesCells len = %d, want %d", len(cells), screen.Lines)
	}
}

func checkSeqInvariant(t *testing.T, screen *ScrollbackScreen, desc string) {
	t.Helper()
	first := screen.FirstSeq()
	total := screen.TotalAdded()
	sb := screen.Scrollback()
	// Top covers exactly len(items) rows starting at FirstSeq. The
	// newest retained row's seq is FirstSeq + Scrollback - 1, which
	// must not exceed TotalAdded - 1. Empty Top → FirstSeq == TotalAdded.
	if first > total {
		t.Fatalf("%s: FirstSeq(%d) > TotalAdded(%d)", desc, first, total)
	}
	if first+uint64(sb) > total {
		t.Fatalf("%s: FirstSeq(%d) + Scrollback(%d) > TotalAdded(%d)",
			desc, first, sb, total)
	}
	if sb == 0 && first != total {
		t.Fatalf("%s: empty Top but FirstSeq(%d) != TotalAdded(%d)",
			desc, first, total)
	}
}

func TestHistoryFirstSeqLineFeed(t *testing.T) {
	t.Parallel()
	// 3-row screen, 5-row scrollback cap. 20 LineFeeds — the first 2
	// move the cursor from row 0 to the bottom without scrolling, the
	// remaining 18 each scroll one row off the top.
	screen := NewScrollbackScreen(5, 3, 5)
	screen.SetMode([]int{ModeLNM}, false)
	for range 20 {
		screen.Draw("X")
		screen.LineFeed()
		checkSeqInvariant(t, screen, "after linefeed")
	}
	if screen.TotalAdded() != 18 {
		t.Fatalf("TotalAdded=%d, want 18", screen.TotalAdded())
	}
	if screen.Scrollback() != 5 {
		t.Fatalf("Scrollback=%d, want 5", screen.Scrollback())
	}
	if screen.FirstSeq() != 13 {
		t.Fatalf("FirstSeq=%d, want 13 (18 total - 5 retained)", screen.FirstSeq())
	}
}

// Shrinking when the rows below the cursor are blank must trim those rows from
// the bottom, not push visible content into history. This is the xterm/tmux
// behavior; the old unconditional top-push hid early output (e.g. a child's
// banner printed before the first resize) behind the scrollback.
func TestHistoryResizeShrinkDropsBlankBottom(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(5, 6, 50)
	screen.Draw("hello")
	screen.LineFeed() // cursor to row 1; rows 1..5 blank

	screen.Resize(4, 5)
	if screen.Scrollback() != 0 {
		t.Fatalf("blank-bottom shrink pushed %d rows into history, want 0", screen.Scrollback())
	}
	assertDisplay(t, screen.Screen, []string{"hello", "     ", "     ", "     "})
	if screen.Cursor.Row != 1 {
		t.Fatalf("cursor row = %d, want 1", screen.Cursor.Row)
	}
}

// When the bottom rows hold content (cursor at the bottom), shrinking must
// still scroll the top rows into history so no content is lost.
func TestHistoryResizeShrinkPushesContentToHistory(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(5, 4, 50)
	screen.SetMode([]int{ModeLNM}, false)
	for i := 0; i < 4; i++ {
		screen.Draw(string(rune('a' + i)))
		if i < 3 {
			screen.LineFeed()
		}
	}

	screen.Resize(2, 5)
	if screen.Scrollback() != 2 {
		t.Fatalf("content shrink pushed %d rows into history, want 2", screen.Scrollback())
	}
	assertDisplay(t, screen.Screen, []string{"c    ", "d    "})
	if got := cell.Text(screen.ScrollbackRows()[0]); strings.TrimRight(got, " ") != "a" {
		t.Fatalf("history[0] = %q, want \"a\"", got)
	}
}

// A mixed shrink trims as many blank below-cursor rows as it can and pushes
// only the remainder from the top.
func TestHistoryResizeShrinkMixed(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(5, 6, 50)
	screen.SetMode([]int{ModeLNM}, false)
	for i := 0; i < 3; i++ {
		screen.Draw(string(rune('a' + i)))
		if i < 2 {
			screen.LineFeed()
		}
	} // rows 0..2 = a,b,c (cursor row 2); rows 3..5 blank

	screen.Resize(2, 5) // shrink by 4: 3 blank bottom rows + 1 from the top
	if screen.Scrollback() != 1 {
		t.Fatalf("mixed shrink pushed %d rows into history, want 1", screen.Scrollback())
	}
	assertDisplay(t, screen.Screen, []string{"b    ", "c    "})
	if got := cell.Text(screen.ScrollbackRows()[0]); strings.TrimRight(got, " ") != "a" {
		t.Fatalf("history[0] = %q, want \"a\"", got)
	}
	if screen.Cursor.Row != 1 {
		t.Fatalf("cursor row = %d, want 1", screen.Cursor.Row)
	}
}

// Growing while a program holds the alt screen must also grow the stashed
// primary buffer — the production crash: client resize while a program held
// the alt screen, alt-screen exit swapped the stale-sized primary back in,
// Display panicked.
func TestHistoryResizeGrowWhileAltScreenActive(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(5, 4, 50)
	screen.Draw("hi")
	screen.SetMode([]int{ModeAltBufCursor}, false)
	screen.Resize(6, 7)

	screen.ResetMode([]int{ModeAltBufCursor}, false)
	if len(screen.Buffer) != screen.Lines {
		t.Fatalf("primary buffer rows = %d, want %d", len(screen.Buffer), screen.Lines)
	}
	for row := range screen.Buffer {
		if len(screen.Buffer[row]) != screen.Columns {
			t.Fatalf("row %d cols = %d, want %d", row, len(screen.Buffer[row]), screen.Columns)
		}
	}
	screen.Display()
}

// Shrinking while the alt screen is active scrolls the stashed primary
// buffer's overflow rows into history, the same as a live shrink would.
func TestHistoryResizeShrinkWhileAltScreenActive(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(5, 4, 50)
	screen.SetMode([]int{ModeLNM}, false)
	for i := 0; i < 4; i++ {
		screen.Draw(string(rune('a' + i)))
		if i < 3 {
			screen.LineFeed()
		}
	}

	screen.SetMode([]int{ModeAltBufCursor}, false)
	screen.Resize(2, 5)

	screen.ResetMode([]int{ModeAltBufCursor}, false)
	if len(screen.Buffer) != screen.Lines {
		t.Fatalf("primary buffer rows = %d, want %d", len(screen.Buffer), screen.Lines)
	}
	if screen.Scrollback() != 2 {
		t.Fatalf("shrink pushed %d rows into history, want 2", screen.Scrollback())
	}
	assertDisplay(t, screen.Screen, []string{"c    ", "d    "})
	if screen.Cursor.Row >= screen.Lines {
		t.Fatalf("restored cursor row %d out of bounds for %d lines", screen.Cursor.Row, screen.Lines)
	}
}

// A line that wraps on the bottom row scrolls the screen from inside Draw:
// each row it pushes off the top goes into the history, a seq of its own,
// as a line feed's does. Lost, a long line's first rows were missing from
// the scrollback, its rest read as the line before's, and the seq the
// shadow keys off fell behind.
func TestALongLineWrappingKeepsEveryRowScrolledOff(t *testing.T) {
	t.Parallel()
	h := NewScrollbackScreen(10, 3, 100)
	var want []string
	for i := range 6 {
		line := strings.Repeat(string(rune('a'+i)), 25) // three rows each
		feed(t, h, line+"\r\n")
		want = append(want, line[:10]+"⏎", line[10:20]+"⏎", line[20:])
	}
	wantShown(t, h, "the rows of six lines of three rows each", want...)
	if got, rows := h.TotalAdded(), uint64(len(h.ScrollbackRows())); got != rows {
		t.Errorf("TotalAdded %d, with %d rows in the history", got, rows)
	}
}
