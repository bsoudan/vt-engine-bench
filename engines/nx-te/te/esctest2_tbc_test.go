package te

import "testing"

// From esctest2/esctest/tests/tbc.py::test_TBC_Default
func TestEsctestTbcTestTBCDefault(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestWrite(t, stream, "\t")
	esctestAssertEQ(t, esctestGetCursorPosition(screen).X, 9)
	esctestTBC(t, stream)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, "\t")
	esctestAssertEQ(t, esctestGetCursorPosition(screen).X, 17)
}

// From esctest2/esctest/tests/tbc.py::test_TBC_0
func TestEsctestTbcTestTBC0(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestWrite(t, stream, "\t")
	esctestAssertEQ(t, esctestGetCursorPosition(screen).X, 9)
	esctestTBC(t, stream, 0)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, "\t")
	esctestAssertEQ(t, esctestGetCursorPosition(screen).X, 17)
}

// From esctest2/esctest/tests/tbc.py::test_TBC_3
func TestEsctestTbcTestTBC3(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestTBC(t, stream, 3)
	esctestCUP(t, stream, esctestPoint{X: 30, Y: 1})
	esctestHTS(t, stream)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, "\t")
	esctestAssertEQ(t, esctestGetCursorPosition(screen).X, 30)
}

// From esctest2/esctest/tests/tbc.py::test_TBC_NoOp
func TestEsctestTbcTestTBCNoOp(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestCUP(t, stream, esctestPoint{X: 10, Y: 1})
	esctestTBC(t, stream, 0)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, "\t")
	esctestAssertEQ(t, esctestGetCursorPosition(screen).X, 9)
	esctestWrite(t, stream, "\t")
	esctestAssertEQ(t, esctestGetCursorPosition(screen).X, 17)
}
