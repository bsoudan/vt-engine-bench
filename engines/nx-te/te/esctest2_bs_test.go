package te

import (
	"strings"
	"testing"
)

// From esctest2/esctest/tests/bs.py::test_BS_Basic
func TestEsctestBsTestBSBasic(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestCUP(t, stream, esctestPoint{X: 3, Y: 3})
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 2, Y: 3})
}

// From esctest2/esctest/tests/bs.py::test_BS_NoWrapByDefault
func TestEsctestBsTestBSNoWrapByDefault(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 3})
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 1, Y: 3})
}

// From esctest2/esctest/tests/bs.py::test_BS_WrapsInWraparoundMode
func TestEsctestBsTestBSWrapsInWraparoundMode(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestReverseWraparoundMode())
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 3})
	esctestWrite(t, stream, "\x08")
	size := esctestGetScreenSize(screen)
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: size.Width, Y: 2})
}

// From esctest2/esctest/tests/bs.py::test_BS_InitialReverseWraparound
func TestEsctestBsTestBSInitialReverseWraparound(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestModeReverseWrapInline)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, ControlESC+"E")
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 1, Y: 2})
}

// From esctest2/esctest/tests/bs.py::test_BS_ReverseWrapRequiresDECAWM
func TestEsctestBsTestBSReverseWrapRequiresDECAWM(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECRESET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestReverseWraparoundMode())
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 3})
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 1, Y: 3})

	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECRESET(t, stream, esctestReverseWraparoundMode())
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 3})
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 1, Y: 3})
}

// From esctest2/esctest/tests/bs.py::test_BS_ReverseWrapWithLeftRight
func TestEsctestBsTestBSReverseWrapWithLeftRight(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestReverseWraparoundMode())
	esctestDECSET(t, stream, esctestModeDECLRMM)
	esctestDECSLRM(t, stream, 5, 10)
	esctestCUP(t, stream, esctestPoint{X: 5, Y: 3})
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 10, Y: 2})
}

// From esctest2/esctest/tests/bs.py::test_BS_ReversewrapFromLeftEdgeToRightMargin
func TestEsctestBsTestBSReversewrapFromLeftEdgeToRightMargin(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestReverseWraparoundMode())
	esctestDECSET(t, stream, esctestModeDECLRMM)
	esctestDECSLRM(t, stream, 5, 10)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 3})
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 10, Y: 2})
}

// From esctest2/esctest/tests/bs.py::test_BS_ReverseWrapGoesToBottom
func TestEsctestBsTestBSReverseWrapGoesToBottom(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestReverseWraparoundMode())
	esctestDECSTBM(t, stream, 2, 5)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 2})
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 80, Y: 5})
}

// From esctest2/esctest/tests/bs.py::test_BS_StopsAtLeftMargin
func TestEsctestBsTestBSStopsAtLeftMargin(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECLRMM)
	esctestDECSLRM(t, stream, 5, 10)
	esctestCUP(t, stream, esctestPoint{X: 5, Y: 1})
	esctestWrite(t, stream, "\x08")
	esctestDECRESET(t, stream, esctestModeDECLRMM)
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 5, Y: 1})
}

// From esctest2/esctest/tests/bs.py::test_BS_MovesLeftWhenLeftOfLeftMargin
func TestEsctestBsTestBSMovesLeftWhenLeftOfLeftMargin(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECLRMM)
	esctestDECSLRM(t, stream, 5, 10)
	esctestCUP(t, stream, esctestPoint{X: 4, Y: 1})
	esctestWrite(t, stream, "\x08")
	esctestDECRESET(t, stream, esctestModeDECLRMM)
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 3, Y: 1})
}

// From esctest2/esctest/tests/bs.py::test_BS_StopsAtOrigin
func TestEsctestBsTestBSStopsAtOrigin(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 1})
	esctestWrite(t, stream, "\x08")
	esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 1, Y: 1})
}

// From esctest2/esctest/tests/bs.py::test_BS_CursorStartsInDoWrapPosition
func TestEsctestBsTestBSCursorStartsInDoWrapPosition(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	size := esctestGetScreenSize(screen)
	esctestCUP(t, stream, esctestPoint{X: size.Width - 1, Y: 1})
	esctestWrite(t, stream, "ab")
	esctestWrite(t, stream, "\x08")
	esctestWrite(t, stream, "X")
	esctestAssertScreenCharsInRectEqual(t, screen, esctestRect{Left: size.Width - 1, Top: 1, Right: size.Width, Bottom: 1}, []string{"Xb"})
}

// From esctest2/esctest/tests/bs.py::test_BS_ReverseWrapStartingInDoWrapPosition
func TestEsctestBsTestBSReverseWrapStartingInDoWrapPosition(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestReverseWraparoundMode())
	size := esctestGetScreenSize(screen)
	esctestCUP(t, stream, esctestPoint{X: size.Width - 1, Y: 1})
	esctestWrite(t, stream, "ab")
	esctestWrite(t, stream, "\x08")
	esctestWrite(t, stream, "X")
	esctestAssertScreenCharsInRectEqual(t, screen, esctestRect{Left: size.Width - 1, Top: 1, Right: size.Width, Bottom: 1}, []string{"aX"})
}

// From esctest2/esctest/tests/bs.py::test_BS_AfterNoWrappedInlines
func TestEsctestBsTestBSAfterNoWrappedInlines(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestModeReverseWrapInline)
	size := esctestGetScreenSize(screen)
	fill := strings.Repeat("*", size.Width-2) + "\n"
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 3})
	esctestWrite(t, stream, fill)
	esctestWrite(t, stream, fill)
	esctestCUP(t, stream, esctestPoint{X: 5, Y: 5})
	esctestWrite(t, stream, strings.Repeat("\x08", size.Width*2))
	if esctestXtermReverseWrap >= 383 {
		esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 1, Y: 4})
	} else {
		esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 5, Y: 3})
	}
}

// From esctest2/esctest/tests/bs.py::test_BS_AfterOneWrappedInline
func TestEsctestBsTestBSAfterOneWrappedInline(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECAWM)
	esctestDECSET(t, stream, esctestModeReverseWrapInline)
	size := esctestGetScreenSize(screen)
	fill := strings.Repeat("*", (size.Width+2)*2)
	esctestCUP(t, stream, esctestPoint{X: 1, Y: 3})
	esctestWrite(t, stream, fill+"\n"+fill)
	esctestWrite(t, stream, strings.Repeat("\x08", size.Width*5))
	if esctestXtermReverseWrap >= 383 {
		esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 1, Y: 6})
	} else {
		esctestAssertEQ(t, esctestGetCursorPosition(screen), esctestPoint{X: 9, Y: 3})
	}
}
