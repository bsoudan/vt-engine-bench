package te

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// A step is what a vtapp's screen is put through: bytes, or a resize.
type step struct {
	bytes      string
	rows, cols int // a resize, when not zero
}

// replica is a vtapp's screen as the vtapp and its view each run one: with
// history, reflowing on a resize, the replies to the program swallowed.
func replica() *ScrollbackScreen {
	s := NewScrollbackScreen(20, 6, 10)
	s.Reflow = true
	s.WriteProcessInput = func(string) {}
	return s
}

// apply puts s, through its own stream, through steps.
func apply(s *ScrollbackScreen, st *Stream, steps []step) {
	for _, p := range steps {
		if p.rows != 0 {
			s.Resize(p.rows, p.cols)
			continue
		}
		st.FeedBytes([]byte(p.bytes))
	}
}

// stateOf is s's whole state, as a snapshot carries it.
func stateOf(t *testing.T, s *ScrollbackScreen) []byte {
	t.Helper()
	b, err := json.Marshal(s.MarshalState())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// lines is a run of numbered lines, enough to scroll.
func lines(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteString("\r\n")
	}
	return b.String()
}

// A view's shadow is restored from the vtapp's snapshot, then fed the same
// bytes and resizes the vtapp's screen is: from then on the two must be the
// same screen, cell for cell, history and all — what the view shows is the
// vtapp's. Each case is a snapshot taken at a point that has state of its own
// to carry; nothing but the screen restored (no stream) is the shadow's.
func TestARestoredScreenReplaysInStep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		before []step
		after  []step
	}{
		{"plain", []step{{bytes: "hello\r\n"}}, []step{{bytes: "world\r\n"}}},
		{"a wrapped line", []step{{bytes: strings.Repeat("w", 33)}}, []step{{bytes: " and on\r\n"}}},
		{"wide characters", []step{{bytes: "表示 😀 x"}}, []step{{bytes: "\r表\r\n"}}},
		{"colors and attributes", []step{{bytes: "\x1b[1;31;44mbold red\x1b[4m"}}, []step{{bytes: "still\x1b[0m plain\r\n"}}},
		{"the alt screen", []step{{bytes: "main\r\n\x1b[?1049halt"}}, []step{{bytes: " more\x1b[?1049l back\r\n"}}},
		{"scroll margins", []step{{bytes: "\x1b[2;4r\x1b[4;1H"}}, []step{{bytes: lines(5)}}},
		{"saved cursor", []step{{bytes: "\x1b[3;5H\x1b7\x1b[1;1H"}}, []step{{bytes: "\x1b8here"}}},
		{"history filling", []step{{bytes: lines(8)}}, []step{{bytes: lines(4)}}},
		{"history evicting", []step{{bytes: lines(30)}}, []step{{bytes: lines(9)}}},
		{"resized after", []step{{bytes: lines(12) + strings.Repeat("r", 30)}}, []step{{rows: 4, cols: 13}, {bytes: "narrow\r\n"}, {rows: 8, cols: 31}}},
		// A pty read ends where it likes: a snapshot sent then is taken in
		// the middle of a sequence, whose rest comes after it.
		{"mid sequence", []step{{bytes: "a\x1b[3"}}, []step{{bytes: "1mred\x1b[0m\r\n"}}},
		{"mid character", []step{{bytes: "a\xe8\xa1"}}, []step{{bytes: "\xa8b\r\n"}}},
		{"mid title", []step{{bytes: "\x1b]2;half a ti"}}, []step{{bytes: "tle\x07after\r\n"}}},
		{"resized before", []step{{bytes: lines(12)}, {rows: 5, cols: 11}, {bytes: strings.Repeat("q", 25)}}, []step{{bytes: lines(3)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vtapp := replica()
			vst := NewStream(vtapp)
			apply(vtapp, vst, tc.before)

			// The snapshot, as the vtapp sends it: the screen, and where
			// its stream is.
			taken := vtapp.MarshalState()
			taken.Stream = vst.MarshalState()
			b, err := json.Marshal(taken)
			if err != nil {
				t.Fatal(err)
			}
			var snap ScrollbackState
			if err := json.Unmarshal(b, &snap); err != nil {
				t.Fatal(err)
			}
			view := replica()
			view.UnmarshalState(&snap)
			viewStream := NewStream(view)
			viewStream.UnmarshalState(snap.Stream)
			if !bytes.Equal(stateOf(t, view), stateOf(t, vtapp)) {
				t.Fatalf("restored, the view is not the vtapp's screen:\nvtapp %s\nview  %s", stateOf(t, vtapp), stateOf(t, view))
			}

			apply(vtapp, vst, tc.after)
			apply(view, viewStream, tc.after)
			if a, b := stateOf(t, vtapp), stateOf(t, view); !bytes.Equal(a, b) {
				t.Fatalf("fed the same, the view left the vtapp's screen:\nvtapp %s\nview  %s", a, b)
			}
		})
	}
}

// The same, for any output split anywhere, with a resize after: what a
// vtapp's program wrote, snapshotted wherever a read ended, then the rest.
func FuzzShadowSync(f *testing.F) {
	f.Add([]byte("a\x1b[31mred\x1b[0m\r\nline\r\n"), uint16(4), uint8(12))
	f.Add([]byte("\x1b[?1049h表示\x1b]2;title\x07\x1b[2;3r"+lines(9)), uint16(17), uint8(9))
	f.Add([]byte("\xe8\xa1\xa8\x1b7\x1b[5;5H\x1b8x"), uint16(1), uint8(30))
	f.Fuzz(func(t *testing.T, out []byte, at uint16, cols uint8) {
		cut := int(at) % (len(out) + 1)
		vtapp := replica()
		vst := NewStream(vtapp)
		vst.FeedBytes(out[:cut])

		taken := vtapp.MarshalState()
		taken.Stream = vst.MarshalState()
		b, err := json.Marshal(taken)
		if err != nil {
			t.Fatal(err)
		}
		var snap ScrollbackState
		if err := json.Unmarshal(b, &snap); err != nil {
			t.Fatal(err)
		}
		view := replica()
		view.UnmarshalState(&snap)
		viewStream := NewStream(view)
		viewStream.UnmarshalState(snap.Stream)

		rest := []step{{bytes: string(out[cut:])}, {rows: 7, cols: 2 + int(cols)%60}, {bytes: "end\r\n"}}
		apply(vtapp, vst, rest)
		apply(view, viewStream, rest)
		if a, b := stateOf(t, vtapp), stateOf(t, view); !bytes.Equal(a, b) {
			t.Fatalf("split at %d of %q, the view left the vtapp's screen:\nvtapp %s\nview  %s", cut, out, a, b)
		}
	})
}
