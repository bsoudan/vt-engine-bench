package te

import (
	"fmt"
	"testing"
)

// pyte's history tests that are not about paging, which pyte hooked onto
// ESC N / ESC P and nx never drives: the screen stays live, and a view
// draws the scrollback itself.

func pyteHistoryChars(lines [][]Cell, columns int) []string {
	out := make([]string, len(lines))
	for y := range lines {
		row := lines[y]
		buf := make([]rune, columns)
		for x := 0; x < columns; x++ {
			if x < len(row) && row[x].Data != "" {
				buf[x] = []rune(row[x].Data)[0]
			} else {
				buf[x] = ' '
			}
		}
		out[y] = string(buf)
	}
	return out
}

// From pyte/tests/test_history.py::test_index
func TestPyteTestHistoryIndex(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(5, 5, 50)
	for idx := 0; idx < screen.Lines; idx++ {
		screen.Draw(fmt.Sprintf("%d", idx))
		if idx != screen.Lines-1 {
			screen.LineFeed()
		}
	}
	if len(screen.scrollback.Top.items) != 0 {
		t.Fatalf("expected empty history")
	}
	line := screen.Rows()[0]
	screen.Index()
	if len(screen.scrollback.Top.items) == 0 {
		t.Fatalf("expected top history")
	}
	if pyteHistoryChars(screen.ScrollbackRows()[len(screen.scrollback.Top.items)-1:], screen.Columns)[0] != pyteHistoryChars([][]Cell{line}, screen.Columns)[0] {
		t.Fatalf("unexpected history line")
	}

	line = screen.Rows()[0]
	screen.Index()
	if len(screen.scrollback.Top.items) != 2 {
		t.Fatalf("expected 2 history entries")
	}
	if pyteHistoryChars(screen.ScrollbackRows()[len(screen.scrollback.Top.items)-1:], screen.Columns)[0] != pyteHistoryChars([][]Cell{line}, screen.Columns)[0] {
		t.Fatalf("unexpected history line")
	}

	for i := 0; i < 100; i++ {
		screen.Index()
	}
	if len(screen.scrollback.Top.items) != 50 {
		t.Fatalf("expected history rotation")
	}
}

// From pyte/tests/test_history.py::test_reverse_index, less what pyte kept
// below the screen for paging: a reverse index scrolls the screen down, and
// the scrollback above it is not touched.
func TestPyteTestHistoryReverseIndex(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(5, 5, 50)
	for idx := 0; idx < len(screen.Rows()); idx++ {
		screen.Draw(fmt.Sprintf("%d", idx))
		if idx != len(screen.Rows())-1 {
			screen.LineFeed()
		}
	}
	screen.CursorPosition(0, 0)
	for i := 0; i < 10; i++ {
		screen.ReverseIndex()
	}
	if len(screen.scrollback.Top.items) != 0 || screen.TotalAdded() != 0 {
		t.Fatalf("a reverse index kept %d rows in the scrollback", len(screen.scrollback.Top.items))
	}
}

// From pyte/tests/test_history.py::test_erase_in_display: erasing the saved
// lines (ED 3) clears the scrollback.
func TestPyteTestHistoryEraseInDisplay(t *testing.T) {
	t.Parallel()
	screen := NewScrollbackScreen(5, 5, 6)
	screen.SetMode([]int{ModeLNM}, false)
	for idx := 0; idx < screen.Lines; idx++ {
		screen.Draw(fmt.Sprintf("%d", idx))
		screen.LineFeed()
	}
	if len(screen.scrollback.Top.items) == 0 {
		t.Fatalf("expected top history")
	}
	screen.EraseInDisplay(3, false)
	if len(screen.scrollback.Top.items) != 0 || screen.FirstSeq() != screen.TotalAdded() {
		t.Fatalf("expected history reset")
	}
}
