package te

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"nx/cell"
)

// gmodel is a screen's model for Deltas: the screen as every shadow of it
// has it, kept in the screen's own cells (gcell) so that a cut compares
// eight bytes a cell and unpacks only the cells that differ. It is
// cell.Differ's logic on gcells, and must make the same delta of the same
// change; the delta corpus test checks that a shadow rebuilt from the
// deltas is the screen. The model's cells index the screen's tables, and
// are compacted with them (Screen.moreRows).
type gmodel struct {
	s       *Screen
	grid    [][]gcell // as the receiver has it; nil, nothing
	cols    int
	pending []cell.Scroll // applied to grid, not yet said
	scratch []Cell        // the cells of one span, unpacked for its runs
	blank   []gcell       // a blank row of the grid's width, to copy from
}

// Held is the grid as the receiver has it, as cells: a copy. Nil before the
// first delta and after a Reset to nil.
func (m *gmodel) Held() [][]Cell {
	if m.grid == nil {
		return nil
	}
	return m.s.unpackRows(m.grid)
}

// rows is the grid as stored, for compaction.
func (m *gmodel) rows() [][]gcell { return m.grid }

// Reset says the receiver has rows, as they are now, without a delta: a
// copy is held. Nil, it has nothing, and the next delta is whole.
func (m *gmodel) Reset(rows [][]gcell) {
	m.pending = nil
	if rows == nil {
		m.grid, m.cols = nil, 0
		return
	}
	m.grid = make([][]gcell, len(rows))
	m.cols = 0
	for i, row := range rows {
		m.grid[i] = append([]gcell(nil), row...)
		m.cols = len(row)
	}
	m.blank = blankLine(m.cols, gblank)
}

// Scroll applies s to the grid held, now, and says it with the next delta;
// cell.Differ.Scroll on gcells.
func (m *gmodel) Scroll(s cell.Scroll) {
	if m.grid == nil || s.Check(m.cols, len(m.grid)) != nil {
		return
	}
	recycleRows(m.grid[s.Top:s.Bottom+1], s.N, m.blank)
	if n := len(m.pending); n > 0 {
		last := &m.pending[n-1]
		if last.Top == s.Top && last.Bottom == s.Bottom && last.History == s.History && (last.N > 0) == (s.N > 0) {
			for _, r := range s.Unseen {
				r.From += last.N
				if k := len(last.Unseen); k > 0 && last.Unseen[k-1].From+last.Unseen[k-1].N == r.From {
					last.Unseen[k-1].N += r.N // one run, on from the last
					continue
				}
				last.Unseen = append(last.Unseen, r)
			}
			last.N += s.N
			return
		}
	}
	m.pending = append(m.pending, s)
}

// recycleRows shifts rows by n, the rows leaving made the blank rows
// uncovered, by a copy of blank, rather than dropped for new ones.
func recycleRows(rows [][]gcell, n int, blank []gcell) {
	h := len(rows)
	k := min(max(n, -n), h)
	if k == 0 {
		return
	}
	gone := append([][]gcell(nil), rows[:k]...)
	if n > 0 {
		copy(rows, rows[k:])
		copy(rows[h-k:], gone)
	} else {
		gone = append(gone[:0], rows[h-k:]...)
		copy(rows[k:], rows[:h-k])
		copy(rows, gone)
	}
	for _, row := range gone {
		copy(row, blank)
	}
}

// fitsRows says whether grid is cols×rows.
func fitsRows(grid [][]gcell, cols, rows int) bool {
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

// Delta is the delta from the grid held to next: the scrolls told since
// the last, then the cells of next that still differ. next of another size
// than the grid held, or nothing held, is a whole delta. After it, the grid
// held is a copy of next.
func (m *gmodel) Delta(next [][]gcell) cell.Delta {
	cols := 0
	if len(next) > 0 {
		cols = len(next[0])
	}
	if m.grid == nil || len(m.grid) != len(next) || m.cols != cols || !fitsRows(next, cols, len(next)) {
		m.Reset(next)
		return m.whole(next)
	}
	out := cell.Delta{Cols: cols, Rows: len(next)}
	if len(m.pending) > 0 {
		out.Scrolls, m.pending = m.pending, nil
	}
	t := cell.NewTable()
	for y, row := range next {
		out.Spans = m.spansOf(out.Spans, y, m.grid[y], row, t)
	}
	if cells := t.Cells(); len(cells) > 0 {
		out.Attrs = cells
	}
	return out
}

// whole is a whole delta of next: the cells of it that are not blank
// (cell.Whole).
func (m *gmodel) whole(next [][]gcell) cell.Delta {
	cols := 0
	for _, row := range next {
		cols = max(cols, len(row))
	}
	d := cell.Delta{Whole: true, Cols: cols, Rows: len(next)}
	t := cell.NewTable()
	for y, row := range next {
		d.Spans = m.spansOf(d.Spans, y, nil, row, t)
	}
	d.Attrs = t.Cells()
	return d
}

// spansOf appends to spans those of row y where next differs from prev (a
// nil prev is a blank row), and copies those cells into prev; cell's
// spansOf on gcells. Two gcells are equal exactly when their cells are,
// the tables holding each text and attribute once. A span never starts on
// the second half of a wide glyph (a cell with no text after one), so a
// device can draw it from its start.
func (m *gmodel) spansOf(spans []cell.Span, y int, prev, next []gcell, t *cell.Table) []cell.Span {
	same := func(x int) bool {
		if prev == nil {
			return next[x] == gblank
		}
		return x < len(prev) && prev[x] == next[x]
	}
	for x := 0; x < len(next); {
		if same(x) {
			x++
			continue
		}
		start, end := x, x+1
		for k := end; k < len(next) && k-end <= cell.SpanGap; k++ {
			if !same(k) {
				end = k + 1
			}
		}
		if start > 0 && next[start].text == 0 && next[start-1].text != 0 {
			start--
		}
		m.scratch = m.s.unpackRow(next[start:end], m.scratch)
		spans = append(spans, cell.Span{Row: y, Col: start, Runs: t.Runs(m.scratch)})
		if prev != nil {
			copy(prev[start:end], next[start:end])
		}
		x = end
	}
	return spans
}

// applyDelta is cell.Delta.Apply on the screen's own cells: d applied to
// grid, which is as the delta before left it, and the grid after (a whole
// delta a new one, else grid itself, its rows moved and written in place).
// The rows a history scroll moves off the top go to the history, oldest
// first, those its Unseen says as blank rows said unseen. A delta that does
// not fit grid is cell.ErrDelta, and grid is left as it was.
func (h *ScrollbackScreen) applyDelta(grid [][]gcell, d cell.Delta) ([][]gcell, error) {
	if err := d.Check(); err != nil {
		return grid, err
	}
	if d.Whole {
		grid = makeBlankCells(d.Rows, d.Cols)
	} else if !fitsRows(grid, d.Cols, d.Rows) {
		return grid, fmt.Errorf("%w: a delta of %dx%d on a grid of %d rows", cell.ErrDelta, d.Cols, d.Rows, len(grid))
	}
	d.Each(func(s cell.Scroll) {
		var keep func(i int, row []gcell) []gcell
		if s.History {
			keep = func(i int, row []gcell) []gcell {
				unseen := s.IsUnseen(i)
				if row == nil || unseen {
					row = blankLine(d.Cols, gblank)
				}
				if unseen {
					h.markUnseen(h.scrollback.TotalAdded)
				}
				return h.keepScrolledOff(row) // the row is the history's; its evicted one comes back
			}
		}
		scrollGrid(grid[s.Top:s.Bottom+1], s.N, d.Cols, keep)
	}, func(sp cell.Span) { h.writeSpan(grid[sp.Row], sp, d) })
	return grid, nil
}

// scrollGrid shifts rows (a scroll's region) by n, the rows uncovered
// blank, cols wide. What leaves the top of an upward shift goes to keep,
// oldest first, nil for each row past the region's height, and keep may
// hand back a row of the right width to reuse; without keep, the rows
// leaving are reused as the blank rows uncovered.
func scrollGrid(rows [][]gcell, n, cols int, keep func(i int, row []gcell) []gcell) {
	h := len(rows)
	var spare [][]gcell
	blank := func() []gcell {
		for len(spare) > 0 {
			row := spare[len(spare)-1]
			spare = spare[:len(spare)-1]
			if len(row) == cols {
				for i := range row {
					row[i] = gblank
				}
				return row
			}
		}
		return blankLine(cols, gblank)
	}
	switch {
	case n > 0:
		k := min(n, h)
		if keep == nil {
			spare = append(spare, rows[:k]...)
		} else {
			for i := range n {
				var row []gcell
				if i < h {
					row = rows[i]
				}
				if s := keep(i, row); s != nil {
					spare = append(spare, s)
				}
			}
		}
		copy(rows, rows[k:])
		for i := h - k; i < h; i++ {
			rows[i] = blank()
		}
	case n < 0:
		k := min(-n, h)
		spare = append(spare, rows[h-k:]...)
		copy(rows[k:], rows[:h-k])
		for i := range k {
			rows[i] = blank()
		}
	}
}

// writeSpan puts sp's cells in row, each run's entry packed once.
func (h *ScrollbackScreen) writeSpan(row []gcell, sp cell.Span, d cell.Delta) {
	x := sp.Col
	for _, r := range sp.Runs {
		base, _ := d.Entry(r.A) // checked
		g := h.pack(base)       // no text: the run's entry as a cell
		switch {
		case r.T != "":
			for _, ch := range r.T {
				c := g
				if ch == utf8.RuneError {
					ch = '�' // as a conversion reads it, whatever bytes it was
				}
				c.text = uint32(ch)
				row[x] = c
				x++
			}
		case r.C != nil:
			for _, data := range r.C {
				c := h.cellOf(data, g.style)
				c.flags |= g.flags
				row[x] = c
				x++
			}
		default:
			for range r.N {
				row[x] = g
				x++
			}
		}
	}
}

// sameRow says whether the screen row and the model row are the same cells.
func sameRow(a, b []gcell) bool { return slices.Equal(a, b) }
