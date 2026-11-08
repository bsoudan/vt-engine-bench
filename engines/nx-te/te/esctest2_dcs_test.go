package te

import "testing"

// From esctest2/esctest/tests/dcs.py::test_DCS_Unrecognized
func TestEsctestDcsTestDCSUnrecognized(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestWriteDCS(t, stream, "z0")
	esctestAssertScreenCharsInRectEqual(t, screen, esctestRect{Left: 1, Top: 1, Right: 1, Bottom: 1}, []string{esctestEmpty()})
}
