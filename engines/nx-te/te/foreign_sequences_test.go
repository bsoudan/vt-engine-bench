package te

import (
	"fmt"
	"strings"
	"testing"
)

// TestForeignSequencesLeaveScreenAlone feeds sequences te doesn't implement
// but real programs send, each shaped like one it does (a final it knows,
// behind a prefix or an intermediate, or an escape it half-knows), and
// checks each is consumed whole: the screen, the cursor and its attributes
// as they were, nothing drawn.
func TestForeignSequencesLeaveScreenAlone(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, seq string }{
		// A private query on a final that scrolls.
		{"XTSMGRAPHICS color registers", "\x1b[?1;1;0S"},
		{"XTSMGRAPHICS sixel geometry", "\x1b[?2;1;0S"},
		{"kitty keyboard query", "\x1b[?u"},
		// Intermediates on finals that restore, move and insert.
		{"DECSMBV", "\x1b[1 u"},
		{"DECRQTSR", "\x1b[1$u"},
		{"SR", "\x1b[1 A"},
		{"SL", "\x1b[1 @"},
		{"DECRARA", "\x1b[1;1;1;1;7$t"},
		// Designations te doesn't keep, whose final would be drawn.
		{"G2 designation", "\x1b*0"},
		{"G3 designation", "\x1b+B"},
		{"96-character G1", "\x1b-A"},
		{"multibyte designation", "\x1b$B"},
		{"multibyte G0 designation", "\x1b$(C"},
		// A second ESC begins the sequence the first abandoned.
		{"ESC ESC CSI", "\x1b\x1b[1m\x1b[m"},
		{"tmux passthrough", "\x1bPtmux;\x1b\x1b]0;x\x07\x1b\\"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			screen := NewScreen(20, 5)
			stream := NewStream(screen)
			esctestWrite(t, stream, "aaaa\r\nbbbb\r\ncccc\r\ndddd\x1b[3;3H")
			before := screen.Display()
			attr := screen.Cursor.Attr

			esctestWrite(t, stream, tc.seq+"X")

			want := append([]string(nil), before...)
			want[2] = "ccXc" + want[2][4:]
			if got := screen.Display(); strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("screen:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			if screen.Cursor.Attr != attr {
				t.Errorf("cursor attributes changed: %+v", screen.Cursor.Attr)
			}
		})
	}
}

// TestNonUTF8BytesDrawnAsReplacement checks that a byte that isn't UTF-8 is
// drawn as U+FFFD rather than read as an 8-bit C1 control: a stray 0x90 (DCS)
// in a Latin-1 file swallowed what followed, and 0x9b (CSI) ran its text as
// a sequence.
func TestNonUTF8BytesDrawnAsReplacement(t *testing.T) {
	t.Parallel()
	screen := NewScreen(20, 5)
	stream := NewStream(screen)

	esctestWrite(t, stream, "a\x90b\x9b1mc")

	if got := strings.TrimRight(screen.Display()[0], " "); got != "a�b�1mc" {
		t.Errorf("row 0 = %q", got)
	}
	if screen.Cursor.Attr.Bold {
		t.Errorf("0x9b ran as CSI: cursor is bold")
	}
}

// TestDECRQMUnknownExtensionMode checks that a private mode of 1000 and up
// that te has no part in is reported unrecognized (0), even once set, while
// those it keeps are reported set or reset. A program asking after grapheme
// clustering must not be told te has it; one asking after synchronized
// output is, since the view holds a frame while it is set.
func TestDECRQMUnknownExtensionMode(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	var reply string
	screen.WriteProcessInput = func(s string) { reply = s }
	stream := NewStream(screen)

	for _, tc := range []struct {
		seq  string
		want string
	}{
		{"\x1b[?2026$p", "\x1b[?2026;2$y"},
		{"\x1b[?2026h\x1b[?2026$p", "\x1b[?2026;1$y"},
		{"\x1b[?2026l\x1b[?2026$p", "\x1b[?2026;2$y"},
		{"\x1b[?2027h\x1b[?2027$p", "\x1b[?2027;0$y"},
		{"\x1b[?2004$p", "\x1b[?2004;2$y"},
		{"\x1b[?2004h\x1b[?2004$p", "\x1b[?2004;1$y"},
		{"\x1b[?1049$p", "\x1b[?1049;2$y"},
	} {
		reply = ""
		esctestWrite(t, stream, tc.seq)
		if reply != tc.want {
			t.Errorf("%q: reply %s, want %s", tc.seq, fmt.Sprintf("%q", reply), fmt.Sprintf("%q", tc.want))
		}
	}
}
