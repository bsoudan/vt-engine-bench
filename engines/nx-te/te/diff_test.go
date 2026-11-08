package te

import (
	"slices"
	"testing"
)

// An index on the bottom row scrolls the screen: its rows change. A reverse
// index in the middle, off the top margin, moves the cursor and changes no
// row.
func TestDiffIndexReverseIndex(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	for row := range screen.Buffer {
		screen.Buffer[row][0].Data = string(rune('a' + row))
	}
	rows := func() [][]Cell {
		out := make([][]Cell, len(screen.Buffer))
		for i, r := range screen.Buffer {
			out[i] = slices.Clone(r)
		}
		return out
	}
	same := func(a, b [][]Cell) bool { return slices.EqualFunc(a, b, slices.Equal) }

	before := rows()
	screen.Cursor.Row = screen.Lines - 1
	screen.Index()
	if same(before, rows()) {
		t.Fatalf("an index at the bottom left every row as it was")
	}
	before = rows()
	screen.Cursor.Row = screen.Lines / 2
	screen.ReverseIndex()
	if !same(before, rows()) {
		t.Fatalf("a reverse index off the top margin changed a row")
	}
}
