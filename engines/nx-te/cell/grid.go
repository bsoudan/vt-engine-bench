package cell

import (
	"encoding/json"
	"errors"
	"unicode/utf8"
)

// Grid is rows of cells, as a frame carries them (doc/done/GRID_CODEC_PLAN.md),
// with a JSON form of its own: what its cells are besides their text, said
// once each in a table, and each row as runs of neighbouring cells that
// share an entry, rather than an object per cell. A row of plain text is a
// run and its text, where the object per cell was some seventy bytes a
// character — and reads as the text it is.
//
//	{"Attrs":[{},{"Attr":{"Bold":true}}],
//	 "Runs":[[{"T":"• "},{"T":"«1▪sh»","A":1},{"N":3}]]}
//
// An entry is a cell without its text, in Cell's own JSON, so a field added
// to Cell is carried without a new form. A row is runs (Run). A nil row is
// null; a nil grid, null.
//
// It is [][]Cell underneath, assignable both ways, so what builds and reads
// a frame's cells is unchanged by it.
type Grid [][]Cell

// gridJSON is a Grid's JSON form.
type gridJSON struct {
	Attrs []Cell  `json:"Attrs,omitempty"`
	Runs  [][]Run `json:"Runs"`
}

// Run is neighbouring cells of a row that share an entry of a table (a
// Grid's, a Delta's): A, the entry (0 left out), and one of T, its text, one
// character a cell; N, a count of cells with no text (transparent, or the
// second half of a wide glyph); C, each cell's text, for cells whose text is
// not one character.
type Run struct { // the text first: it is what a reader looks for
	T string   `json:"T,omitempty"`
	N int      `json:"N,omitempty"`
	C []string `json:"C,omitempty"`
	A int      `json:"A,omitempty"`
}

// Len is how many cells r is.
func (r Run) Len() int {
	switch {
	case r.T != "":
		return utf8.RuneCountInString(r.T)
	case r.C != nil:
		return len(r.C)
	}
	return r.N
}

// The kinds of run: how its cells' text is said.
const (
	runText  = iota // T: one character a cell
	runNone         // N: no text
	runCells        // C: each cell's own
)

// runKind is the kind of run a cell's text goes in.
func runKind(data string) int {
	switch {
	case data == "":
		return runNone
	case utf8.RuneCountInString(data) == 1 && utf8.ValidString(data):
		return runText
	}
	return runCells
}

// table is the entries runs name: each cell without its text, once.
type table struct {
	index map[Cell]int
	cells []Cell
	last  Cell // the entry asked for last, and its number: a row's cells
	lastA int  // are mostly the one before's
}

// newTable is a table holding cells already, numbered as they are.
func newTable(cells []Cell) *table {
	t := &table{index: make(map[Cell]int, len(cells)), cells: cells, lastA: -1}
	for i, c := range cells {
		if _, ok := t.index[c]; !ok {
			t.index[c] = i
		}
	}
	return t
}

// entry is c's entry, added if it is new.
func (t *table) entry(c Cell) int {
	c.Data = ""
	if t.lastA >= 0 && c == t.last {
		return t.lastA
	}
	i, ok := t.index[c]
	if !ok {
		i = len(t.cells)
		t.index[c] = i
		t.cells = append(t.cells, c)
	}
	t.last, t.lastA = c, i
	return i
}

// runsOf is row as runs of t's entries, added to t as they are new. An empty
// row is no runs, not nil.
func runsOf(row []Cell, t *table) []Run {
	runs := []Run{}
	for x := 0; x < len(row); {
		a, kind := t.entry(row[x]), runKind(row[x].Data)
		end := x + 1
		for end < len(row) && t.entry(row[end]) == a && runKind(row[end].Data) == kind {
			end++
		}
		r := Run{A: a}
		switch kind {
		case runText:
			b := make([]byte, 0, end-x)
			for _, c := range row[x:end] {
				b = append(b, c.Data...)
			}
			r.T = string(b)
		case runNone:
			r.N = end - x
		case runCells:
			r.C = make([]string, 0, end-x)
			for _, c := range row[x:end] {
				r.C = append(r.C, c.Data)
			}
		}
		runs = append(runs, r)
		x = end
	}
	return runs
}

var errGridEntry = errors.New("cell: a run names no entry of its grid's table")

// entryOf is entry a of attrs, a cell without its text: none, of no table,
// is the zero cell.
func entryOf(attrs []Cell, a int) (Cell, error) {
	if a < 0 || a >= len(attrs) {
		if a != 0 || len(attrs) != 0 {
			return Cell{}, errGridEntry
		}
		return Cell{}, nil
	}
	c := attrs[a]
	c.Data = ""
	return c, nil
}

// cellsOf appends runs' cells, their entries attrs', to row.
func cellsOf(row []Cell, runs []Run, attrs []Cell) ([]Cell, error) {
	for _, r := range runs {
		base, err := entryOf(attrs, r.A)
		if err != nil {
			return row, err
		}
		switch {
		case r.T != "":
			for _, ch := range r.T {
				c := base
				c.Data = string(ch)
				row = append(row, c)
			}
		case r.C != nil:
			for _, data := range r.C {
				c := base
				c.Data = data
				row = append(row, c)
			}
		default:
			for range r.N {
				row = append(row, base)
			}
		}
	}
	return row, nil
}

// MarshalJSON is the grid as runs.
func (g Grid) MarshalJSON() ([]byte, error) {
	if g == nil {
		return []byte("null"), nil
	}
	t := newTable(nil)
	out := gridJSON{Runs: make([][]Run, len(g))}
	for y, row := range g {
		if row != nil { // else null
			out.Runs[y] = runsOf(row, t)
		}
	}
	out.Attrs = t.cells
	return json.Marshal(out)
}

// UnmarshalJSON reads a grid's runs.
func (g *Grid) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*g = nil
		return nil
	}
	var in gridJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	out := make(Grid, len(in.Runs))
	for y, runs := range in.Runs {
		if runs == nil {
			continue
		}
		row, err := cellsOf([]Cell{}, runs, in.Attrs)
		if err != nil {
			return err
		}
		out[y] = row
	}
	*g = out
	return nil
}
