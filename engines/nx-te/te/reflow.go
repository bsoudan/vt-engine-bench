package te

// A reflow re-wraps what was written at a new width: the history and the
// screen are taken as one list of logical lines — a logical line is a run of
// rows each wrapped into the next (cell.Cell.Wrapped on a row's last cell),
// ended by one that is not — and each is laid out again at the new width.
// ScrollbackScreen.Resize does it when Reflow is set.

// reflowMark is a place on the screen a reflow keeps: the cursor, a saved
// cursor, the screen's top. row is the screen's (it may be past its last
// row, for a saved cursor below what was written); pending is a wrap
// waiting on the next character, the cursor one column past the edge.
type reflowMark struct {
	row, col int
	pending  bool
}

// reflow lays hist and screen out again cols wide, the screen lines high,
// and returns the new history and screen. marks are moved to where what
// was under them went, onto the new screen; marks[0] is the cursor, or nil,
// and nothing written below it or below the last row with anything on it
// is kept: the blank rows there are made again, as many as the screen
// needs. top is where the screen's first row went, in the result: the
// screen starts there, unless what is below it no longer fits, and then it
// is the last lines rows of it, the cursor on it. histMax bounds the
// history, its oldest rows dropped.
func reflow(hist, screen [][]gcell, cols, lines, histMax int, blank gcell, marks []*reflowMark) (newHist, newScreen [][]gcell) {
	all := make([][]gcell, 0, len(hist)+len(screen))
	all = append(all, hist...)
	all = append(all, screen...)
	h := len(hist)

	// last is the last row of what was written: the cursor's, or one with
	// something on it below it.
	last := h - 1
	if len(marks) > 0 && marks[0] != nil {
		last = min(h+marks[0].row, len(all)-1)
	}
	for i := len(all) - 1; i > last && i >= h; i-- {
		if !blankRow(all[i]) {
			last = i
			break
		}
	}

	// Each mark's place in a logical line: which one, and how far into it.
	type place struct{ line, off int }
	top := &reflowMark{} // the screen's first row
	at := make([]place, len(marks)+1)
	below := make([]int, len(marks)+1) // rows past last, for a mark there
	all = all[:last+1]
	every := append([]*reflowMark{top}, marks...)
	for i, m := range every {
		if m == nil {
			continue
		}
		if r := h + m.row; r > last {
			below[i] = r - last
		}
	}

	var logical [][]gcell
	var cur []gcell
	for i, row := range all {
		for k, m := range every {
			if m != nil && h+m.row == i {
				at[k] = place{line: len(logical), off: len(cur) + m.col}
			}
		}
		wrapped := len(row) > 0 && row[len(row)-1].wrapped() && i < last
		n := len(row)
		if wrapped {
			for n > 0 && row[n-1].empty() {
				n-- // space the line never filled
			}
		}
		for _, c := range row[:n] {
			c.flags &^= flagWrapped | flagEmpty
			cur = append(cur, c)
		}
		if !wrapped {
			logical = append(logical, cur)
			cur = nil
		}
	}

	// A line's trailing blanks are not what was written, but for the
	// columns a mark in it is at.
	need := make([]int, len(logical))
	for k, m := range every {
		if m != nil && below[k] == 0 {
			need[at[k].line] = max(need[at[k].line], at[k].off)
		}
	}
	for i, l := range logical {
		n := len(l)
		for n > need[i] && l[n-1].blank() {
			n--
		}
		for n < need[i] {
			l = append(l, blank)
			n = len(l)
		}
		logical[i] = l[:n]
	}

	// Each line again, cols wide; where every cell of it went, and the
	// place just past its end.
	type rc struct{ row, col int }
	var rows [][]gcell
	starts := make([]int, len(logical))
	moved := make([][]rc, len(logical))
	for i, l := range logical {
		starts[i] = len(rows)
		where := make([]rc, len(l)+1)
		var row []gcell
		flush := func(wrapped bool) {
			if wrapped {
				for len(row) < cols {
					c := blank
					c.flags |= flagEmpty
					row = append(row, c)
				}
				row[len(row)-1].flags |= flagWrapped
			}
			rows = append(rows, row)
			row = make([]gcell, 0, cols)
		}
		row = make([]gcell, 0, cols)
		for j := 0; j < len(l); {
			w := 1
			if j+1 < len(l) && l[j].text != 0 && l[j+1].text == 0 && cols >= 2 {
				w = 2 // a wide glyph and the placeholder after it
			}
			if len(row)+w > cols {
				flush(true)
			}
			for k := range w {
				where[j+k] = rc{len(rows) - starts[i], len(row)}
				row = append(row, l[j+k])
			}
			j += w
		}
		where[len(l)] = rc{len(rows) - starts[i], len(row)}
		for len(row) < cols {
			row = append(row, blank)
		}
		rows = append(rows, row)
		moved[i] = where
	}

	// The marks, in rows of the result.
	contentEnd := len(rows)
	abs := make([]int, len(every))
	for k, m := range every {
		if m == nil {
			continue
		}
		if below[k] > 0 {
			abs[k] = contentEnd - 1 + below[k]
			m.col, m.pending = min(m.col, cols-1), false
			continue
		}
		p := moved[at[k].line][at[k].off]
		r, c := starts[at[k].line]+p.row, p.col
		switch {
		case c < cols:
			m.pending = false
		case k > 0: // a cursor past the edge: its wrap waits
			m.pending = true
		default:
			c = cols - 1
		}
		abs[k], m.col = r, c
	}

	start := abs[0]
	if contentEnd-start > lines {
		start = contentEnd - lines
	}
	if len(marks) > 0 && marks[0] != nil {
		start = min(start, abs[1])
	}
	newHist = rows[:start]
	if len(newHist) > histMax {
		newHist = newHist[len(newHist)-histMax:]
	}
	newScreen = make([][]gcell, 0, lines)
	newScreen = append(newScreen, rows[start:min(len(rows), start+lines)]...)
	for len(newScreen) < lines {
		newScreen = append(newScreen, blankLine(cols, blank))
	}
	for k, m := range every[1:] {
		if m != nil {
			m.row = min(max(abs[k+1]-start, 0), lines-1)
		}
	}
	return newHist, newScreen
}
