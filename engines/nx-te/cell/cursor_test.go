package cell

import "testing"

// Each DECSCUSR number is its shape and blink, as xterm draws them: 0, the
// default, a blinking block; one DECSCUSR doesn't define, the default.
func TestCursorStyleIsDECSCUSRs(t *testing.T) {
	for _, c := range []struct {
		n      int
		style  CursorStyle
		shape  CursorShape
		blink  bool
		said   string
		steady CursorStyle
	}{
		{0, 0, CursorBlock, true, "default", 2},
		{1, 1, CursorBlock, true, "blinking block", 2},
		{2, 2, CursorBlock, false, "steady block", 2},
		{3, 3, CursorUnderline, true, "blinking underline", 4},
		{4, 4, CursorUnderline, false, "steady underline", 4},
		{5, 5, CursorBar, true, "blinking bar", 6},
		{6, 6, CursorBar, false, "steady bar", 6},
		{7, 0, CursorBlock, true, "default", 2},
		{-1, 0, CursorBlock, true, "default", 2},
	} {
		s := CursorStyleOf(c.n)
		if s != c.style || s.Shape() != c.shape || s.Blink() != c.blink || s.String() != c.said || s.Steady() != c.steady {
			t.Errorf("DECSCUSR %d: style %d, shape %d, blink %v, %q, steady %d; want %d, %d, %v, %q, %d",
				c.n, s, s.Shape(), s.Blink(), s.String(), s.Steady(), c.style, c.shape, c.blink, c.said, c.steady)
		}
	}
}
