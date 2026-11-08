package te

import (
	"strings"
	"testing"
)

func esctestReadDCSResponse(response string) string {
	payload := strings.TrimPrefix(response, ControlDCS)
	payload = strings.TrimSuffix(payload, ControlST)
	return payload
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECELF
func TestEsctestDecrqssTestDecrqssDecelf(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECELF(t, stream, 0)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "+q")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r0+q")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECLFKC
func TestEsctestDecrqssTestDecrqssDeclfkc(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECLFKC(t, stream, 0)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "*}")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r0*}")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSASD
func TestEsctestDecrqssTestDecrqssDecsasd(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSASD(t, stream, 0)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "$}")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r0$}")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSACE
func TestEsctestDecrqssTestDecrqssDecsace(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSACE(t, stream, 0)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "*x")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r0*x")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSCA
func TestEsctestDecrqssTestDecrqssDecsca(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSCA(t, stream, 1)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "\"q")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r1\"q")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSCL
func TestEsctestDecrqssTestDecrqssDecscl(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	sevenBit := 1
	esctestDECSCL(t, stream, 65, &sevenBit)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "\"p")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r65;1\"p")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSTBM
func TestEsctestDecrqssTestDecrqssDecstbm(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSTBM(t, stream, 5, 6)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "r")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r5;6r")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_SGR
func TestEsctestDecrqssTestDecrqssSgr(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestSGR(t, stream, 1)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "m")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r0;1m")
}

// The colors set are in the answer, as xterm says them: the 16 by their
// own codes, the 256 and true color by colon. A program that set true
// color, and finds it here, knows the terminal kept it.
func TestDecrqssSgrColors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ set, want string }{
		{"31;104", "1$r0;31;104m"},
		{"38;5;200;48;5;17", "1$r0;38:5:200;48:5:17m"},
		{"48;2;1;2;3", "1$r0;48:2::1:2:3m"},
		{"1;38:2::255:0:128", "1$r0;1;38:2::255:0:128m"},
	} {
		screen := NewScreen(80, 24)
		stream := NewStream(screen)
		esctestWrite(t, stream, ControlCSI+c.set+"m")
		response := esctestCaptureResponse(screen, func() {
			esctestDECRQSS(t, stream, "m")
		})
		if got := esctestReadDCSResponse(response); got != c.want {
			t.Errorf("SGR %s: DECRQSS says %q; want %q", c.set, got, c.want)
		}
	}
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSCUSR
func TestEsctestDecrqssTestDecrqssDecscusr(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSCUSR(t, stream, 4)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, " q")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r4 q")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSLRM
func TestEsctestDecrqssTestDecrqssDecslrm(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSET(t, stream, esctestModeDECLRMM)
	esctestDECSLRM(t, stream, 3, 4)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "s")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r3;4s")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSLPP
func TestEsctestDecrqssTestDecrqssDecslpp(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestXtermWinops(t, stream, 27)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "t")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r27t")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSMKR
func TestEsctestDecrqssTestDecrqssDecsmkr(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSMKR(t, stream, 0)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "+r")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r0+r")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSNLS
func TestEsctestDecrqssTestDecrqssDecsnls(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSNLS(t, stream, 24)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "*|")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r24*|")
}

// From esctest2/esctest/tests/decrqss.py::test_DECRQSS_DECSSDT
func TestEsctestDecrqssTestDecrqssDecssdt(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	esctestDECSSDT(t, stream, 0)
	response := esctestCaptureResponse(screen, func() {
		esctestDECRQSS(t, stream, "$~")
	})
	esctestAssertEQ(t, esctestReadDCSResponse(response), "1$r0$~")
}
