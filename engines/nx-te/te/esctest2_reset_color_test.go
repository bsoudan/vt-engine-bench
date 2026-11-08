package te

import (
	"testing"

	"nx/cell"
)

// xtermANSI is the 16 ANSI colors as a terminal answering OSC 4 has them:
// xterm knows its own; te is told the terminal it shadows (SetColors).
func xtermANSI() cell.Colors {
	return cell.Colors{ANSI: [16]string{
		"rgb:0000/0000/0000", "rgb:cdcd/0000/0000", "rgb:0000/cdcd/0000", "rgb:cdcd/cdcd/0000",
		"rgb:0000/0000/eeee", "rgb:cdcd/0000/cdcd", "rgb:0000/cdcd/cdcd", "rgb:e5e5/e5e5/e5e5",
		"rgb:7f7f/7f7f/7f7f", "rgb:ffff/0000/0000", "rgb:0000/ffff/0000", "rgb:ffff/ffff/0000",
		"rgb:5c5c/5c5c/ffff", "rgb:ffff/0000/ffff", "rgb:0000/ffff/ffff", "rgb:ffff/ffff/ffff",
	}}
}

// From esctest2/esctest/tests/reset_color.py::test_ResetColor_Standard
func TestEsctestResetColorTestResetColorStandard(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	screen.SetColors(xtermANSI())
	stream := NewStream(screen)
	response := esctestCaptureResponse(screen, func() {
		esctestChangeColor(t, stream, "0", "?")
	})
	original := esctestReadOSC(t, response, "4")
	esctestChangeColor(t, stream, "0", "#aaaabbbbcccc")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeColor(t, stream, "0", "?")
	})
	if got := esctestReadOSC(t, response, "4"); got != ";0;rgb:aaaa/bbbb/cccc" {
		t.Fatalf("unexpected color %q", got)
	}
	esctestResetColor(t, stream, "0")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeColor(t, stream, "0", "?")
	})
	if got := esctestReadOSC(t, response, "4"); got != original {
		t.Fatalf("expected %q, got %q", original, got)
	}
}

// From esctest2/esctest/tests/reset_color.py::test_ResetColor_All
func TestEsctestResetColorTestResetColorAll(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	screen.SetColors(xtermANSI())
	stream := NewStream(screen)
	response := esctestCaptureResponse(screen, func() {
		esctestChangeColor(t, stream, "3", "?")
	})
	original := esctestReadOSC(t, response, "4")
	esctestChangeColor(t, stream, "3", "#aabbcc")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeColor(t, stream, "3", "?")
	})
	if got := esctestReadOSC(t, response, "4"); got != ";3;rgb:aaaa/bbbb/cccc" {
		t.Fatalf("unexpected color %q", got)
	}
	esctestResetColor(t, stream)
	response = esctestCaptureResponse(screen, func() {
		esctestChangeColor(t, stream, "3", "?")
	})
	if got := esctestReadOSC(t, response, "4"); got != original {
		t.Fatalf("expected %q, got %q", original, got)
	}
}

// A shadow screen tells a program asking for an ANSI color the color of the
// terminal it shadows, and asking for one nobody knows — the terminal never
// said, or it is past the 16 and the program never set it — it is told
// nothing, as a terminal that doesn't know says nothing: never black.
func TestQueryColorAnswersWhatIsKnownAlone(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	ask := func(index string) string {
		return esctestCaptureResponse(screen, func() { esctestChangeColor(t, stream, index, "?") })
	}
	if got := ask("1"); got != "" {
		t.Fatalf("ANSI 1, the terminal's not known, answered %q", got)
	}
	colors := cell.Colors{}
	colors.ANSI[1] = "rgb:c0/1c/28"
	screen.SetColors(colors)
	if got := esctestReadOSC(t, ask("1"), "4"); got != ";1;rgb:c0c0/1c1c/2828" {
		t.Fatalf("ANSI 1 answered %q, want the terminal's", got)
	}
	if got := ask("2"); got != "" {
		t.Fatalf("ANSI 2, the terminal's not said, answered %q", got)
	}
	if got := ask("100"); got != "" {
		t.Fatalf("color 100, never set, answered %q", got)
	}
}
