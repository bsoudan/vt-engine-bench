package te

import "testing"

// With autowrap off, a character written at the right margin leaves the cursor
// on the margin column, as xterm does: the next character replaces the last
// one, and an erase-to-end-of-line from there takes it. The cursor used to sit
// one past the edge, where nothing could reach the last column again — which
// hid a renderer that erased after painting the last column, since this
// emulator is what the test harness scrapes the console with.
func TestNoAutowrapCursorStaysOnMargin(t *testing.T) {
	t.Parallel()
	s := NewScrollbackScreen(10, 2, 10)
	s.WriteProcessInput = func(string) {}
	st := NewStream(s)
	_ = st.Feed("\x1b[?7l0123456789")
	if s.Cursor.Col != 9 {
		t.Fatalf("cursor col %d after filling the row with autowrap off, want 9 (the margin)", s.Cursor.Col)
	}
	_ = st.Feed("X")
	if got := s.Display()[0]; got != "012345678X" {
		t.Fatalf("row %q, want the margin character replaced: 012345678X", got)
	}
	_ = st.Feed("\x1b[K")
	if got := s.Display()[0]; got != "012345678 " {
		t.Fatalf("row %q after EL at the margin, want the last column erased", got)
	}
	if s.Cursor.Row != 0 {
		t.Fatalf("cursor moved to row %d; autowrap is off", s.Cursor.Row)
	}
}
