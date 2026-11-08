package cell

import "testing"

// Put lays text out the way the emulator would draw it: a wide glyph takes a
// cell and a placeholder, so whatever renders the row emits the glyph once and
// skips its second column, and the text after it lands where the terminal
// will actually put it.
func TestPutLeavesPlaceholdersAfterWideGlyphs(t *testing.T) {
	row := make([]Cell, 8)
	for i := range row {
		row[i] = Cell{Data: " "}
	}
	next := Unicode.Put(row, 1, "a😀b", Attr{Bold: true})
	if next != 5 {
		t.Fatalf("next column %d, want 5", next)
	}
	want := []string{" ", "a", "😀", "", "b", " ", " ", " "}
	for i, w := range want {
		if row[i].Data != w {
			t.Fatalf("cell %d is %q, want %q (row %q)", i, row[i].Data, w, cellsText(row))
		}
	}
	if !row[3].Attr.Bold {
		t.Fatal("the placeholder must carry the glyph's attributes")
	}
	// Under a policy that splits the family, it is laid out as three glyphs.
	row = make([]Cell, 8)
	if next := (WidthPolicy{}).Put(row, 0, "👨‍👩‍👧", Attr{}); next != 6 {
		t.Fatalf("a split family took %d columns, want 6", next)
	}
	// Clipping: nothing past the row's end, but the width is still counted.
	row = make([]Cell, 3)
	if next := Unicode.Put(row, 2, "😀x", Attr{}); next != 5 || row[2].Data != "" {
		t.Fatalf("clipped put: next %d, cell 2 %q", next, row[2].Data)
	}
}

// Text is a row as it reads, a wide glyph once (its placeholder adds
// nothing), and so is a transparent cell; Lines is a grid's rows so, a row
// with none of its own an empty line.
func TestTextReadsARowAsItIsDrawn(t *testing.T) {
	row := BlankRow(6)
	Unicode.Put(row, 0, "a😀b", Attr{})
	row[5] = Cell{}
	if got := Text(row); got != "a😀b " {
		t.Fatalf("Text is %q, want %q", got, "a😀b ")
	}
	if got := Text(nil); got != "" {
		t.Fatalf("Text of no row is %q, want empty", got)
	}
	lines := Lines([][]Cell{row, nil, BlankRow(2)})
	if len(lines) != 3 || lines[0] != "a😀b " || lines[1] != "" || lines[2] != "  " {
		t.Fatalf("Lines is %q, want [%q %q %q]", lines, "a😀b ", "", "  ")
	}
	if got := Lines(nil); len(got) != 0 {
		t.Fatalf("Lines of no grid is %q, want none", got)
	}
}

func TestStringWidthFollowsThePolicy(t *testing.T) {
	if w := Unicode.StringWidth("❤️> "); w != 4 {
		t.Fatalf("unicode width of a heart prompt: %d, want 4", w)
	}
	if w := (WidthPolicy{}).StringWidth("❤️> "); w != 3 {
		t.Fatalf("wcwidth width of a heart prompt: %d, want 3", w)
	}
}

func cellsText(row []Cell) string {
	s := ""
	for _, c := range row {
		if c.Data == "" {
			s += "·"
		} else {
			s += c.Data
		}
	}
	return s
}

// Emoji is a cluster a font may draw in color: wide from the emoji blocks,
// asked for by U+FE0F, or a narrow text-default emoji (the table's first and
// last, ✳ among them); not a wide CJK character, nor a narrow symbol with no
// emoji of its own.
func TestEmojiIsWhatAFontMayDrawInColor(t *testing.T) {
	for _, c := range []struct {
		cluster string
		wide    bool
		want    bool
	}{
		{"😀", true, true}, {"❤️", false, true}, {"⌨", false, true}, {"✳", false, true}, {"🛳", false, true},
		{"漢", true, false}, {"★", false, false}, {"❯", false, false}, {"⏣", false, false}, {"a", false, false},
	} {
		if got := Emoji(c.cluster, c.wide); got != c.want {
			t.Errorf("Emoji(%q, %v) = %v; want %v", c.cluster, c.wide, got, c.want)
		}
	}
}

// Fit cuts by columns, whole clusters only: a wide glyph that would cross
// the edge goes, and the ellipsis lands on the cell boundary it left; a
// combining mark stays with its letter.
func TestFitCutsByColumns(t *testing.T) {
	for _, tc := range []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"日本語", 6, "日本語"},
		{"日本語", 5, "日本…"},
		{"日本語", 4, "日…"}, // 日 and the ellipsis: three columns, the glyph not split
		{"日本語", 2, "…"},
		{"café au lait", 5, "café…"},
		{"café au lait", 4, "caf…"},
	} {
		got := Unicode.Fit(tc.s, tc.n)
		if got != tc.want || Unicode.StringWidth(got) > tc.n {
			t.Errorf("Fit(%q, %d) = %q (%d columns); want %q", tc.s, tc.n, got, Unicode.StringWidth(got), tc.want)
		}
	}
}
