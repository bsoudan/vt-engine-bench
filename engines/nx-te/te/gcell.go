package te

import "unicode/utf8"

// A screen stores its cells as gcells, eight bytes each, as Ghostty does:
// the text is a codepoint, or an index into the screen's table of
// multi-codepoint clusters, and the attributes are an index into its table
// of distinct Attrs. cell.Cell, 64 bytes with a string and the attributes
// inline, is what the screen hands out (Rows, ScrollbackRows, a snapshot,
// a delta) and takes in; the tables translate at that edge. A row of 120
// cells is then 960 bytes to fill or erase instead of 7.7 KB, and holds no
// pointer for the garbage collector to trace.
type gcell struct {
	text  uint32 // the codepoint; with flagGrapheme, the cluster's index; 0 is no text ("")
	style uint16 // the Attr's index in the screen's styles; 0 is Attr{}
	flags uint8  // flagWrapped and flagEmpty (cell.Cell's Wrapped and Empty), flagGrapheme
	_     uint8
}

const (
	flagWrapped uint8 = 1 << iota
	flagEmpty
	flagGrapheme
)

// gblank is the blank cell, cell.Cell.Blank: a space in the default
// attributes, not wrapped, not empty.
var gblank = gcell{text: ' '}

func (g gcell) wrapped() bool { return g.flags&flagWrapped != 0 }
func (g gcell) empty() bool   { return g.flags&flagEmpty != 0 }
func (g gcell) blank() bool   { return g == gblank }

// setWrapped and setEmpty are g with the flag as said.
func (g gcell) setWrapped(on bool) gcell { return g.withFlag(flagWrapped, on) }
func (g gcell) setEmpty(on bool) gcell   { return g.withFlag(flagEmpty, on) }

func (g gcell) withFlag(flag uint8, on bool) gcell {
	if on {
		g.flags |= flag
	} else {
		g.flags &^= flag
	}
	return g
}

// cellTables is what a screen's gcells index into: the distinct Attrs its
// cells have worn (styles[0] is Attr{}) and the multi-codepoint clusters
// they hold. Both grow as new ones are used and are compacted to what the
// screen, its alternate screen and its history still hold when the style
// table fills (compactTables).
type cellTables struct {
	styles      []Attr
	styleIDs    map[Attr]uint16
	graphemes   []string
	graphemeIDs map[string]uint32
	widths      map[gcell]int8 // the width of each text seen (widthOf)
	compacting  bool
	// guarded says a style in the table protects its cells from an erase
	// (DECSCA's or ISO 6429's protection). Every cell's style is in the
	// table, so until one does, no cell on the screen is protected and an
	// erase need not look at the cells it clears.
	guarded bool
}

// maxStyles is how many distinct Attrs the table holds, the style index
// being 16 bits.
const maxStyles = 1 << 16

// styleID is the index of a in the style table, added if new.
func (s *Screen) styleID(a Attr) uint16 {
	if a == (Attr{}) {
		return 0
	}
	t := &s.tables
	if id, ok := t.styleIDs[a]; ok {
		return id
	}
	if t.styleIDs == nil {
		t.styleIDs = make(map[Attr]uint16)
		t.styles = []Attr{{}}
	}
	if len(t.styles) == maxStyles {
		if t.compacting {
			return 0 // every style in use at once: the attributes are lost, not the text
		}
		s.compactTables()
		return s.styleID(a)
	}
	id := uint16(len(t.styles))
	t.styles = append(t.styles, a)
	t.styleIDs[a] = id
	if a.Protected || a.ISOProtected {
		t.guarded = true
	}
	return id
}

// style is the Attr at id.
func (s *Screen) style(id uint16) Attr {
	if id == 0 {
		return Attr{}
	}
	return s.tables.styles[id]
}

// graphemeID is the index of cluster in the grapheme table, added if new.
func (s *Screen) graphemeID(cluster string) uint32 {
	t := &s.tables
	if id, ok := t.graphemeIDs[cluster]; ok {
		return id
	}
	if t.graphemeIDs == nil {
		t.graphemeIDs = make(map[string]uint32)
		t.graphemes = []string{""} // index 0 is never a cluster: text 0 is no text
	}
	id := uint32(len(t.graphemes))
	t.graphemes = append(t.graphemes, cluster)
	t.graphemeIDs[cluster] = id
	return id
}

// compactTables rebuilds the tables from the cells that index into them:
// the screen's, the alternate screen's, and those moreRows says (a
// ScrollbackScreen's history), each cell's indexes replaced.
func (s *Screen) compactTables() {
	old := s.tables
	s.tables = cellTables{compacting: true}
	remap := func(rows [][]gcell) {
		for _, row := range rows {
			for i := range row {
				g := &row[i]
				if g.style != 0 {
					g.style = s.styleID(old.styles[g.style])
				}
				if g.flags&flagGrapheme != 0 {
					g.text = s.graphemeID(old.graphemes[g.text])
				}
			}
		}
	}
	remap(s.rows)
	remap(s.altRows)
	if s.moreRows != nil {
		remap(s.moreRows())
	}
	s.tables.compacting = false
}

// cellOf is the cell holding text, one grapheme cluster or "", in style.
func (s *Screen) cellOf(text string, style uint16) gcell {
	g := gcell{style: style}
	if text == "" {
		return g
	}
	if r, n := utf8.DecodeRuneInString(text); n == len(text) && (r != utf8.RuneError || text == "�") {
		g.text = uint32(r)
		return g
	}
	g.text = s.graphemeID(text)
	g.flags |= flagGrapheme
	return g
}

// blankOf is a space in a.
func (s *Screen) blankOf(a Attr) gcell { return gcell{text: ' ', style: s.styleID(a)} }

// text is g's text: the cluster it holds, "" for none.
func (s *Screen) text(g gcell) string {
	switch {
	case g.text == 0:
		return ""
	case g.flags&flagGrapheme != 0:
		return s.tables.graphemes[g.text]
	case g.text < 0x80:
		return asciiCell[g.text]
	}
	return string(rune(g.text))
}

// widthOf is the columns g's text takes under the screen's width policy:
// one for printable ASCII, none for no text, and for the rest what the
// policy says of it, remembered per text.
func (s *Screen) widthOf(g gcell) int {
	if g.text == 0 {
		return 0
	}
	if g.flags&flagGrapheme == 0 && g.text >= 0x20 && g.text < 0x7f {
		return 1
	}
	key := gcell{text: g.text, flags: g.flags & flagGrapheme}
	if w, ok := s.tables.widths[key]; ok {
		return int(w)
	}
	w := s.Widths.Width(s.text(g))
	if s.tables.widths == nil {
		s.tables.widths = make(map[gcell]int8)
	}
	s.tables.widths[key] = int8(w)
	return w
}

// pack is c as the screen stores it; unpack is the cell back.
func (s *Screen) pack(c Cell) gcell {
	g := s.cellOf(c.Data, s.styleID(c.Attr))
	return g.setWrapped(c.Wrapped).setEmpty(c.Empty)
}

func (s *Screen) unpack(g gcell) Cell {
	return Cell{Data: s.text(g), Attr: s.style(g.style), Wrapped: g.wrapped(), Empty: g.empty()}
}

// unpackRow is row as cells, into the slice given (reused) or a new one.
func (s *Screen) unpackRow(row []gcell, into []Cell) []Cell {
	into = into[:0]
	for _, g := range row {
		into = append(into, s.unpack(g))
	}
	return into
}

func (s *Screen) unpackRows(rows [][]gcell) [][]Cell {
	if rows == nil {
		return nil
	}
	out := make([][]Cell, len(rows))
	for i, row := range rows {
		out[i] = s.unpackRow(row, make([]Cell, 0, len(row)))
	}
	return out
}

func (s *Screen) packRow(row []Cell) []gcell {
	out := make([]gcell, len(row))
	for i, c := range row {
		out[i] = s.pack(c)
	}
	return out
}

func (s *Screen) packRows(rows [][]Cell) [][]gcell {
	if rows == nil {
		return nil
	}
	out := make([][]gcell, len(rows))
	for i, row := range rows {
		out[i] = s.packRow(row)
	}
	return out
}

// Rows is a copy of the screen's rows as cells. LinesCells is the same.
func (s *Screen) Rows() [][]Cell { return s.unpackRows(s.rows) }

// setRows makes rows the screen's, packed; tests put a screen in a state
// with it.
func (s *Screen) setRows(rows [][]Cell) { s.rows = s.packRows(rows) }

// blankLine is a row of cols cells, each cell.
func blankLine(cols int, cell gcell) []gcell {
	line := make([]gcell, cols)
	for col := range line {
		line[col] = cell
	}
	return line
}

// makeBlankCells is lines rows of cols blank cells.
func makeBlankCells(lines, cols int) [][]gcell {
	cells := make([][]gcell, lines)
	for row := range cells {
		cells[row] = blankLine(cols, gblank)
	}
	return cells
}

// fitRowsWidth truncates or pads every row in buf to exactly columns cells.
// A wrapped row stays wrapped: widened, what is added is empty and the wrap
// moves to the new last cell; narrowed, the wrap goes on the cell that is
// last now. Widening and narrowing back gives the row as it was.
func fitRowsWidth(buf [][]gcell, columns int, blank gcell) {
	for row := range buf {
		line := buf[row]
		wrapped := len(line) > 0 && line[len(line)-1].wrapped()
		switch {
		case len(line) > columns:
			line = line[:columns]
			if wrapped {
				line[columns-1] = line[columns-1].setWrapped(true)
			}
		case len(line) < columns:
			missing := make([]gcell, columns-len(line))
			for i := range missing {
				missing[i] = blank.setEmpty(wrapped)
			}
			if wrapped {
				line[len(line)-1] = line[len(line)-1].setWrapped(false)
				missing[len(missing)-1] = missing[len(missing)-1].setWrapped(true)
			}
			line = append(line, missing...)
		}
		buf[row] = line
	}
}

// blankRow says a row has nothing written on it: every cell blank. A row
// with a colored background, say, is not blank, and is kept rather than
// trimmed.
func blankRow(row []gcell) bool {
	for _, g := range row {
		if !g.blank() {
			return false
		}
	}
	return true
}
