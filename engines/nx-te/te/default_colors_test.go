package te

import (
	"strings"
	"testing"

	"nx/cell"
)

// A program asking the default foreground or background (OSC 10, 11) is told
// the colors of the terminal the screen shadows, as the console hands them
// down; what the program set itself wins until it resets it; and a screen
// told nothing says nothing, as a terminal that doesn't know would, rather
// than answer black.
func TestDefaultColorsAnswerTheQuery(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	var replies []string
	screen.WriteProcessInput = func(s string) { replies = append(replies, s) }
	stream := NewStream(screen)
	ask := func(t *testing.T, want ...string) {
		t.Helper()
		replies = nil
		esctestWrite(t, stream, "\x1b]10;?\x07\x1b]11;?\x07")
		if len(replies) != len(want) {
			t.Fatalf("replies %q, want %q", replies, want)
		}
		for i := range want {
			if replies[i] != want[i] {
				t.Fatalf("replies %q, want %q", replies, want)
			}
		}
	}

	ask(t)

	screen.SetColors(cell.Colors{Fg: "rgb:ffff/ffff/ffff", Bg: "rgb:1e/1e/1e"})
	ask(t, "\x1b]10;rgb:ffff/ffff/ffff\x1b\\", "\x1b]11;rgb:1e1e/1e1e/1e1e\x1b\\")

	esctestWrite(t, stream, "\x1b]11;rgb:8080/0000/0000\x07")
	ask(t, "\x1b]10;rgb:ffff/ffff/ffff\x1b\\", "\x1b]11;rgb:8080/0000/0000\x1b\\")

	esctestWrite(t, stream, "\x1b]111\x07")
	ask(t, "\x1b]10;rgb:ffff/ffff/ffff\x1b\\", "\x1b]11;rgb:1e1e/1e1e/1e1e\x1b\\")
}

// A program asking whether the colors are dark or light (CSI ? 996 n) is
// told by the background; one that asked to hear of a change (mode 2031) is
// told when the colors change, and only then. Colors not known go unreported.
func TestColorSchemeReports(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	var replies []string
	screen.WriteProcessInput = func(s string) { replies = append(replies, s) }
	stream := NewStream(screen)
	expect := func(t *testing.T, want ...string) {
		t.Helper()
		if strings.Join(replies, "") != strings.Join(want, "") {
			t.Fatalf("replies %q, want %q", replies, want)
		}
		replies = nil
	}

	esctestWrite(t, stream, "\x1b[?996n")
	expect(t)

	dark := cell.Colors{Fg: "rgb:ffff/ffff/ffff", Bg: "rgb:1e1e/1e1e/1e1e"}
	light := cell.Colors{Fg: "rgb:0000/0000/0000", Bg: "#fafafa"}
	screen.SetColors(dark) // no one asked to be told
	expect(t)
	esctestWrite(t, stream, "\x1b[?996n")
	expect(t, "\x1b[?997;1n")

	esctestWrite(t, stream, "\x1b[?2031h")
	screen.SetColors(light)
	expect(t, "\x1b[?997;2n")
	screen.SetColors(light) // the same again is no change
	expect(t)

	esctestWrite(t, stream, "\x1b[?2031$p")
	expect(t, "\x1b[?2031;1$y")
	esctestWrite(t, stream, "\x1b[?2031l")
	screen.SetColors(dark)
	expect(t)
}
