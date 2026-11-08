package cell

// Differ makes deltas (Delta) by comparison: it holds a grid as its
// receiver has it — the last delta it made, applied — and the delta to a
// next grid is the scrolls it is told, applied to the grid it holds first
// so that they pass on, and then the cells that still differ. It is the one
// place a delta is made by comparing grids: where a layer paints over what
// it was given (a one-plane frontend), and a screen's model (te.Deltas). A
// scroll not told costs rows, never a wrong cell.
//
// The zero Differ holds nothing: its first delta is whole.
type Differ struct {
	grid    [][]Cell // as the receiver has it; nil, nothing
	cols    int
	pending []Scroll // applied to grid, not yet said
}

// Held is the grid as the receiver has it, scrolls told since the last delta
// applied: read-only. Nil before the first delta and after a Reset to nil.
func (d *Differ) Held() [][]Cell { return d.grid }

// Reset says the receiver has grid, as it is now, without a delta: a copy
// is held. Nil, it has nothing, and the next delta is whole.
func (d *Differ) Reset(grid [][]Cell) {
	d.pending = nil
	if grid == nil {
		d.grid, d.cols = nil, 0
		return
	}
	d.grid = make([][]Cell, len(grid))
	d.cols = 0
	for i, row := range grid {
		d.grid[i] = append([]Cell(nil), row...)
		d.cols = len(row)
	}
}

// Scroll applies s to the grid held, now, and says it with the next delta:
// what a screen's hook tells as each scroll happens, so the next scroll's
// rows leaving are judged against the grid as it then stands. A scroll
// outside the grid, or with nothing held, is dropped: the cells say it.
// One that goes on from the one before — the same rows, the same way, the
// same kind — is one scroll with it.
func (d *Differ) Scroll(s Scroll) {
	if d.grid == nil || (Delta{Cols: d.cols, Rows: len(d.grid), Scrolls: []Scroll{s}}).check() != nil {
		return
	}
	recycle(d.grid[s.Top:s.Bottom+1], s.N)
	if n := len(d.pending); n > 0 {
		last := &d.pending[n-1]
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
	d.pending = append(d.pending, s)
}

// recycle is scrollRows on rows the Differ owns: the rows leaving are
// made the blank rows uncovered, not dropped for new ones, so a screen's
// model scrolling a line at a time makes no row.
func recycle(rows [][]Cell, n int) {
	h := len(rows)
	k := min(max(n, -n), h)
	if k == 0 {
		return
	}
	gone := append([][]Cell(nil), rows[:k]...)
	if n > 0 {
		copy(rows, rows[k:])
		copy(rows[h-k:], gone)
	} else {
		gone = append(gone[:0], rows[h-k:]...)
		copy(rows[k:], rows[:h-k])
		copy(rows, gone)
	}
	for _, row := range gone {
		for i := range row {
			row[i] = Cell{Data: " "}
		}
	}
}

// Delta is the delta from the grid held to next: the scrolls told since
// the last (Scroll), then scrolls, both applied to the grid held, then the
// cells of next that still differ. next of another size than the grid held,
// or nothing held, is a whole delta. After it, the grid held is a copy of
// next.
func (d *Differ) Delta(next [][]Cell, scrolls []Scroll) Delta {
	cols := 0
	if len(next) > 0 {
		cols = len(next[0])
	}
	if d.grid == nil || len(d.grid) != len(next) || d.cols != cols || !fits(next, cols, len(next)) {
		d.Reset(next)
		return Whole(next)
	}
	for _, s := range scrolls {
		d.Scroll(s)
	}
	out := Delta{Cols: cols, Rows: len(next)}
	if len(d.pending) > 0 {
		out.Scrolls, d.pending = d.pending, nil
	}
	t := newTable(nil)
	for y, row := range next {
		out.Spans = spansOf(out.Spans, y, d.grid[y], row, t)
	}
	if len(t.cells) > 0 {
		out.Attrs = t.cells
	}
	return out
}
