package te

type scrollbackDeque struct {
	items [][]gcell
	max   int
}

// append adds line to the end. Returns true if an existing row was evicted
// from the front because the deque was at capacity — callers that track
// seq numbers (Top deque in ScrollbackScreen) bump FirstSeq on eviction.
func (d *scrollbackDeque) append(line []gcell) (evicted []gcell, ok bool) {
	if d.max == 0 {
		return line, false // not kept: the caller may reuse it
	}
	if len(d.items) == d.max {
		evicted, ok = d.items[0], true
		d.items[0] = nil
		d.items = d.items[1:]
	}
	d.items = append(d.items, line) // the row is the deque's now
	return evicted, ok
}

// Scrollback is the rows scrolled off the top of the screen, oldest first.
//
// Seq-space model: each row ever appended to Top gets a monotonically
// increasing seq number. TotalAdded is the exclusive upper bound ("next
// seq"), so a just-pushed row has seq = TotalAdded - 1. FirstSeq is the seq
// of Top.items[0] — the oldest retained row. Top.items covers exactly the
// seq range [FirstSeq, FirstSeq + len(Top.items)), and TotalAdded - FirstSeq
// == len(Top.items).
type Scrollback struct {
	Top scrollbackDeque
	// TotalAdded is the exclusive upper bound of the seq space: it
	// monotonically increases by one on each row scrolled off the
	// visible screen into Top.
	TotalAdded uint64
	// FirstSeq is the seq of Top.items[0]. When Top is empty, FirstSeq
	// equals TotalAdded.
	FirstSeq uint64
}

// ScrollbackScreen is a Screen that keeps what scrolls off its top: every
// scroll of the whole primary screen, however it was reached (Screen.Index,
// a line wrapping on the bottom row inside Screen.Draw), hands the row
// that leaves to keepScrolledOff (Screen.scrolledOff). The screen always
// stays live, never paged: a view that shows the scrollback reads it
// (ScrollbackRows) and draws it itself. What differs from Screen is a
// reset and an erase of the saved lines (ED 3), which clear the scrollback
// too, and a resize, which keeps what leaves the screen in it, or reflows
// it (Reflow).
type ScrollbackScreen struct {
	*Screen
	scrollback Scrollback
	// Reflow says a resize to a new width re-wraps what was written, the
	// history and the primary screen, rather than padding or cutting each
	// row (reflow.go). The screen a program writes to sets it, and so does
	// a view's shadow of it (vtappview), so the same resize in its journal
	// re-wraps the same rows.
	Reflow bool
	// Cleared, if set, is told the history was just forgotten — an erase of
	// the saved lines (ED 3), a reset — so a shadow's can be too, at the
	// same point among the deltas (doc/done/DELTA_PLAN.md, stage 4). It
	// runs mid-feed.
	Cleared func()
	// unseen is the history's rows a shadow never saw (unseen.go), in
	// order; none for a screen a program writes to.
	unseen []seqs
}

// NewScrollbackScreen creates a ScrollbackScreen keeping up to history rows of
// scrollback.
func NewScrollbackScreen(cols, lines, history int) *ScrollbackScreen {
	h := &ScrollbackScreen{scrollback: Scrollback{Top: scrollbackDeque{max: history}}}
	h.Screen = newScreen(cols, lines)
	h.Screen.scrolledOff = h.keepScrolledOff
	h.Screen.moreRows = append(h.Screen.moreRows, h.historyRows)
	if screenMade != nil {
		screenMade(h.Screen, h)
	}
	return h
}

// Reset resets the screen and clears the scrollback.
func (h *ScrollbackScreen) Reset() {
	h.Screen.Reset()
	h.clearScrollback()
}

// EraseInDisplay is the Screen's, and erasing the saved lines (ED 3, what
// clear(1) sends) clears the scrollback too.
func (h *ScrollbackScreen) EraseInDisplay(how int, private bool, rest ...int) {
	h.Screen.EraseInDisplay(how, private, rest...)
	if how == 3 {
		h.clearScrollback()
	}
}

// clearScrollback forgets the scrollback; the seqs go on from TotalAdded.
func (h *ScrollbackScreen) clearScrollback() {
	h.scrollback.Top.items = nil
	h.scrollback.FirstSeq = h.scrollback.TotalAdded // empty: FirstSeq collapses to TotalAdded
	h.unseen = nil
	if h.Cleared != nil {
		h.Cleared()
	}
}

// keepScrolledOff keeps a row the screen scrolled off its top in the history,
// a seq of its own. The Screen calls it (Screen.scrolledOff) for every scroll
// of the whole primary screen, however the scroll was reached: a line
// wrapping on the bottom row scrolls inside Screen.Draw.
func (h *ScrollbackScreen) keepScrolledOff(row []gcell) []gcell {
	spare, evicted := h.scrollback.Top.append(row)
	if evicted {
		h.scrollback.FirstSeq++
	}
	h.scrollback.TotalAdded++
	return spare
}

// ScrollbackRows returns the retained scrolled-off rows, oldest first — the
// content above the live screen — as cells, a copy the caller owns.
// TotalAdded()/Scrollback() give the seq anchor and count for view-level
// scrollback that pins its offset so live output doesn't drift the viewport.
func (h *ScrollbackScreen) ScrollbackRows() [][]Cell { return h.unpackRows(h.scrollback.Top.items) }

// historyRows is the history's rows as stored, for Screen.moreRows: they
// index the screen's tables too.
func (h *ScrollbackScreen) historyRows() [][]gcell { return h.scrollback.Top.items }

// Scrollback returns the number of lines in the scrollback buffer.
func (h *ScrollbackScreen) Scrollback() int {
	return len(h.scrollback.Top.items)
}

// TotalAdded returns the exclusive upper bound of the scrollback seq
// space: the seq of the next row to be pushed, equivalently
// (seq of newest retained row) + 1 when the retained range is
// contiguous with the upper bound.
func (h *ScrollbackScreen) TotalAdded() uint64 {
	return h.scrollback.TotalAdded
}

// FirstSeq returns the seq of Top.items[0] — the oldest retained
// scrollback row. When Top is empty, FirstSeq == TotalAdded.
func (h *ScrollbackScreen) FirstSeq() uint64 {
	return h.scrollback.FirstSeq
}

// Resize adjusts the screen to the new dimensions, preserving scrollback
// history. When shrinking vertically, rows that no longer fit on-screen
// scroll off the top into history.Top (newest-last), advancing
// TotalAdded and FirstSeq as if each row had been scrolled off via
// LineFeed. When growing, blank rows are appended at the bottom. Column
// changes truncate or pad each visible row.
//
// Without Reflow, wrapped lines are not reflowed: a row wider than the new
// width gets truncated (each row's Wrapped mark kept, as fitRowsWidth
// keeps it). With Reflow, a change of width re-wraps the history and the primary screen
// instead (reflowResize).
//
// The embedded Screen.Resize() that this type would otherwise inherit
// calls DeleteLines on shrink, which discards rows — correct for a
// plain Screen but wrong for ScrollbackScreen: scrolled-off content must
// land in scrollback, not be thrown away.
func (h *ScrollbackScreen) Resize(lines, columns int) {
	if lines <= 0 {
		lines = 1
	}
	if columns <= 0 {
		columns = 1
	}
	if h.rows == nil {
		h.Screen.Resize(lines, columns)
		return
	}
	if h.Lines == lines && h.Columns == columns {
		return
	}
	if h.Reflow && columns != h.Columns {
		h.reflowResize(lines, columns)
		return
	}

	// Shrink vertically. Blank rows below the cursor are trimmed from the
	// bottom first (xterm/tmux behavior) — pushing top rows into history
	// instead would hide visible content behind the scrollback on every
	// shrink (e.g. a child's first output printed before an initial resize
	// lands). Only the remainder scrolls into history, in oldest-first
	// order so seqs remain monotonic.
	if lines < h.Lines {
		drop := h.Lines - lines
		for drop > 0 {
			last := len(h.rows) - 1
			if last <= h.Cursor.Row || !blankRow(h.rows[last]) {
				break
			}
			h.rows = h.rows[:last]
			drop--
		}
		for i := 0; i < drop; i++ {
			if _, evicted := h.scrollback.Top.append(h.rows[i]); evicted {
				h.scrollback.FirstSeq++
			}
			h.scrollback.TotalAdded++
		}
		h.rows = h.rows[drop:]
		if h.Cursor.Row >= drop {
			h.Cursor.Row -= drop
		} else {
			h.Cursor.Row = 0
		}
	}

	// Column changes: truncate or pad each remaining row.
	fitRowsWidth(h.rows, columns, h.blankCell())

	// Grow vertically: append blank rows at the bottom.
	if lines > h.Lines {
		for i := 0; i < lines-h.Lines; i++ {
			h.rows = append(h.rows, blankLine(columns, h.blankCell()))
		}
	}

	h.resizeInactiveBuffer(lines, columns)

	h.Lines = lines
	h.Columns = columns
	h.SetMargins(0, 0)
	h.leftMargin = 0
	if h.Cursor.Col >= columns {
		h.Cursor.Col = columns - 1
	}
	h.wrapNext = false
}

// resizeInactiveBuffer keeps the buffer stashed in altBuffer in step with
// the new dimensions. enter/exitAltScreen swap it in verbatim, so a
// stale-sized buffer under the new Lines/Columns makes Display index out of
// range on the next snapshot. While the alt screen is active the stashed
// buffer is the primary screen: its overflow rows scroll into history
// exactly like a live shrink (blank rows below the saved cursor trimmed
// first), and the saved cursor shifts with them. When the primary screen is
// active the stashed alt buffer is simply recreated blank, matching
// Screen.Resize — alt-screen programs repaint on resize anyway.
func (h *ScrollbackScreen) resizeInactiveBuffer(lines, columns int) {
	if !h.altActive {
		if h.altRows != nil {
			h.altRows = makeBlankCells(lines, columns)
			h.altWrapNext = false
		}
		return
	}

	if lines < len(h.altRows) {
		drop := len(h.altRows) - lines
		savedRow := 0
		if h.altSavedCursor != nil {
			savedRow = h.altSavedCursor.Row
		}
		for drop > 0 {
			last := len(h.altRows) - 1
			if last <= savedRow || !blankRow(h.altRows[last]) {
				break
			}
			h.altRows = h.altRows[:last]
			drop--
		}
		for i := 0; i < drop; i++ {
			if _, evicted := h.scrollback.Top.append(h.altRows[i]); evicted {
				h.scrollback.FirstSeq++
			}
			h.scrollback.TotalAdded++
		}
		h.altRows = h.altRows[drop:]
		if h.altSavedCursor != nil {
			if h.altSavedCursor.Row >= drop {
				h.altSavedCursor.Row -= drop
			} else {
				h.altSavedCursor.Row = 0
			}
		}
	}

	fitRowsWidth(h.altRows, columns, h.blankCell())
	for len(h.altRows) < lines {
		h.altRows = append(h.altRows, blankLine(columns, h.blankCell()))
	}
	h.altWrapNext = false
}

// reflowResize is Resize to a new width with Reflow: the history and the
// primary screen are re-wrapped at it (reflow), the cursor and the saved
// cursors kept where what was under them went. The reflowed history is new
// rows, numbered on from TotalAdded, so a seq only grows and none names a
// different row after. Under the alt screen the primary screen stashed
// there is reflowed with the cursor saved on entering it; the alt screen
// itself is fitted, not reflowed: a full-screen program repaints.
func (h *ScrollbackScreen) reflowResize(lines, columns int) {
	blank := h.blankCell()

	primary, cursor, saves, wrapNext := h.rows, &h.Cursor, h.Savepoints, h.wrapNext
	if h.altActive {
		primary, cursor, saves, wrapNext = h.altRows, h.altSavedCursor, h.altSavepoints, h.altWrapNext
	}
	var marks []*reflowMark
	if cursor != nil {
		marks = append(marks, &reflowMark{row: cursor.Row, col: cursor.Col, pending: wrapNext})
	} else {
		marks = append(marks, nil)
	}
	for _, sp := range saves {
		marks = append(marks, &reflowMark{row: sp.Cursor.Row, col: sp.Cursor.Col, pending: sp.WrapNext})
	}

	hist, screen := reflow(h.scrollback.Top.items, primary, columns, lines, h.scrollback.Top.max, blank, marks)

	if m := marks[0]; m != nil {
		cursor.Row, cursor.Col, wrapNext = m.row, m.col, m.pending
	}
	for i := range saves {
		m := marks[i+1]
		saves[i].Cursor.Row, saves[i].Cursor.Col, saves[i].WrapNext = m.row, m.col, m.pending
	}

	total := h.scrollback.TotalAdded
	h.scrollback.Top.items = hist
	h.scrollback.FirstSeq = total
	h.scrollback.TotalAdded = total + uint64(len(hist))

	if h.altActive {
		h.altRows, h.altWrapNext = screen, wrapNext
		if h.Cursor.Row >= lines { // the alt screen: its top goes, so the cursor stays on it
			h.rows = h.rows[h.Cursor.Row-lines+1:]
			h.Cursor.Row = lines - 1
		}
		if len(h.rows) > lines {
			h.rows = h.rows[:lines]
		}
		fitRowsWidth(h.rows, columns, blank)
		for len(h.rows) < lines {
			h.rows = append(h.rows, blankLine(columns, blank))
		}
		h.Cursor.Col = min(h.Cursor.Col, columns-1)
		h.wrapNext = false
	} else {
		h.rows, h.wrapNext = screen, wrapNext
		if h.altRows != nil {
			h.altRows = makeBlankCells(lines, columns)
			h.altWrapNext = false
		}
	}

	h.Lines = lines
	h.Columns = columns
	h.Screen.SetMargins(0, 0)
	h.leftMargin, h.rightMargin = 0, columns-1
}
