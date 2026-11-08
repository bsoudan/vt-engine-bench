package te

import "testing"

// From esctest2/esctest/tests/apc.py::test_APC_Basic
func TestEsctestApcTestAPCBasic(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestWriteAPC(t, stream, "xyz")
	esctestWrite(t, stream, "A")
	esctestAssertScreenCharsInRectEqual(t, screen, esctestRect{Left: 1, Top: 1, Right: 3, Bottom: 1}, []string{"A" + esctestEmpty() + esctestEmpty()})
}
