package te

import "testing"

// A space in the default colors is blank however the default was reached
// — never set, set back by SGR 39 and 49, reset by SGR 0, or left by an
// erase — and every path that asks agrees: the cell (Cell.Blank), a
// snapshot's trim (trimBuffer) and a row (blankRow, which reflow and a
// shrink ask).
func TestADefaultColoredSpaceIsBlankEveryWay(t *testing.T) {
	t.Parallel()
	s := NewScrollbackScreen(12, 3, 10)
	st := NewStream(s)
	_ = st.Feed("\x1b[31;42mx\x1b[39;49m   \x1b[0m   ")          // row 0: x, then spaces
	_ = st.Feed("\x1b[2;1H\x1b[33;44mpainted\x1b[39;49m\x1b[2K") // row 1: erased in the default colors
	for col, c := range s.Rows()[0][1:] {
		if !c.Blank() {
			t.Errorf("row 0, col %d: %+v is not blank", col+1, c)
		}
	}
	if trimmed := trimBuffer([][]Cell{s.Rows()[0]}); len(trimmed[0]) != 1 {
		t.Errorf("a snapshot keeps %d cells of row 0; want the x alone", len(trimmed[0]))
	}
	for row := 1; row < 3; row++ {
		if !blankRow(s.rows[row]) {
			t.Errorf("row %d is not blank: %+v", row, s.Rows()[row])
		}
	}
	// A colored background is not blank, by any of them.
	_ = st.Feed("\x1b[3;1H\x1b[44m\x1b[2K")
	if s.Rows()[2][0].Blank() || blankRow(s.rows[2]) || len(trimBuffer([][]Cell{s.Rows()[2]})[0]) == 0 {
		t.Error("a row erased in a colored background was taken for blank")
	}
}
