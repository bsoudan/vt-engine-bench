package te

import (
	"testing"
)

func TestResetBetweenAttributes(t *testing.T) {
	t.Parallel()
	screen := NewScreen(2, 2)
	screen.SelectGraphicRendition([]int{31, 0, 41}, false)
	if screen.Cursor.Attr.Fg != pyteColor("default") || screen.Cursor.Attr.Bg != pyteColor("red") {
		t.Fatalf("expected reset fg and red bg")
	}
}

// Resize must keep the inactive buffer in step with the new dimensions:
// the alt-screen swap installs it verbatim, so a stale-sized buffer under
// the new Lines/Columns makes Display index out of range (seen in
// production via regionActor.snapshot after a client resized a region
// whose program held the alt screen).
func TestResizeWhileAltScreenActive(t *testing.T) {
	t.Parallel()
	screen := NewScreen(4, 3)
	screen.Draw("foo")
	screen.SetMode([]int{ModeAltBufCursor}, false)
	screen.Resize(5, 6)

	screen.ResetMode([]int{ModeAltBufCursor}, false)
	if len(screen.Rows()) != screen.Lines {
		t.Fatalf("primary buffer rows = %d, want %d", len(screen.Rows()), screen.Lines)
	}
	for row := range screen.Rows() {
		if len(screen.Rows()[row]) != screen.Columns {
			t.Fatalf("row %d cols = %d, want %d", row, len(screen.Rows()[row]), screen.Columns)
		}
	}
	screen.Display()
}

// Mode 47 keeps the alt buffer across exit/re-entry, so a resize while on
// the primary screen must also resize the retained alt buffer.
func TestResizeWithInactiveAltBuffer(t *testing.T) {
	t.Parallel()
	screen := NewScreen(4, 3)
	screen.SetMode([]int{ModeAltBuf}, false)
	screen.ResetMode([]int{ModeAltBuf}, false)
	screen.Resize(5, 6)

	screen.SetMode([]int{ModeAltBuf}, false)
	if len(screen.Rows()) != screen.Lines {
		t.Fatalf("alt buffer rows = %d, want %d", len(screen.Rows()), screen.Lines)
	}
	for row := range screen.Rows() {
		if len(screen.Rows()[row]) != screen.Columns {
			t.Fatalf("row %d cols = %d, want %d", row, len(screen.Rows()[row]), screen.Columns)
		}
	}
	screen.Display()
}

// Shrinking while the alt screen is active must clamp the saved primary
// cursor; mode 1049 exit restores it verbatim and an out-of-bounds cursor
// panics the next Draw.
func TestResizeShrinkWhileAltScreenClampsSavedCursor(t *testing.T) {
	t.Parallel()
	screen := NewScreen(10, 10)
	screen.CursorPosition(10, 10)
	screen.SetMode([]int{ModeAltBufCursor}, false)
	screen.Resize(4, 5)

	screen.ResetMode([]int{ModeAltBufCursor}, false)
	if screen.Cursor.Row >= screen.Lines || screen.Cursor.Col >= screen.Columns {
		t.Fatalf("restored cursor (%d,%d) out of bounds for %dx%d",
			screen.Cursor.Row, screen.Cursor.Col, screen.Lines, screen.Columns)
	}
	screen.Draw("x")
}

func TestDrawAutowrapAndIRM(t *testing.T) {
	t.Parallel()
	screen := NewScreen(3, 3)
	screen.SetMode([]int{ModeLNM}, false)
	for _, ch := range "abc" {
		screen.Draw(string(ch))
	}
	if screen.Display()[0] != "abc" {
		t.Fatalf("expected abc")
	}
	screen.Draw("a")
	if screen.Cursor.Row != 1 || screen.Cursor.Col != 1 {
		t.Fatalf("expected wrapped cursor")
	}

	screen = NewScreen(3, 3)
	screen.ResetMode([]int{ModeDECAWM}, false)
	for _, ch := range "abc" {
		screen.Draw(string(ch))
	}
	screen.Draw("a")
	if screen.Display()[0] != "aba" {
		t.Fatalf("expected overwrite")
	}

	screen.SetMode([]int{ModeIRM}, false)
	screen.CursorPosition(0, 0)
	screen.Draw("x")
	if screen.Display()[0] != "xab" {
		t.Fatalf("expected insert mode")
	}
}

func TestDrawUTF8(t *testing.T) {
	t.Parallel()
	screen := NewScreen(1, 1)
	stream := NewStream(screen)
	if err := stream.FeedBytes([]byte("\xE2\x80\x9D")); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if screen.Display()[0] != "”" {
		t.Fatalf("expected utf8 char")
	}
}

func TestDrawWidthCombining(t *testing.T) {
	t.Parallel()
	screen := NewScreen(4, 2)
	screen.Draw("\u0308")
	screen.Draw("bad")
	screen.Draw("\u0308")
	if screen.Display()[0] != "bad̈ " {
		t.Fatalf("expected combining")
	}
	screen.Draw("!")
	screen.Draw("\u0308")
	if screen.Display()[0] != "bad̈!̈" {
		t.Fatalf("expected combining previous line")
	}
}

func TestCarriageReturnAndIndex(t *testing.T) {
	t.Parallel()
	screen := NewScreen(3, 3)
	screen.Cursor.Col = 2
	screen.CarriageReturn()
	if screen.Cursor.Col != 0 {
		t.Fatalf("expected carriage return")
	}

	screen = NewScreen(2, 2)
	screen.Draw("wo")
	screen.Cursor.Row = 1
	screen.Index()
	if screen.Cursor.Row != 1 {
		t.Fatalf("expected cursor row")
	}
}

func TestTabStops(t *testing.T) {
	t.Parallel()
	screen := NewScreen(10, 10)
	if _, ok := screen.TabStops[8]; !ok {
		t.Fatalf("expected default tabstop")
	}
	screen.ClearTabStop(3)
	screen.Cursor.Col = 1
	screen.SetTabStop()
	screen.Cursor.Col = 8
	screen.SetTabStop()
	screen.Cursor.Col = 0
	screen.Tab()
	if screen.Cursor.Col != 1 {
		t.Fatalf("expected tab to col1")
	}
}

func TestHideCursorMode(t *testing.T) {
	t.Parallel()
	screen := NewScreen(10, 10)
	if screen.Cursor.Hidden {
		t.Fatalf("expected cursor visible")
	}
	screen.ResetMode([]int{ModeDECTCEM}, false)
	if !screen.Cursor.Hidden {
		t.Fatalf("expected cursor hidden")
	}
	screen.SetMode([]int{ModeDECTCEM}, false)
	if screen.Cursor.Hidden {
		t.Fatalf("expected cursor visible")
	}
}
