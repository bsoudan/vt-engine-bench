package te

import "testing"

// From esctest2/esctest/tests/hts.py::test_HTS_Basic
func TestEsctestHtsTestHTSBasic(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestTBC(t, stream, 3)
	esctestCUP(t, stream, esctestPoint{X: 20, Y: 1})
	esctestHTS(t, stream)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, "\t")
	esctestAssertEQ(t, esctestGetCursorPosition(screen).X, 20)
}
