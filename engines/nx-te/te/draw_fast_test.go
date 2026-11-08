package te

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// TestDrawASCIIMatchesGeneral checks the one claim the ASCII fast path makes:
// a screen drawn through it is the screen the general path draws, cell for
// cell, cursor and history included. Random text mixes printable ASCII with
// combining marks, wide glyphs, emoji and their modifiers, in every state
// the fast path decides on (autowrap, left and right margins, insert mode,
// the line-drawing charset, reverse video) and some it does not.
func TestDrawASCIIMatchesGeneral(t *testing.T) {
	pieces := []string{
		"a", "Z", " ", "~", "0", "hello", "The quick brown fox",
		"é", "e\u0301", "字", "漢字", "🙂", "\u0301", "αβγ", "👍🏽", "👨\u200d👩\u200d👧",
		"x\u0301y", "ab字cd", "a\u200db", "❤\ufe0f",
	}
	type step struct {
		name string
		do   func(s *Screen)
	}
	steps := []step{
		{"CR LF", func(s *Screen) { s.CarriageReturn(); s.LineFeed() }},
		{"CUP", func(s *Screen) { s.CursorPosition(2, 3) }},
		{"CUP end", func(s *Screen) { s.CursorPosition(5, 19) }},
		{"DECAWM off", func(s *Screen) { s.ResetMode([]int{7}, true) }},
		{"DECAWM on", func(s *Screen) { s.SetMode([]int{7}, true) }},
		{"IRM on", func(s *Screen) { s.SetMode([]int{4}, false) }},
		{"IRM off", func(s *Screen) { s.ResetMode([]int{4}, false) }},
		{"DECLRMM margins", func(s *Screen) { s.SetMode([]int{69}, true); s.SetLeftRightMargins(3, 12) }},
		{"DECLRMM off", func(s *Screen) { s.ResetMode([]int{69}, true) }},
		{"SO (G1)", func(s *Screen) { s.ShiftOut() }},
		{"SI (G0)", func(s *Screen) { s.ShiftIn() }},
		{"DECSCNM on", func(s *Screen) { s.SetMode([]int{5}, true) }},
		{"DECSCNM off", func(s *Screen) { s.ResetMode([]int{5}, true) }},
		{"SGR bold red", func(s *Screen) { s.SelectGraphicRendition([]int{1, 31}, false) }},
		{"SGR reset", func(s *Screen) { s.SelectGraphicRendition([]int{0}, false) }},
		{"EL", func(s *Screen) { s.EraseInLine(2, false) }},
	}

	r := rand.New(rand.NewPCG(7, 11))
	for iter := 0; iter < 3000; iter++ {
		var script []string
		var ops []func(s *Screen)
		for n := r.IntN(12) + 1; n > 0; n-- {
			if r.IntN(3) == 0 {
				st := steps[r.IntN(len(steps))]
				script = append(script, st.name)
				ops = append(ops, st.do)
				continue
			}
			var b strings.Builder
			for k := r.IntN(6) + 1; k > 0; k-- {
				b.WriteString(pieces[r.IntN(len(pieces))])
			}
			text := b.String()
			script = append(script, fmt.Sprintf("Draw(%q)", text))
			ops = append(ops, func(s *Screen) { s.Draw(text) })
		}

		run := func(fast bool) *ScrollbackScreen {
			defer func(was bool) { asciiFastPath = was }(asciiFastPath)
			asciiFastPath = fast
			h := NewScrollbackScreen(20, 5, 10)
			for _, op := range ops {
				op(h.Screen)
			}
			return h
		}
		want, got := run(false), run(true)
		if diff := screenDiff(want, got); diff != "" {
			t.Fatalf("iteration %d: fast path differs from the general path: %s\nscript:\n  %s",
				iter, diff, strings.Join(script, "\n  "))
		}
	}
}

// screenDiff says how got differs from want, or "" when it does not.
func screenDiff(want, got *ScrollbackScreen) string {
	if want.Cursor != got.Cursor || want.wrapNext != got.wrapNext {
		return fmt.Sprintf("cursor %+v wrapNext %v, want %+v %v", got.Cursor, got.wrapNext, want.Cursor, want.wrapNext)
	}
	if want.lastDrawn != got.lastDrawn {
		return fmt.Sprintf("lastDrawn %q, want %q", got.lastDrawn, want.lastDrawn)
	}
	for y := range want.Buffer {
		if !slices.Equal(want.Buffer[y], got.Buffer[y]) {
			return fmt.Sprintf("row %d:\n  got  %v\n  want %v", y, got.Buffer[y], want.Buffer[y])
		}
	}
	wh, gh := want.ScrollbackRows(), got.ScrollbackRows()
	if len(wh) != len(gh) {
		return fmt.Sprintf("%d history rows, want %d", len(gh), len(wh))
	}
	for i := range wh {
		if !slices.Equal(wh[i], gh[i]) {
			return fmt.Sprintf("history row %d:\n  got  %v\n  want %v", i, gh[i], wh[i])
		}
	}
	return ""
}
