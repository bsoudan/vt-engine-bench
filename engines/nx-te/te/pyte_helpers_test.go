// The pyte_*_test.go files port the tests of pyte (https://github.com/selectel/pyte),
// the emulator this package's screen and history models descend from, each
// test naming the upstream test it came from. The helpers here are what
// those ports need.
package te

import (
	"slices"
	"strings"
	"testing"
)

func assertDisplay(t *testing.T, screen *Screen, expected []string) {
	t.Helper()
	got := screen.Display()
	if len(got) != len(expected) {
		t.Fatalf("expected %d lines, got %d", len(expected), len(got))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("line %d expected %q, got %q", i, expected[i], got[i])
		}
	}
}

func updateScreen(screen *Screen, lines []string, colored []int) *Screen {
	coloredSet := map[int]struct{}{}
	for _, row := range colored {
		coloredSet[row] = struct{}{}
	}
	for y, line := range lines {
		for x, char := range line {
			attr := screen.defaultAttr()
			if _, ok := coloredSet[y]; ok {
				attr.Fg = pyteColor("red")
			}
			screen.rows[y][x] = screen.pack(Cell{Data: string(char), Attr: attr})
		}
	}
	return screen
}

func tolist(screen *Screen) [][]Cell {
	out := make([][]Cell, screen.Lines)
	for y := 0; y < screen.Lines; y++ {
		row := make([]Cell, screen.Columns)
		copy(row, screen.Rows()[y])
		out[y] = row
	}
	return out
}

type attrOpt func(*Attr)

func cellWith(screen *Screen, data string, opts ...attrOpt) Cell {
	attr := screen.defaultAttr()
	for _, opt := range opts {
		opt(&attr)
	}
	return Cell{Data: data, Attr: attr}
}

// pyteColor is the color pyte names: "default", one of the eight ANSI colors
// (pyte says brown for yellow), or one of them bright.
func pyteColor(name string) Color {
	ansi := []string{"black", "red", "green", "brown", "blue", "magenta", "cyan", "white"}
	name, bright := strings.CutPrefix(name, "bright")
	i := slices.Index(ansi, name)
	if i < 0 {
		return Color{}
	}
	if bright {
		i += 8
	}
	return Color{Mode: ColorANSI16, Index: uint8(i)}
}

func withFg(name string) attrOpt {
	return func(attr *Attr) { attr.Fg = pyteColor(name) }
}

func withBg(name string) attrOpt {
	return func(attr *Attr) { attr.Bg = pyteColor(name) }
}

func withBold(value bool) attrOpt {
	return func(attr *Attr) {
		attr.Bold = value
	}
}

func withFaint(value bool) attrOpt {
	return func(attr *Attr) {
		attr.Faint = value
	}
}

func withItalics(value bool) attrOpt {
	return func(attr *Attr) {
		attr.Italics = value
	}
}

func withUnderline(value bool) attrOpt {
	return func(attr *Attr) {
		attr.Underline = value
	}
}

func withStrikethrough(value bool) attrOpt {
	return func(attr *Attr) {
		attr.Strikethrough = value
	}
}

func withReverse(value bool) attrOpt {
	return func(attr *Attr) {
		attr.Reverse = value
	}
}

func withBlink(value bool) attrOpt {
	return func(attr *Attr) {
		attr.Blink = value
	}
}

func withConceal(value bool) attrOpt {
	return func(attr *Attr) {
		attr.Conceal = value
	}
}

func assertCellsEqual(t *testing.T, got [][]Cell, want [][]Cell) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d rows, got %d", len(want), len(got))
	}
	for row := range want {
		if len(got[row]) != len(want[row]) {
			t.Fatalf("row %d expected %d cols, got %d", row, len(want[row]), len(got[row]))
		}
		for col := range want[row] {
			assertCellEqual(t, got[row][col], want[row][col], row, col)
		}
	}
}

func assertCellEqual(t *testing.T, got Cell, want Cell, row, col int) {
	t.Helper()
	if got.Data != want.Data {
		t.Fatalf("cell(%d,%d) expected data %q, got %q", row, col, want.Data, got.Data)
	}
	if got.Attr.Fg != want.Attr.Fg {
		t.Fatalf("cell(%d,%d) expected fg %+v, got %+v", row, col, want.Attr.Fg, got.Attr.Fg)
	}
	if got.Attr.Bg != want.Attr.Bg {
		t.Fatalf("cell(%d,%d) expected bg %+v, got %+v", row, col, want.Attr.Bg, got.Attr.Bg)
	}
	if got.Attr.Bold != want.Attr.Bold || got.Attr.Italics != want.Attr.Italics ||
		got.Attr.Underline != want.Attr.Underline || got.Attr.Strikethrough != want.Attr.Strikethrough ||
		got.Attr.Reverse != want.Attr.Reverse || got.Attr.Blink != want.Attr.Blink || got.Attr.Conceal != want.Attr.Conceal {
		t.Fatalf("cell(%d,%d) attr mismatch", row, col)
	}
}
