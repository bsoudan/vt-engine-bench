package te

import (
	"encoding/json"
	"strings"
	"testing"

	"nx/cell"
)

// TestScreenStateRoundTrip creates a screen with complex state, marshals it,
// unmarshals into a fresh screen, and verifies all fields match.
func TestScreenStateRoundTrip(t *testing.T) {
	t.Parallel()
	s := NewScreen(80, 24)
	stream := NewStream(s)

	// Draw some content.
	stream.FeedBytes([]byte("Hello, world!"))

	// Set cursor position.
	stream.FeedBytes([]byte("\x1b[5;10H")) // row 5, col 10

	// Set colors: red foreground.
	stream.FeedBytes([]byte("\x1b[31mColored"))

	// Set title.
	stream.FeedBytes([]byte("\x1b]0;My Title\x1b\\"))

	// Set tab stop at current column, then move and set another.
	stream.FeedBytes([]byte("\x1bH"))      // set tab stop
	stream.FeedBytes([]byte("\x1b[1;20H")) // move to col 20
	stream.FeedBytes([]byte("\x1bH"))      // set another tab stop

	// Set margins.
	stream.FeedBytes([]byte("\x1b[5;20r")) // top=5, bottom=20

	// Save cursor.
	stream.FeedBytes([]byte("\x1b7"))

	// Switch to alt buffer.
	stream.FeedBytes([]byte("\x1b[?1049h"))
	stream.FeedBytes([]byte("Alt screen content"))

	// Marshal.
	state := s.MarshalState()

	// Serialize to JSON and back to verify JSON round-trip.
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	t.Logf("JSON size: %d bytes", len(data))

	var state2 ScreenState
	if err := json.Unmarshal(data, &state2); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Restore into a fresh screen.
	s2 := &Screen{}
	s2.UnmarshalState(&state2)

	// Verify dimensions.
	if s2.Columns != s.Columns || s2.Lines != s.Lines {
		t.Errorf("dimensions: got %dx%d, want %dx%d", s2.Columns, s2.Lines, s.Columns, s.Lines)
	}

	// Verify cursor.
	if s2.Cursor.Row != s.Cursor.Row || s2.Cursor.Col != s.Cursor.Col {
		t.Errorf("cursor: got (%d,%d), want (%d,%d)", s2.Cursor.Row, s2.Cursor.Col, s.Cursor.Row, s.Cursor.Col)
	}

	// Verify buffer content matches.
	for row := range s.Lines {
		for col := range s.Columns {
			if col < len(s.Buffer[row]) && col < len(s2.Buffer[row]) {
				if s.Buffer[row][col].Data != s2.Buffer[row][col].Data {
					t.Errorf("buffer[%d][%d]: got %q, want %q",
						row, col, s2.Buffer[row][col].Data, s.Buffer[row][col].Data)
				}
			}
		}
	}

	// Verify alt buffer exists.
	if s.altActive != s2.altActive {
		t.Errorf("altActive: got %v, want %v", s2.altActive, s.altActive)
	}

	// Verify title.
	if s2.Title != s.Title {
		t.Errorf("title: got %q, want %q", s2.Title, s.Title)
	}

	// Verify modes match.
	for k := range s.Mode {
		if _, ok := s2.Mode[k]; !ok {
			t.Errorf("mode %d missing after unmarshal", k)
		}
	}

	// Verify margins.
	if s.Margins != nil && s2.Margins != nil {
		if s.Margins.Top != s2.Margins.Top || s.Margins.Bottom != s2.Margins.Bottom {
			t.Errorf("margins: got %+v, want %+v", s2.Margins, s.Margins)
		}
	} else if (s.Margins == nil) != (s2.Margins == nil) {
		t.Errorf("margins nil mismatch: got %v, want %v", s2.Margins, s.Margins)
	}

	// Verify tab stops.
	for k := range s.TabStops {
		if _, ok := s2.TabStops[k]; !ok {
			t.Errorf("tab stop %d missing after unmarshal", k)
		}
	}

	// Verify savepoints.
	if len(s2.Savepoints) != len(s.Savepoints) {
		t.Errorf("savepoints: got %d, want %d", len(s2.Savepoints), len(s.Savepoints))
	}
}

// TestScrollbackStateRoundTrip tests serialization of ScrollbackScreen including scrollback.
func TestScrollbackStateRoundTrip(t *testing.T) {
	t.Parallel()
	h := NewScrollbackScreen(80, 5, 100) // small screen to force scrollback
	stream := NewStream(h)

	// Fill screen and generate scrollback by writing many lines.
	for i := range 20 {
		stream.FeedBytes([]byte("Line " + string(rune('A'+i)) + "\r\n"))
	}

	scrollback := h.ScrollbackRows()
	if len(scrollback) == 0 {
		t.Fatal("expected scrollback history after 20 lines on 5-row screen")
	}
	t.Logf("scrollback lines: %d", len(scrollback))

	// Marshal.
	state := h.MarshalState()

	// JSON round-trip.
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	t.Logf("JSON size: %d bytes", len(data))

	var state2 ScrollbackState
	if err := json.Unmarshal(data, &state2); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Restore.
	h2 := NewScrollbackScreen(1, 1, 100)
	h2.UnmarshalState(&state2)

	// The whole of it, not only the text: attributes, the numbering of the
	// history, every field a snapshot carries.
	again, err := json.Marshal(h2.MarshalState())
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(data) {
		t.Errorf("restored and taken again, the state differs:\nwas %s\nnow %s", data, again)
	}

	// Verify dimensions.
	if h2.Columns != h.Columns || h2.Lines != h.Lines {
		t.Errorf("dimensions: got %dx%d, want %dx%d", h2.Columns, h2.Lines, h.Columns, h.Lines)
	}

	// Verify scrollback preserved.
	scrollback2 := h2.ScrollbackRows()
	if len(scrollback2) != len(scrollback) {
		t.Errorf("scrollback: got %d lines, want %d", len(scrollback2), len(scrollback))
	}

	// Verify scrollback content matches.
	for i := range min(len(scrollback), len(scrollback2)) {
		orig := cell.Text(scrollback[i])
		restored := cell.Text(scrollback2[i])
		if orig != restored {
			t.Errorf("scrollback[%d]: got %q, want %q", i, restored, orig)
		}
	}

	// Verify visible buffer matches.
	for row := range h.Lines {
		orig := cell.Text(h.Buffer[row])
		restored := cell.Text(h2.Buffer[row])
		if orig != restored {
			t.Errorf("buffer[%d]: got %q, want %q", row, restored, orig)
		}
	}
}

// A screen restored from a snapshot must wrap lines like a fresh one. The
// snapshot omits an empty wrap map and the copy of nothing was nil, so the
// first line to reach the last column after a restore panicked in the autowrap
// path — recovered by the Stream, which then dropped the rest of the chunk. A
// View restores a snapshot on every bind, so any full-screen program that pads
// lines to the width (top) lost most of every frame.
func TestRestoredScreenWrapsAfterRoundTrip(t *testing.T) {
	t.Parallel()
	src := NewScrollbackScreen(20, 4, 100)
	var st ScrollbackState
	b, _ := json.Marshal(src.MarshalState())
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	dst := NewScrollbackScreen(20, 4, 100)
	dst.WriteProcessInput = func(string) {}
	dst.UnmarshalState(&st)

	// A line that overflows the width (the wrap is deferred to the character
	// that would not fit), then more: with no wrap map, the rest was lost.
	if err := NewStream(dst).Feed(strings.Repeat("x", 25) + "\r\nAFTER"); err != nil {
		t.Fatalf("feed after restore: %v", err)
	}
	if got := dst.Display()[2]; !strings.HasPrefix(got, "AFTER") {
		t.Fatalf("row 2 = %q, want AFTER: output after a full-width line was lost", got)
	}

	// The alternate screen too: a restored alt buffer must wrap as well.
	src2 := NewScrollbackScreen(20, 4, 100)
	src2.WriteProcessInput = func(string) {}
	_ = NewStream(src2).Feed("\x1b[?1049h")
	b, _ = json.Marshal(src2.MarshalState())
	var st2 ScrollbackState
	_ = json.Unmarshal(b, &st2)
	dst2 := NewScrollbackScreen(20, 4, 100)
	dst2.WriteProcessInput = func(string) {}
	dst2.UnmarshalState(&st2)
	if err := NewStream(dst2).Feed(strings.Repeat("y", 25) + "\r\nALT"); err != nil {
		t.Fatalf("feed on a restored alt screen: %v", err)
	}
	if got := dst2.Display()[2]; !strings.HasPrefix(got, "ALT") {
		t.Fatalf("alt row 2 = %q, want ALT", got)
	}
}
