package te

import (
	"slices"
	"strings"
	"testing"
)

// SyncOutput is told where each synchronized frame begins and ends, as it
// is parsed: what the screen holds when a frame begins is the one before
// it, whole. Each begin is told, set already or not; an end only of a frame
// begun — a reset clearing the mode ends one too.
func TestSyncOutputIsToldWhereFramesBeginAndEnd(t *testing.T) {
	t.Parallel()
	screen := NewScreen(20, 3)
	stream := NewStream(screen)
	var told []string
	screen.SyncOutput = func(begun bool) {
		row := strings.TrimRight(screen.Display()[0], " ")
		if begun {
			told = append(told, "begun over ["+row+"]")
		} else {
			told = append(told, "ended at ["+row+"]")
		}
	}
	for _, seq := range []string{
		"\x1b[?2026l",                     // none begun: nothing ended
		"\x1b[?2026h\x1b[Hone\x1b[?2026l", // a frame
		"\x1b[?2026h\x1b[Htwo\x1b[?2026h", // begun again mid-frame
		"\x1b[?2026l",                     // ended
		"\x1b[?2026h\x1b[Hthree\x1bc",     // a reset ends it
	} {
		esctestWrite(t, stream, seq)
	}
	want := []string{
		"begun over []", "ended at [one]",
		"begun over [one]", "begun over [two]", "ended at [two]",
		"begun over [two]", "ended at []",
	}
	if !slices.Equal(told, want) {
		t.Fatalf("told %q\nwant %q", told, want)
	}
}
