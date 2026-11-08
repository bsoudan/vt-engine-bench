package te

import (
	"testing"

	"nx/cell"
)

// From esctest2/esctest/tests/reset_special_color.py::test_ResetSpecialColor_Single.
// A color past the 16 is not one te is told of the terminal it shadows: it
// is not known, and goes unanswered (QueryColor), until set and after reset.
func TestEsctestResetSpecialColorTestResetSpecialColorSingle(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	response := esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor(t, stream, "0", "?")
	})
	if response != "" {
		t.Fatalf("a color not known answered %q", response)
	}
	esctestChangeSpecialColor(t, stream, "0", "#aaaabbbbcccc")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor(t, stream, "0", "?")
	})
	if got := esctestReadOSC(t, response, "4"); got != ";16;rgb:aaaa/bbbb/cccc" {
		t.Fatalf("unexpected color %q", got)
	}
	esctestResetSpecialColor(t, stream, "0")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor(t, stream, "0", "?")
	})
	if response != "" {
		t.Fatalf("reset, it answered %q: not known again", response)
	}
}

// From esctest2/esctest/tests/reset_special_color.py::test_ResetSpecialColor_Single2
func TestEsctestResetSpecialColorTestResetSpecialColorSingle2(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	response := esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor2(t, stream, "0", "?")
	})
	original := esctestReadOSC(t, response, "5")
	esctestChangeSpecialColor2(t, stream, "0", "#aaaabbbbcccc")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor2(t, stream, "0", "?")
	})
	if got := esctestReadOSC(t, response, "5"); got != ";0;rgb:aaaa/bbbb/cccc" {
		t.Fatalf("unexpected color %q", got)
	}
	esctestResetSpecialColor(t, stream, "0")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor2(t, stream, "0", "?")
	})
	if got := esctestReadOSC(t, response, "5"); got != original {
		t.Fatalf("expected %q, got %q", original, got)
	}
}

// From esctest2/esctest/tests/reset_special_color.py::test_ResetSpecialColor_Multiple
func TestEsctestResetSpecialColorTestResetSpecialColorMultiple(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	responses := []string{}
	screen.WriteProcessInput = func(data string) { responses = append(responses, data) }
	esctestChangeSpecialColor(t, stream, "0", "?", "1", "?")
	if len(responses) != 0 {
		t.Fatalf("colors not known answered %q", responses) // as Single says
	}
	esctestChangeSpecialColor(t, stream, "0", "#aaaabbbbcccc")
	esctestChangeSpecialColor(t, stream, "1", "#ddddeeeeffff")
	responses = []string{}
	screen.WriteProcessInput = func(data string) { responses = append(responses, data) }
	esctestChangeSpecialColor(t, stream, "0", "?")
	esctestChangeSpecialColor(t, stream, "1", "?")
	if got := esctestReadOSC(t, responses[0], "4"); got != ";16;rgb:aaaa/bbbb/cccc" {
		t.Fatalf("unexpected color %q", got)
	}
	if got := esctestReadOSC(t, responses[1], "4"); got != ";17;rgb:dddd/eeee/ffff" {
		t.Fatalf("unexpected color %q", got)
	}
	esctestResetSpecialColor(t, stream, "0", "1")
	responses = []string{}
	screen.WriteProcessInput = func(data string) { responses = append(responses, data) }
	esctestChangeSpecialColor(t, stream, "0", "?", "1", "?")
	if len(responses) != 0 {
		t.Fatalf("reset, they answered %q: not known again", responses)
	}
}

// From esctest2/esctest/tests/reset_special_color.py::test_ResetSpecialColor_Multiple2
func TestEsctestResetSpecialColorTestResetSpecialColorMultiple2(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	responses := []string{}
	screen.WriteProcessInput = func(data string) { responses = append(responses, data) }
	esctestChangeSpecialColor2(t, stream, "0", "?", "1", "?")
	if len(responses) < 2 {
		t.Fatalf("expected responses")
	}
	original1 := esctestReadOSC(t, responses[0], "5")
	original2 := esctestReadOSC(t, responses[1], "5")
	esctestChangeSpecialColor2(t, stream, "0", "#aaaabbbbcccc")
	esctestChangeSpecialColor2(t, stream, "1", "#ddddeeeeffff")
	responses = []string{}
	screen.WriteProcessInput = func(data string) { responses = append(responses, data) }
	esctestChangeSpecialColor2(t, stream, "0", "?")
	esctestChangeSpecialColor2(t, stream, "1", "?")
	if got := esctestReadOSC(t, responses[0], "5"); got != ";0;rgb:aaaa/bbbb/cccc" {
		t.Fatalf("unexpected color %q", got)
	}
	if got := esctestReadOSC(t, responses[1], "5"); got != ";1;rgb:dddd/eeee/ffff" {
		t.Fatalf("unexpected color %q", got)
	}
	esctestResetSpecialColor(t, stream, "0", "1")
	responses = []string{}
	screen.WriteProcessInput = func(data string) { responses = append(responses, data) }
	esctestChangeSpecialColor2(t, stream, "0", "?", "1", "?")
	if esctestReadOSC(t, responses[0], "5") != original1 {
		t.Fatalf("expected %q", original1)
	}
	if esctestReadOSC(t, responses[1], "5") != original2 {
		t.Fatalf("expected %q", original2)
	}
}

// From esctest2/esctest/tests/reset_special_color.py::test_ResetSpecialColor_Dynamic
func TestEsctestResetSpecialColorTestResetSpecialColorDynamic(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	screen.SetColors(cell.Colors{Fg: "rgb:ffff/ffff/ffff"}) // xterm knows its own; te is told
	stream := NewStream(screen)
	response := esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor(t, stream, "10", "?")
	})
	original := esctestReadOSC(t, response, "10")
	esctestChangeSpecialColor(t, stream, "10", "#aaaabbbbcccc")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor(t, stream, "10", "?")
	})
	if got := esctestReadOSC(t, response, "10"); got != ";rgb:aaaa/bbbb/cccc" {
		t.Fatalf("unexpected color %q", got)
	}
	esctestResetDynamicColor(t, stream, "110")
	response = esctestCaptureResponse(screen, func() {
		esctestChangeSpecialColor(t, stream, "10", "?")
	})
	if got := esctestReadOSC(t, response, "10"); got != original {
		t.Fatalf("expected %q, got %q", original, got)
	}
}
