package cell

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// Delta is what changed in a grid since the one before
// (doc/done/DELTA_PLAN.md): its size, the scrolls that happened inside it,
// in order, and then the cells that differ, as spans of runs with a table of
// attributes, the form a Grid's JSON has. Applied to a grid that is as the
// one before was, it makes the one after; two deltas in a row are one delta
// (Merge). A whole delta replaces the grid: blank, Cols×Rows, then the
// scrolls and the cells.
//
// The order is complete whatever happened between two deltas: a row written
// and then scrolled is found by the cells at the row it ended on, and a
// scroll not said costs rows, never a wrong cell. The rows a scroll uncovers
// are blank (BlankRow); a program that left them in another color has the
// cells say so.
//
// A delta applies to a grid with a history (Apply) and to a canvas, rows a
// screen is the last of (ApplyCanvas), where the history scroll is rows
// appended: so a frontend holds a vtapp's screen as a plain canvas, with no
// emulator.
//
// Its JSON is Grid's runs with the scrolls beside them; its bytes
// (MarshalBinary) the same runs with varints, what a module boundary copies.
type Delta struct {
	Whole   bool     `json:",omitempty"` // the grid is replaced: blank, Cols×Rows, then the scrolls and cells
	Cols    int      `json:",omitempty"` // the grid's size after this delta
	Rows    int      `json:",omitempty"`
	Scrolls []Scroll `json:",omitempty"` // in order, before the cells (but see Span.Before)
	Attrs   []Cell   `json:",omitempty"` // the table: a cell without its text (Grid's Attrs)
	Spans   []Span   `json:",omitempty"` // runs of cells from (Row, Col), each run an entry of the table
}

// Scroll is rows Top..Bottom of a grid, the whole width, shifted up N rows
// (N < 0: down), the rows uncovered blank. A History scroll is the primary
// screen's full scroll, whose rows leaving the top are kept: they are the
// grid's own, as it has them then, but those Unseen says — a row written
// and scrolled off between two deltas, which no delta showed. Such a row
// does not ride the scroll (doc/done/DELTA_PLAN.md, Decided 6): it is kept
// as a blank row, and a receiver that wants it asks its sender for it (a
// view paging into it). So a scroll costs what was shown, never the rows
// that went past unseen.
type Scroll struct {
	Top     int  `json:",omitempty"`
	Bottom  int  `json:",omitempty"`
	N       int  `json:",omitempty"`
	History bool `json:",omitempty"`
	// Unseen is the rows leaving that no delta showed, as runs of the N,
	// oldest first, in order. None: every row leaving is as the grid has
	// it.
	Unseen []Rows `json:",omitempty"`
}

// Rows is N rows from From: a run of the rows a scroll moves off.
type Rows struct {
	From int `json:",omitempty"`
	N    int `json:",omitempty"`
}

// unseen says whether row i of the rows s moves off is one Unseen says.
func (s Scroll) unseen(i int) bool {
	for _, r := range s.Unseen {
		if i >= r.From && i < r.From+r.N {
			return true
		}
	}
	return false
}

// Span is cells of one row from a column, as runs of the delta's table.
// Before is how many of the delta's scrolls come after it: zero, the cells
// after every scroll, as a delta cut from a screen has them; a merge of a
// delta with cells and one with scrolls has the first's cells before the
// second's scrolls (Merge).
type Span struct {
	Row    int   `json:",omitempty"`
	Col    int   `json:",omitempty"`
	Before int   `json:",omitempty"`
	Runs   []Run `json:",omitempty"`
}

// Len is how many cells the span is.
func (s Span) Len() int {
	n := 0
	for _, r := range s.Runs {
		n += r.Len()
	}
	return n
}

// maxCells is the most cells a delta's grid may have: a delta's size comes
// from its sender, and a whole one is that many cells made.
const maxCells = 1 << 24

// ErrDelta is a delta that cannot be applied to what it is given: a size
// that is not the grid's, a scroll or a span outside it, a run naming no
// entry of the table. The receiver's state is untouched; it is behind or
// was sent a bad delta, and asks for the whole again.
var ErrDelta = errors.New("cell: a delta that does not fit its grid")

// Empty says d changes nothing.
func (d Delta) Empty() bool { return !d.Whole && len(d.Scrolls) == 0 && len(d.Spans) == 0 }

// Whole is a whole delta of grid: the cells of it that are not blank. Its
// size is the grid's, as wide as its widest row.
func Whole(grid [][]Cell) Delta {
	cols := 0
	for _, row := range grid {
		cols = max(cols, len(row))
	}
	d := Delta{Whole: true, Cols: cols, Rows: len(grid)}
	t := newTable(nil)
	for y, row := range grid {
		d.Spans = spansOf(d.Spans, y, nil, row, t)
	}
	d.Attrs = t.cells
	return d
}

// check says whether d is well made: its size, its scrolls and spans inside
// it, its runs naming entries of its table, its spans in an order Apply can
// follow.
func (d Delta) check() error {
	if d.Cols < 0 || d.Rows < 0 || (d.Cols > 0 && d.Rows > maxCells/d.Cols) {
		return fmt.Errorf("%w: a size of %dx%d", ErrDelta, d.Cols, d.Rows)
	}
	for _, s := range d.Scrolls {
		if s.Top < 0 || s.Bottom >= d.Rows || s.Top > s.Bottom || s.N == 0 || s.N < -maxCells || s.N > maxCells {
			return fmt.Errorf("%w: a scroll of rows %d..%d by %d in %d", ErrDelta, s.Top, s.Bottom, s.N, d.Rows)
		}
		// A history scroll is that many rows kept, or put on a canvas.
		if s.History && (s.Top != 0 || s.Bottom != d.Rows-1 || s.N < 0 || s.N > maxCells/max(d.Cols, 1)) {
			return fmt.Errorf("%w: a history scroll of rows %d..%d by %d", ErrDelta, s.Top, s.Bottom, s.N)
		}
		if s.Unseen != nil && !s.History {
			return fmt.Errorf("%w: rows unseen off a scroll that keeps none", ErrDelta)
		}
		next := 0
		for _, r := range s.Unseen {
			if r.From < next || r.N <= 0 || r.N > s.N || r.From > s.N-r.N {
				return fmt.Errorf("%w: %d rows unseen from %d off a scroll of %d", ErrDelta, r.N, r.From, s.N)
			}
			next = r.From + r.N
		}
	}
	before := len(d.Scrolls)
	for _, sp := range d.Spans {
		if sp.Row < 0 || sp.Row >= d.Rows || sp.Col < 0 || sp.Before < 0 || sp.Before > before {
			return fmt.Errorf("%w: a span at %d,%d before %d of %d scrolls", ErrDelta, sp.Row, sp.Col, sp.Before, len(d.Scrolls))
		}
		before = sp.Before
		n := 0
		for _, r := range sp.Runs {
			if _, err := entryOf(d.Attrs, r.A); err != nil {
				return fmt.Errorf("%w: %v", ErrDelta, err)
			}
			if r.Len() < 0 {
				return fmt.Errorf("%w: a run of %d cells", ErrDelta, r.Len())
			}
			n += r.Len()
		}
		if n > d.Cols-sp.Col {
			return fmt.Errorf("%w: a span of %d cells at %d,%d in %d", ErrDelta, n, sp.Row, sp.Col, d.Cols)
		}
	}
	return nil
}

// each does d's scrolls and spans in their order: each span after the
// scrolls before it.
func (d Delta) each(scroll func(Scroll), span func(Span)) {
	done := 0
	for _, sp := range d.Spans {
		for ; done < len(d.Scrolls)-sp.Before; done++ {
			scroll(d.Scrolls[done])
		}
		span(sp)
	}
	for ; done < len(d.Scrolls); done++ {
		scroll(d.Scrolls[done])
	}
}

// blankGrid is rows blank rows, cols wide.
func blankGrid(cols, rows int) [][]Cell {
	g := make([][]Cell, rows)
	for i := range g {
		g[i] = BlankRow(cols)
	}
	return g
}

// fits says whether grid is cols×rows.
func fits(grid [][]Cell, cols, rows int) bool {
	if len(grid) != rows {
		return false
	}
	for _, row := range grid {
		if len(row) != cols {
			return false
		}
	}
	return true
}

// write puts span's cells in row.
func (d Delta) write(row []Cell, sp Span) {
	x := sp.Col
	for _, r := range sp.Runs {
		base, _ := entryOf(d.Attrs, r.A) // checked
		switch {
		case r.T != "":
			for i, ch := range r.T { // each cell's text the run's own: no copy
				row[x] = base
				if ch == utf8.RuneError {
					row[x].Data = string(ch) // as a conversion reads it, whatever bytes it was
				} else {
					row[x].Data = r.T[i : i+utf8.RuneLen(ch)]
				}
				x++
			}
		case r.C != nil:
			for _, data := range r.C {
				row[x] = base
				row[x].Data = data
				x++
			}
		default:
			for range r.N {
				row[x] = base
				x++
			}
		}
	}
}

// scrollRows shifts rows (a scroll's region) by n, the rows uncovered blank,
// cols wide; what leaves the top of an upward shift is given to off, oldest
// first, the rows past the region's height blank.
func scrollRows(rows [][]Cell, n, cols int, off func(i int, row []Cell)) {
	h := len(rows)
	switch {
	case n > 0:
		for i := range n {
			if off == nil {
				break
			}
			if i < h {
				off(i, rows[i])
			} else {
				off(i, nil)
			}
		}
		k := min(n, h)
		copy(rows, rows[k:])
		for i := h - k; i < h; i++ {
			rows[i] = BlankRow(cols)
		}
	case n < 0:
		k := min(-n, h)
		copy(rows[k:], rows[:h-k])
		for i := range k {
			rows[i] = BlankRow(cols)
		}
	}
}

// Apply applies d to grid, which is as the delta before left it, and says
// the grid after: a whole delta a new one, else grid itself, its rows
// moved and written in place. The rows a history scroll moves off the top
// go to keep, oldest first (a blank one for each past the screen's
// height), those its Unseen says blank and said unseen; keep may be nil,
// and must not change them. A delta that does not fit grid is ErrDelta,
// and grid is left as it was.
func (d Delta) Apply(grid [][]Cell, keep func(row []Cell, unseen bool)) ([][]Cell, error) {
	if err := d.check(); err != nil {
		return grid, err
	}
	if d.Whole {
		grid = blankGrid(d.Cols, d.Rows)
	} else if !fits(grid, d.Cols, d.Rows) {
		return grid, fmt.Errorf("%w: a delta of %dx%d on a grid of %d rows", ErrDelta, d.Cols, d.Rows, len(grid))
	}
	d.each(func(s Scroll) {
		var off func(int, []Cell)
		if s.History && keep != nil {
			off = func(i int, row []Cell) {
				unseen := s.unseen(i)
				if row == nil || unseen {
					row = BlankRow(d.Cols)
				}
				keep(row, unseen)
			}
		}
		scrollRows(grid[s.Top:s.Bottom+1], s.N, d.Cols, off)
	}, func(sp Span) { d.write(grid[sp.Row], sp) })
	return grid, nil
}

// ApplyCanvas applies d to a canvas (doc/done/DELTA_PLAN.md, Decided 9):
// rows, the canvas's, of which the screen d is of is rows[top:top+d.Rows].
// It says the canvas's rows and the screen's top after. The canvas ends
// where the screen does: rows past it go, and rows it lacks are blank. A
// history scroll of N is N blank rows appended, the rows Unseen says made
// blank where they are, and the top N rows on: what left the screen stays
// on the canvas, at the rows it was. Any other scroll moves the screen's rows, and
// a span is at top+Row. A whole delta is a canvas of the screen alone, its
// top 0; numbering the canvas's rows (its First) is the caller's. A delta
// that does not fit is ErrDelta, the canvas left as it was.
//
// The canvas given is never written: the canvas said is a new slice of
// rows, sharing those the delta did not write, and a row a span writes is
// a copy, made once. So a canvas may be shared — a snapshot sent by
// reference, a window of it on show — and a delta applied to one holder's
// changes no other's.
func (d Delta) ApplyCanvas(rows [][]Cell, top int) ([][]Cell, int, error) {
	if err := d.check(); err != nil {
		return rows, top, err
	}
	// shared is the rows of the canvas given a span may reach: the screen's,
	// moved within it by a scroll, or left above it by a history scroll (a
	// span past one is on the rows it appended, which are new).
	var shared map[*Cell]bool
	if d.Whole {
		rows, top = blankGrid(d.Cols, d.Rows), 0
	} else {
		if top < 0 || top > len(rows) {
			return rows, top, fmt.Errorf("%w: a screen at row %d of a canvas of %d", ErrDelta, top, len(rows))
		}
		for _, row := range rows[top:min(len(rows), top+d.Rows)] {
			if len(row) != d.Cols {
				return rows, top, fmt.Errorf("%w: a row %d wide on a screen %d wide", ErrDelta, len(row), d.Cols)
			}
		}
		end := min(len(rows), top+d.Rows)
		if len(d.Spans) > 0 && d.Cols > 0 {
			shared = make(map[*Cell]bool, end-top)
			for _, row := range rows[top:end] {
				shared[&row[0]] = true
			}
		}
		out := make([][]Cell, end, top+d.Rows)
		copy(out, rows[:end])
		rows = out
		for len(rows) < top+d.Rows {
			rows = append(rows, BlankRow(d.Cols))
		}
	}
	// own is the row at i, a copy of it first if it is the canvas given's.
	own := func(i int) []Cell {
		row := rows[i]
		if len(row) > 0 && shared[&row[0]] {
			row = append([]Cell(nil), row...)
			rows[i] = row
		}
		return row
	}
	d.each(func(s Scroll) {
		if !s.History {
			scrollRows(rows[top+s.Top:top+s.Bottom+1], s.N, d.Cols, nil)
			return
		}
		// N blank rows appended, at once; the rows leaving that Unseen
		// says, there already, made blank in their place (new rows: the
		// canvas given's are never written).
		for _, r := range s.Unseen {
			for i := top + r.From; i < top+r.From+r.N && i < len(rows); i++ {
				rows[i] = BlankRow(d.Cols)
			}
		}
		for range s.N {
			rows = append(rows, BlankRow(d.Cols))
		}
		top += s.N
	}, func(sp Span) { d.write(own(top+sp.Row), sp) })
	return rows, top, nil
}

// Merge is d and then next as one delta: applied, the same as the two
// applied in turn. Their scrolls and cells are one after the other — d's
// cells before next's scrolls (Span.Before) — and next's runs are said in
// d's table. A whole delta followed by a delta is whole with next's appended;
// a delta followed by a whole is the whole; an empty delta, the other. next
// of another size than d's after, and not whole, is ErrDelta.
func (d Delta) Merge(next Delta) (Delta, error) {
	switch {
	case next.Whole || d.Empty():
		return next, nil
	case next.Empty():
		return d, nil
	case next.Cols != d.Cols || next.Rows != d.Rows:
		return d, fmt.Errorf("%w: a delta of %dx%d after one of %dx%d", ErrDelta, next.Cols, next.Rows, d.Cols, d.Rows)
	}
	out := Delta{Whole: d.Whole, Cols: d.Cols, Rows: d.Rows}
	out.Scrolls = append(append([]Scroll(nil), d.Scrolls...), next.Scrolls...)
	if len(out.Scrolls) == 0 {
		out.Scrolls = nil
	}
	t := newTable(append([]Cell(nil), d.Attrs...))
	for _, sp := range d.Spans {
		sp.Before += len(next.Scrolls)
		out.Spans = append(out.Spans, sp)
	}
	for _, sp := range next.Spans {
		runs := make([]Run, len(sp.Runs))
		for i, r := range sp.Runs {
			base, err := entryOf(next.Attrs, r.A)
			if err != nil {
				return d, fmt.Errorf("%w: %v", ErrDelta, err)
			}
			r.A = t.entry(base)
			runs[i] = r
		}
		sp.Runs = runs
		out.Spans = append(out.Spans, sp)
	}
	if len(t.cells) > 0 {
		out.Attrs = t.cells
	}
	return out, nil
}

// spanGap is how many cells alike may lie between two that differ in a row
// and still be one span: a span's own row and column cost about as much.
const spanGap = 3

// spansOf appends to spans those of row y where next differs from prev (a
// nil prev is a blank row), and copies those cells into prev. A span never
// starts on the second half of a wide glyph (a cell with no text after
// one), so a device can draw it from its start.
func spansOf(spans []Span, y int, prev, next []Cell, t *table) []Span {
	blank := Cell{Data: " "}
	same := func(x int) bool {
		if prev == nil {
			return next[x] == blank
		}
		return x < len(prev) && prev[x] == next[x]
	}
	for x := 0; x < len(next); {
		if same(x) {
			x++
			continue
		}
		start, end := x, x+1
		for k := end; k < len(next) && k-end <= spanGap; k++ {
			if !same(k) {
				end = k + 1
			}
		}
		if start > 0 && next[start].Data == "" && next[start-1].Data != "" {
			start--
		}
		spans = append(spans, Span{Row: y, Col: start, Runs: runsOf(next[start:end], t)})
		if prev != nil {
			copy(prev[start:end], next[start:end])
		}
		x = end
	}
	return spans
}
