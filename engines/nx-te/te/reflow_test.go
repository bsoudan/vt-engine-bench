package te

import (
	"reflect"
	"strings"
	"testing"
)

// reflowed is a ScrollbackScreen cols by lines that reflows, with text written
// to it.
func reflowed(t *testing.T, cols, lines, history int, text string) *ScrollbackScreen {
	t.Helper()
	h := NewScrollbackScreen(cols, lines, history)
	h.Reflow = true
	feed(t, h, text)
	return h
}

func feed(t *testing.T, h *ScrollbackScreen, text string) {
	t.Helper()
	if err := NewStream(h).Feed(text); err != nil {
		t.Fatal(err)
	}
}

// rowText is a row as it reads, a wide glyph's placeholder left out and
// trailing blanks trimmed, with "⏎" after it if it wraps into the next.
func rowText(row []Cell) string {
	var b strings.Builder
	for _, c := range row {
		b.WriteString(c.Data)
	}
	s := strings.TrimRight(b.String(), " ")
	if len(row) > 0 && row[len(row)-1].Wrapped {
		s += "⏎"
	}
	return s
}

// shown is the history then the screen, a row each, the screen's trailing
// blank rows left out.
func shown(h *ScrollbackScreen) []string {
	var out []string
	for _, row := range h.ScrollbackRows() {
		out = append(out, rowText(row))
	}
	n := len(h.Buffer)
	for n > 0 && rowText(h.Buffer[n-1]) == "" {
		n--
	}
	for _, row := range h.Buffer[:n] {
		out = append(out, rowText(row))
	}
	return out
}

func wantShown(t *testing.T, h *ScrollbackScreen, what string, want ...string) {
	t.Helper()
	if got := shown(h); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: %q, want %q", what, got, want)
	}
	for r, row := range h.Buffer {
		if len(row) != h.Columns {
			t.Errorf("%s: row %d is %d wide, want %d", what, r, len(row), h.Columns)
		}
	}
}

func wantCursor(t *testing.T, h *ScrollbackScreen, what string, row, col int) {
	t.Helper()
	if h.Cursor.Row != row || h.Cursor.Col != col {
		t.Errorf("%s: the cursor at (%d,%d), want (%d,%d)", what, h.Cursor.Row, h.Cursor.Col, row, col)
	}
}

// A wrapped line is joined when the screen widens and wrapped again when
// it narrows; the cursor keeps its place in its line.
func TestReflowJoinsAndRewraps(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "0123456789abcde\r\n$ ")
	wantShown(t, h, "written", "0123456789⏎", "abcde", "$")
	wantCursor(t, h, "written", 2, 2)

	h.Resize(4, 20)
	wantShown(t, h, "widened", "0123456789abcde", "$")
	wantCursor(t, h, "widened", 1, 2)

	h.Resize(4, 10)
	wantShown(t, h, "narrowed back", "0123456789⏎", "abcde", "$")
	wantCursor(t, h, "narrowed back", 2, 2)

	h.Resize(4, 4)
	wantShown(t, h, "narrower", "0123⏎", "4567⏎", "89ab⏎", "cde", "$")
	wantCursor(t, h, "narrower", 3, 2)
	if got := len(h.ScrollbackRows()); got != 1 {
		t.Errorf("narrower: %d rows in the history, want the 1 that no longer fits", got)
	}
}

// A line wrapped across the history's edge is joined as one; the screen
// starts where its first row went.
func TestReflowJoinsAcrossTheHistory(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 3, 100, "aaaaaaaaaaBBBBB\r\nx\r\ny")
	if got := len(h.ScrollbackRows()); got != 1 {
		t.Fatalf("written: %d rows in the history, want 1", got)
	}
	h.Resize(3, 20)
	wantShown(t, h, "widened", "aaaaaaaaaaBBBBB", "x", "y")
	if got := len(h.ScrollbackRows()); got != 0 {
		t.Errorf("widened: %d rows in the history, want the line joined onto the screen", got)
	}
	wantCursor(t, h, "widened", 2, 1)
}

// A line exactly the width ended by a line break is not joined to the
// next: the wrap waiting for a next character never came.
func TestReflowKeepsALineOfExactlyTheWidth(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "0123456789\r\nnext")
	h.Resize(4, 20)
	wantShown(t, h, "widened", "0123456789", "next")
}

// A wide glyph that would straddle the edge goes on the next row whole,
// the space it leaves not part of the line: joined again, it is gone.
func TestReflowWideGlyphAtTheEdge(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "123456789世界")
	wantShown(t, h, "written", "123456789⏎", "世界")
	if !h.Buffer[0][9].Empty {
		t.Errorf("written: the cell the glyph left is %+v, want Empty", h.Buffer[0][9])
	}

	h.Resize(4, 20)
	wantShown(t, h, "widened", "123456789世界")
	wantCursor(t, h, "widened", 0, 13)

	h.Resize(4, 10)
	wantShown(t, h, "narrowed back", "123456789⏎", "世界")
	if !h.Buffer[0][9].Empty {
		t.Errorf("narrowed back: the cell the glyph left is %+v, want Empty", h.Buffer[0][9])
	}
	if h.Buffer[1][0].Data != "世" || h.Buffer[1][1].Data != "" {
		t.Errorf("narrowed back: the glyph is %+v %+v, want it and its placeholder", h.Buffer[1][0], h.Buffer[1][1])
	}
}

// A coloured blank at a line's end is written, and kept.
func TestReflowKeepsAColouredBlank(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "ab\x1b[44m   \x1b[0m\r\nc")
	for _, cols := range []int{20, 4, 10} {
		h.Resize(4, cols)
		var blue int
		for _, row := range h.Buffer {
			for _, c := range row {
				if c.Attr.Bg.Mode != ColorDefault {
					blue++
				}
			}
		}
		if blue != 3 {
			t.Errorf("at %d columns: %d coloured cells, want 3", cols, blue)
		}
	}
	wantShown(t, h, "back at 10", "ab", "c")
}

// The cursor in the middle of a line stays at its character; on a blank
// row below what was written, it stays that many rows below it.
func TestReflowKeepsTheCursor(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "0123456789abcdef\x1b[3D")
	wantCursor(t, h, "written", 1, 3)
	h.Resize(4, 20)
	wantCursor(t, h, "widened", 0, 13)

	h = reflowed(t, 10, 6, 100, "0123456789ab\r\n\r\n\r\n")
	wantCursor(t, h, "below", 4, 0)
	h.Resize(6, 20)
	wantShown(t, h, "widened", "0123456789ab")
	wantCursor(t, h, "widened, below", 3, 0)
}

// A cursor waiting to wrap at the edge is past the end of its line: widened,
// the next character goes on the same row; narrowed to the line's length,
// it waits there.
func TestReflowAWrapThatWaits(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "0123456789")
	h.Resize(4, 20)
	wantCursor(t, h, "widened", 0, 10)
	feed(t, h, "X")
	wantShown(t, h, "widened, then a character", "0123456789X")

	h = reflowed(t, 10, 4, 100, "01234")
	h.Resize(4, 5)
	wantCursor(t, h, "narrowed to the line", 0, 5)
	feed(t, h, "X")
	wantShown(t, h, "narrowed, then a character", "01234⏎", "X")
}

// A narrowing that makes more rows than the history holds drops the
// oldest, as a scroll does; the newest stay.
func TestReflowBoundsTheHistory(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 2, 3, "0123456789\r\nabcdefghij\r\nABCDEFGHIJ\r\nend")
	h.Resize(2, 5)
	hist := h.ScrollbackRows()
	if len(hist) != 3 {
		t.Fatalf("narrowed: %d rows in the history, want 3", len(hist))
	}
	if got := rowText(hist[2]); got != "ABCDE⏎" {
		t.Errorf("narrowed: the newest history row %q, want %q", got, "ABCDE⏎")
	}
	wantShown(t, h, "narrowed", "abcde⏎", "fghij", "ABCDE⏎", "FGHIJ", "end")
}

// Under the alt screen, the program's screen is fitted, not reflowed; the
// primary screen stashed under it is reflowed, and back on it the cursor
// is where it was in its line.
func TestReflowLeavesTheAltScreen(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "0123456789abc\r\n$ \x1b[?1049h\x1b[HALTALTALT")
	h.Resize(4, 20)
	if got := rowText(h.Buffer[0]); got != "ALTALTALT" {
		t.Errorf("the alt screen's first row %q after widening, want it as written", got)
	}
	feed(t, h, "\x1b[?1049l")
	wantShown(t, h, "back on the primary", "0123456789abc", "$")
	wantCursor(t, h, "back on the primary", 1, 2)
}

// A reflow's history is new rows: its seqs follow on from every seq before
// it, so none names a different row after.
func TestReflowSeqsOnlyGrow(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 2, 100, "0123456789abc\r\nx\r\ny\r\nz")
	total, first := h.TotalAdded(), h.FirstSeq()
	h.Resize(2, 4)
	if h.FirstSeq() != total || h.TotalAdded() != total+uint64(len(h.ScrollbackRows())) {
		t.Errorf("seqs [%d,%d) after the reflow, from [%d,%d) before; want them to start at %d",
			h.FirstSeq(), h.TotalAdded(), first, total, total)
	}
}

// Without Reflow, a resize pads or cuts each row, as it always has.
func TestResizeWithoutReflowCuts(t *testing.T) {
	t.Parallel()
	h := NewScrollbackScreen(10, 4, 100)
	feed(t, h, "0123456789\r\n$ ")
	h.Resize(4, 5)
	h.Resize(4, 10)
	wantShown(t, h, "narrowed and widened", "01234", "$")
}

// A saved cursor (DECSC) is moved as the cursor is: restored after a
// reflow, it is at the character it was saved at.
func TestReflowMovesASavedCursor(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "0123456789abc\x1b7\r\n$ ")
	h.Resize(4, 20)
	feed(t, h, "\x1b8")
	wantCursor(t, h, "restored", 0, 13)
}

// The screen starts where its first row went: a screen cleared with the
// prompt on its top row keeps it there, the history not pulled down.
func TestReflowKeepsTheScreensTop(t *testing.T) {
	t.Parallel()
	h := reflowed(t, 10, 4, 100, "a\r\nb\r\nc\r\nd\r\ne\r\n\x1b[2J\x1b[H$ ")
	before := len(h.ScrollbackRows())
	h.Resize(4, 20)
	if rowText(h.Buffer[0]) != "$" {
		t.Errorf("widened: the top row %q, want the prompt", rowText(h.Buffer[0]))
	}
	wantCursor(t, h, "widened", 0, 2)
	if got := len(h.ScrollbackRows()); got < before {
		t.Errorf("widened: %d rows in the history, from %d: pulled onto the screen", got, before)
	}
}
