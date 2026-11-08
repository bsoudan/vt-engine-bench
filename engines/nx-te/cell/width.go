package cell

import (
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// WidthPolicy is how the terminal a grid is drawn into sizes the emoji that
// terminals disagree about. Unicode says a text-default character followed by
// the presentation selector U+FE0F (❤️) is two columns, a zero-width-joiner
// sequence (👨‍👩‍👧) is one two-column glyph, and a skin-tone modifier (👍🏽) joins
// its base. Some terminals do all of that, some none, some part — and a shadow
// screen that guesses wrong puts the cursor in the wrong column. The console
// measures the real terminal and hands the answer to every screen it shadows.
//
// Each field is true when the terminal follows Unicode for that class. The
// zero value is a terminal that follows none of it, which is what wcwidth-based
// terminals do; Unicode is the policy the console assumes until it has asked.
type WidthPolicy struct {
	VS16     bool // the presentation selector makes a text-default character wide
	ZWJ      bool // a zero-width-joiner sequence is one glyph
	Modifier bool // a skin-tone modifier joins its base rather than drawing beside it
}

// Unicode is the policy of a terminal that renders emoji the way Unicode says.
var Unicode = WidthPolicy{VS16: true, ZWJ: true, Modifier: true}

const (
	vs16 = "️"
	zwj  = "‍"
)

// Pieces splits one grapheme cluster into the glyphs the terminal will draw
// for it under the policy: the cluster itself when the terminal joins it, or
// its parts — the members of a ZWJ sequence, a base and its modifiers — when it
// draws them side by side. Each piece is at most two columns wide.
func (p WidthPolicy) Pieces(cluster string) []string {
	if !p.ZWJ && strings.Contains(cluster, zwj) {
		var out []string
		for _, part := range strings.Split(cluster, zwj) {
			if part != "" {
				out = append(out, p.Pieces(part)...)
			}
		}
		return out
	}
	if !p.Modifier {
		if base, mods := splitModifiers(cluster); len(mods) > 0 {
			return append(p.Pieces(base), mods...)
		}
	}
	return []string{cluster}
}

// Width is the number of columns one grapheme cluster occupies under the
// policy: the sum of its pieces.
func (p WidthPolicy) Width(cluster string) int {
	w := 0
	for _, piece := range p.Pieces(cluster) {
		w += p.pieceWidth(piece)
	}
	return w
}

// pieceWidth sizes one glyph. Only the presentation selector is in question
// here: a terminal that ignores it draws the base character at its own width.
func (p WidthPolicy) pieceWidth(piece string) int {
	if r, n := utf8.DecodeRuneInString(piece); n == len(piece) && r >= 0x1f3fb && r <= 0x1f3ff {
		return 2 // a modifier drawn on its own is a glyph in its own right
	}
	if !p.VS16 && strings.Contains(piece, vs16) {
		return uniseg.StringWidth(strings.ReplaceAll(piece, vs16, ""))
	}
	return uniseg.StringWidth(piece)
}

// splitModifiers separates a cluster into what is left without its skin-tone
// modifiers (U+1F3FB..U+1F3FF) and the modifiers themselves, in order.
func splitModifiers(cluster string) (string, []string) {
	var base strings.Builder
	var mods []string
	for _, r := range cluster {
		if r >= 0x1f3fb && r <= 0x1f3ff {
			mods = append(mods, string(r))
			continue
		}
		base.WriteRune(r)
	}
	return base.String(), mods
}

// StringWidth is the number of columns s occupies under the policy.
func (p WidthPolicy) StringWidth(s string) int {
	w := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		w += p.Width(g.Str())
	}
	return w
}

// Fit cuts s to at most n columns under the policy, ending it in an
// ellipsis where it was cut: whole clusters only, so a wide glyph is not
// split across the edge (the ellipsis lands on a cell boundary, a column
// short if that is where the glyph left it) and a combining mark stays
// with what it marks. n of 0 or less is "".
func (p WidthPolicy) Fit(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if p.StringWidth(s) <= n {
		return s
	}
	var b strings.Builder
	w := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cw := p.Width(g.Str())
		if w+cw+1 > n { // the ellipsis takes a column
			break
		}
		b.WriteString(g.Str())
		w += cw
	}
	return b.String() + "…"
}

// Put lays s out into row from col, one cell per column: a wide cluster takes
// its cell and leaves the empty placeholder the renderer expects in the next,
// a zero-width cluster joins the cell before it, and nothing is written past
// the row's end. It returns the column after the text.
func (p WidthPolicy) Put(row []Cell, col int, s string, a Attr) int {
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cluster := g.Str()
		for _, piece := range p.Pieces(cluster) {
			w := p.pieceWidth(piece)
			switch {
			case w == 0:
				if col > 0 && col-1 < len(row) {
					row[col-1].Data += piece
				}
			case col+w > len(row) || col < 0:
				// No room: clipped, but the width still advances.
			default:
				row[col] = Cell{Data: piece, Attr: a}
				for i := 1; i < w; i++ {
					row[col+i] = Cell{Data: "", Attr: a}
				}
			}
			col += w
		}
	}
	return col
}

// Emoji says a cell's cluster may be drawn as a color emoji: asked for by the
// presentation selector (U+FE0F), wide and from the blocks emoji are drawn
// from, or one of the narrow ones there a font may draw in color all the same
// (textEmoji: ✳, not ★ or ❯). A wide CJK character is in none of them.
func Emoji(cluster string, wide bool) bool {
	if strings.ContainsRune(cluster, '️') {
		return true
	}
	r, _ := utf8.DecodeRuneInString(cluster)
	if !wide {
		return isTextEmoji(r)
	}
	return r >= 0x1F000 && r <= 0x1FAFF || r >= 0x2300 && r <= 0x23FF || r >= 0x2600 && r <= 0x27BF || r >= 0x2B00 && r <= 0x2BFF
}

// Shade hides each color emoji in row[from:to] behind ░, a column at a
// time: an emoji's colors are the font's, and take no grey, so what is drawn
// in one is shaded over instead.
func Shade(row []Cell, from, to int) {
	for c := max(from, 0); c < min(to, len(row)); c++ {
		if wide := c+1 < len(row) && row[c+1].Data == ""; Emoji(row[c].Data, wide) {
			row[c].Data = "░"
			if wide {
				row[c+1].Data = "░"
			}
		}
	}
}
