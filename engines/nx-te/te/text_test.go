package te

import (
	"encoding/json"
	"strings"
	"testing"
)

// fed is a screen of cols by lines after being fed s.
func fed(t *testing.T, cols, lines int, s string) *ScrollbackScreen {
	t.Helper()
	h := NewScrollbackScreen(cols, lines, 100)
	if err := NewStream(h).FeedBytes([]byte(s)); err != nil {
		t.Fatal(err)
	}
	return h
}

// rowsOf is what a screen holds, history first.
func rowsOf(h *ScrollbackScreen) [][]Cell {
	return append(append([][]Cell(nil), h.ScrollbackRows()...), h.Buffer...)
}

// screenRows is what a screen of cols shows after being fed s, history first.
func screenRows(t *testing.T, cols, lines int, s string) [][]Cell {
	t.Helper()
	return rowsOf(fed(t, cols, lines, s))
}

func TestTextIsWhatASelectionCopies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		cols     int
		feed     string
		r0, r1   int // rows, inclusive
		from, to int
		want     string
	}{
		{"part of a line", 20, "hello world", 0, 0, 6, 10, "world"},
		{"trailing blanks dropped", 20, "hi", 0, 0, 0, 19, "hi"},
		{"lines joined by breaks", 20, "one\r\ntwo\r\nthree", 0, 2, 1, 2, "ne\ntwo\nthr"},
		{"a blank line kept", 20, "a\r\n\r\nb", 0, 2, 0, 19, "a\n\nb"},
		{"a wrapped line is one line", 10, "0123456789abc\r\nnext", 0, 2, 0, 9, "0123456789abc\nnext"},
		// The line fills the row and ends: the terminal never wrapped it.
		{"a full line that ends is not wrapped", 10, "0123456789\r\nnext", 0, 1, 0, 9, "0123456789\nnext"},
		// A blank in the last column is text when the line wrapped there.
		{"a wrapped line keeps the blank at the edge", 10, "012345678 abc", 0, 1, 0, 9, "012345678 abc"},
		// A wide glyph that does not fit wraps whole, and leaves the column blank.
		{"a wide glyph wrapped whole", 5, "abcd世x", 0, 1, 0, 4, "abcd世x"},
		{"a line wrapped twice", 4, "abcdefghij\r\nk", 0, 3, 0, 3, "abcdefghij\nk"},
		{"a wide glyph once", 10, "a世界b", 0, 0, 0, 9, "a世界b"},
		{"from history into the screen", 10, "h1\r\nh2\r\nh3\r\nl1\r\nl2", 1, 4, 0, 9, "h2\nh3\nl1\nl2"},
		// A wrap scrolled into the history is still a wrap.
		{"a wrapped line in the history", 10, "0123456789abc\r\n1\r\n2\r\n3\r\n4", 0, 1, 0, 9, "0123456789abc"},
		// What overwrites the last cell, or erases the row, is not the line
		// that wrapped: it no longer goes on.
		{"the last cell written over", 10, "0123456789abc\x1b[1;10HZ", 0, 1, 0, 9, "012345678Z\nabc"},
		{"the row erased", 10, "0123456789abc\x1b[1;1H\x1b[2K", 0, 1, 0, 9, "\nabc"},
		// Rewriting a wrapped line through the edge again, as a shell's line
		// editor does, wraps it again.
		{"a line rewritten through the edge", 10, "0123456789abc\x1b[1;1H0123456789ABC", 0, 1, 0, 9, "0123456789ABC"},
		// A combining mark that joins the glyph at the wrapping edge leaves the
		// wrap where it was.
		{"a mark on the glyph at the edge", 10, "0123456789ab\r\u0301", 0, 1, 0, 9, "012345678" + "9\u0301" + "ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := screenRows(t, tc.cols, 3, tc.feed)
			if got := Text(rows[tc.r0:tc.r1+1], tc.from, tc.to); got != tc.want {
				t.Errorf("Text = %q, want %q\nrows:\n%s", got, tc.want, dumpRows(rows))
			}
		})
	}
}

// A wrap is the row's, and goes where the row goes: a snapshot carries it, so
// a shadow screen built from one copies as the screen it shadows does.
func TestAWrapSurvivesASnapshot(t *testing.T) {
	t.Parallel()
	h := fed(t, 10, 3, "0123456789abc\r\n1\r\n2\r\n3\r\nxyzXYZxyzXYZ")
	b, err := json.Marshal(h.MarshalState())
	if err != nil {
		t.Fatal(err)
	}
	var st ScrollbackState
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	shadow := NewScrollbackScreen(10, 3, 100)
	shadow.UnmarshalState(&st)
	rows := rowsOf(shadow)
	if got := Text(rows[0:2], 0, 9); got != "0123456789abc" {
		t.Errorf("the history's wrap was lost: %q\n%s", got, dumpRows(rows))
	}
	n := len(rows)
	if got := Text(rows[n-2:], 0, 9); got != "xyzXYZxyzXYZ" {
		t.Errorf("the screen's wrap was lost: %q\n%s", got, dumpRows(rows))
	}
}

func dumpRows(rows [][]Cell) string {
	var b strings.Builder
	for _, r := range rows {
		for _, c := range r {
			b.WriteString(c.Data)
		}
		if len(r) > 0 && r[len(r)-1].Wrapped {
			b.WriteString("↩")
		}
		b.WriteString("|\n")
	}
	return b.String()
}

// resized is a screen of cols by 3 fed s, then made to width.
func resized(t *testing.T, cols int, s string, widths ...int) *ScrollbackScreen {
	t.Helper()
	h := fed(t, cols, 3, s)
	for _, w := range widths {
		h.Resize(3, w)
	}
	return h
}

// then is h after being fed s.
func then(t *testing.T, h *ScrollbackScreen, s string) *ScrollbackScreen {
	t.Helper()
	if err := NewStream(h).FeedBytes([]byte(s)); err != nil {
		t.Fatal(err)
	}
	return h
}

// A line copies the same whatever has been done to its row's width since
// it was written: widened, the space added is Empty and the line still goes
// on at the next row; narrowed, what fits is kept and the line still goes
// on; characters deleted leave Empty space at the end; characters inserted
// push what is at the end out, as on a VT, and the line still goes on.
func TestAWrappedLineCopiesTheSameAfterItsRowChanges(t *testing.T) {
	t.Parallel()
	const wrapped = "0123456789abc\r\nnext" // wraps once on 10 columns
	for _, tc := range []struct {
		name string
		h    *ScrollbackScreen
		want string // rows 0 to 2
	}{
		{"widened", resized(t, 10, wrapped, 15), "0123456789abc\nnext"},
		{"widened twice", resized(t, 10, wrapped, 12, 20), "0123456789abc\nnext"},
		{"widened and narrowed back", resized(t, 10, wrapped, 15, 10), "0123456789abc\nnext"},
		{"narrowed: what no longer fits is lost", resized(t, 10, wrapped, 8), "01234567abc\nnext"},
		{"a full line widened is not wrapped", resized(t, 10, "0123456789\r\nabc\r\nnext", 15), "0123456789\nabc\nnext"},
		{"characters deleted", fed(t, 10, 3, wrapped+"\x1b[1;3H\x1b[2P"), "01456789abc\nnext"},
		{"the last character deleted", fed(t, 10, 3, wrapped+"\x1b[1;10H\x1b[P"), "012345678abc\nnext"},
		{"characters deleted, then widened", resized(t, 10, wrapped+"\x1b[1;3H\x1b[2P", 15), "01456789abc\nnext"},
		{"characters inserted: the end is pushed out", fed(t, 10, 3, wrapped+"\x1b[1;3H\x1b[2@"), "01  234567abc\nnext"},
		// Inserting into a widened row pushes out the space it never filled
		// first: nothing of the line is lost.
		{"characters inserted into a widened row", then(t, resized(t, 10, wrapped, 15), "\x1b[1;1H\x1b[2@"), "  0123456789abc\nnext"},
		// Rows in the history keep the width they were written at: widening
		// does not touch them, and they copy as they did.
		{"a wrapped line in the history, widened", resized(t, 10, wrapped+"\r\n1\r\n2\r\n3", 15), "0123456789abc\nnext"},
		// A wide glyph that did not fit wrapped whole; widened, still one line.
		{"a wide glyph wrapped whole, widened", resized(t, 5, "abcd世x\r\nnext", 8), "abcd世x\nnext"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := rowsOf(tc.h)
			if got := Text(rows[:3], 0, len(rows[2])-1); got != tc.want {
				t.Errorf("Text = %q, want %q\nrows:\n%s", got, tc.want, dumpRows(rows))
			}
		})
	}
}

// Widened and narrowed back, a row is the row it was, cell for cell.
func TestAWrappedRowWidenedAndNarrowedBackIsAsItWas(t *testing.T) {
	t.Parallel()
	before := rowsOf(fed(t, 10, 3, "0123456789abc"))
	after := rowsOf(resized(t, 10, "0123456789abc", 15, 10))
	for r := range before {
		for c := range before[r] {
			if before[r][c] != after[r][c] {
				t.Fatalf("row %d col %d: %+v, was %+v", r, c, after[r][c], before[r][c])
			}
		}
	}
}

// What is written into the space a widening added is text, and the line
// ends there: the Empty space before it is a blank, the Empty space after
// it still nothing.
func TestTextWrittenIntoEmptySpace(t *testing.T) {
	t.Parallel()
	rows := rowsOf(then(t, resized(t, 10, "0123456789abc", 15), "\x1b[1;12HX"))
	if got := Text(rows[:2], 0, 14); got != "0123456789 Xabc" {
		t.Errorf("Text = %q\n%s", got, dumpRows(rows))
	}
}

// A snapshot drops the blank cells at the end of a row, and pads them back:
// but not a wrapped row's Empty space, nor its last cell, which a copy reads
// — a widened row, or one characters were deleted from, copies the same
// from a snapshot as from the screen it was taken of.
func TestEmptySpaceSurvivesASnapshot(t *testing.T) {
	t.Parallel()
	for _, h := range []*ScrollbackScreen{
		resized(t, 10, "0123456789abc", 15),
		fed(t, 10, 3, "0123456789abc\x1b[1;10H\x1b[P"),
	} {
		want := Text(rowsOf(h)[:2], 0, 14)
		b, err := json.Marshal(h.MarshalState())
		if err != nil {
			t.Fatal(err)
		}
		var st ScrollbackState
		if err := json.Unmarshal(b, &st); err != nil {
			t.Fatal(err)
		}
		shadow := NewScrollbackScreen(h.Columns, 3, 100)
		shadow.UnmarshalState(&st)
		rows := rowsOf(shadow)
		if got := Text(rows[:2], 0, 14); got != want {
			t.Errorf("from a snapshot: %q, want %q\n%s", got, want, dumpRows(rows))
		}
	}
}

// Within left and right margins (DECLRMM) what autowrap carries on is a
// column of the margins, not the row's line: nothing is flagged, and a copy
// of the rows is rows.
func TestAWrapWithinMarginsIsNotALine(t *testing.T) {
	t.Parallel()
	h := fed(t, 10, 3, "\x1b[?69h\x1b[3;6s\x1b[1;3Habcdefg")
	rows := rowsOf(h)
	for r, row := range rows {
		for c, cl := range row {
			if cl.Wrapped || cl.Empty {
				t.Fatalf("row %d col %d is flagged: %+v\n%s", r, c, cl, dumpRows(rows))
			}
		}
	}
	if got := Text(rows[:2], 0, 9); got != "  abcd\n  efg" {
		t.Errorf("Text = %q\n%s", got, dumpRows(rows))
	}
}
