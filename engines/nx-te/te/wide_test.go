package te

import (
	"testing"

	"nx/cell"
)

// Terminals draw an emoji two columns wide whether it is a wide character on
// its own (😀) or a text-default character made an emoji by the presentation
// selector U+FE0F (❤️, ✔️, ⚠️), and a modifier or ZWJ sequence (👍🏽, 👨‍👩‍👧) as one
// two-column glyph. The screen must agree, or the cursor it reports — and the
// place the console then puts the real cursor — drifts from where the terminal
// actually drew the text. A prompt with a heart in it is the everyday case.
func TestDrawEmojiWidths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
		cols int // columns the text occupies
	}{
		{"wide emoji", "😀", 2},
		{"heart with presentation selector", "❤️", 2},
		{"check mark with presentation selector", "✔️", 2},
		{"warning with presentation selector", "⚠️", 2},
		{"text-default heart without the selector", "❤", 1},
		{"skin tone modifier", "👍🏽", 2},
		{"zwj family", "👨‍👩‍👧", 2},
		{"flag", "🇩🇪", 2},
		{"prompt", "❤️> ", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScreen(40, 2)
			s.Draw(tc.text)
			if s.Cursor.Col != tc.cols {
				t.Fatalf("Draw(%q): cursor at column %d, want %d", tc.text, s.Cursor.Col, tc.cols)
			}
			// Every wide cluster leaves exactly one placeholder after it, so
			// the renderer emits the glyph once and skips the second column.
			placeholders := 0
			for _, c := range s.Buffer[0][:tc.cols] {
				if c.Data == "" {
					placeholders++
				}
			}
			if want := tc.cols - len(visible(tc.text)); placeholders != want {
				t.Fatalf("Draw(%q): %d placeholder cells, want %d: %q", tc.text, placeholders, want, s.Display()[0])
			}
		})
	}
}

// visible counts the grapheme clusters that occupy a column.
func visible(s string) []string {
	var out []string
	for _, r := range []rune(s) {
		switch {
		case r == 0xfe0f || r == 0x200d || (r >= 0x1f3fb && r <= 0x1f3ff):
			// selectors, joiners, modifiers ride along
		case r >= 0x1f1e6 && r <= 0x1f1ff && len(out) > 0 && isRegional(out[len(out)-1]):
			// second half of a flag
		case r == '👩' || r == '👧':
			if len(out) > 0 && out[len(out)-1] == "👨" {
				continue // joined into the family
			}
			out = append(out, string(r))
		default:
			out = append(out, string(r))
		}
	}
	return out
}

func isRegional(s string) bool {
	r := []rune(s)
	return len(r) == 1 && r[0] >= 0x1f1e6 && r[0] <= 0x1f1ff
}

// Terminals disagree about the emoji classes above; the screen sizes them by
// the policy it is told, so its cursor lands where the real terminal's does.
func TestDrawEmojiWidthsFollowThePolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		policy cell.WidthPolicy
		text   string
		cols   int
	}{
		{"unicode: heart", cell.Unicode, "❤️", 2},
		{"wcwidth: heart", cell.WidthPolicy{}, "❤️", 1},
		{"unicode: family", cell.Unicode, "👨‍👩‍👧", 2},
		{"wcwidth: family", cell.WidthPolicy{}, "👨‍👩‍👧", 6},
		{"unicode: skin tone", cell.Unicode, "👍🏽", 2},
		{"wcwidth: skin tone", cell.WidthPolicy{}, "👍🏽", 4},
		{"wcwidth: a wide emoji is still wide", cell.WidthPolicy{}, "😀", 2},
		{"vs16 only: family stays split", cell.WidthPolicy{VS16: true}, "👨‍👩‍👧", 6},
		{"wcwidth: prompt", cell.WidthPolicy{}, "❤️> ", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScreen(40, 2)
			s.Widths = tc.policy
			s.Draw(tc.text)
			if s.Cursor.Col != tc.cols {
				t.Fatalf("Draw(%q) under %+v: cursor at column %d, want %d", tc.text, tc.policy, s.Cursor.Col, tc.cols)
			}
		})
	}
}
