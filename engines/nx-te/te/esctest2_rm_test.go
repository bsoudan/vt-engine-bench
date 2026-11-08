package te

import "testing"

// From esctest2/esctest/tests/rm.py::test_RM_IRM
func TestEsctestRmTestRMIRM(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestSM(t, stream, esctestModeIRM)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, "X")
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, "W")
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestRM(t, stream, esctestModeIRM)
	esctestWrite(t, stream, "YZ")
	esctestAssertScreenCharsInRectEqual(t, screen, esctestRect{Left: 1, Top: 1, Right: 2, Bottom: 1}, []string{"YZ"})
}
