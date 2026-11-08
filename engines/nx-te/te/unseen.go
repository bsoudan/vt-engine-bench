package te

import (
	"nx/cell"
)

// unseen.go — a ScrollbackScreen as a shadow (doc/done/DELTA_PLAN.md,
// stage 4): a screen no stream feeds, made by the deltas another screen's
// Deltas cut (Apply), whose history has rows it never saw. A row written
// and scrolled off between two cuts does not ride the scroll (Decided 6):
// the shadow keeps it blank and says it is unseen (Unseen), until the rows
// are asked of the screen that has them and come (Fill). The screen a
// program writes to never has one.

// seqs is the rows of the history from seq from to seq to.
type seqs struct{ from, to uint64 }

// Apply applies a delta another screen's Deltas cut to this one: its
// scrolls, the rows a history scroll moves off going to the history as a
// scroll of the screen's own would take them, those unseen kept blank and
// said so; then its cells. A whole delta of another size is the program
// resizing its own screen (DECCOLM): the screen is made that size first.
// A delta that does not fit is cell.ErrDelta, the screen as it was.
func (h *ScrollbackScreen) Apply(d cell.Delta) error {
	if d.Whole && (d.Cols != h.Columns || d.Rows != h.Lines) && d.Cols > 0 && d.Rows > 0 {
		h.Screen.Resize(d.Rows, d.Cols)
	}
	rows, err := h.applyDelta(h.rows, d)
	if err != nil {
		return err
	}
	h.rows = rows
	return nil
}

// markUnseen says the row at seq, about to be kept, was never seen.
func (h *ScrollbackScreen) markUnseen(seq uint64) {
	if n := len(h.unseen); n > 0 && h.unseen[n-1].to == seq {
		h.unseen[n-1].to++
		return
	}
	h.unseen = append(h.unseen, seqs{seq, seq + 1})
}

// pruneUnseen forgets what was unseen of rows the history no longer holds.
func (h *ScrollbackScreen) pruneUnseen() {
	first := h.scrollback.FirstSeq
	i := 0
	for i < len(h.unseen) && h.unseen[i].to <= first {
		i++
	}
	h.unseen = h.unseen[i:]
	if len(h.unseen) > 0 && h.unseen[0].from < first {
		h.unseen[0].from = first
	}
	if len(h.unseen) == 0 {
		h.unseen = nil
	}
}

// Unseen says the history's row at seq is one this screen never saw: kept
// blank, the screen that has it to be asked for it.
func (h *ScrollbackScreen) Unseen(seq uint64) bool {
	if seq < h.scrollback.FirstSeq {
		return false
	}
	for _, s := range h.unseen {
		if seq >= s.from && seq < s.to {
			return true
		}
	}
	return false
}

// HasUnseen says some row of the history is unseen.
func (h *ScrollbackScreen) HasUnseen() bool {
	h.pruneUnseen()
	return len(h.unseen) > 0
}

// UnseenIn is the first and one past the last of the rows unseen from seq
// from to seq to; false, none is.
func (h *ScrollbackScreen) UnseenIn(from, to uint64) (lo, hi uint64, ok bool) {
	h.pruneUnseen()
	for _, s := range h.unseen {
		a, b := max(s.from, from), min(s.to, to)
		if a >= b {
			continue
		}
		if !ok {
			lo, ok = a, true
		}
		hi = b
	}
	return lo, hi, ok
}

// Fill is the rows asked for, from seq from to seq to, come: rows, the
// history's from seq first, written where they are still held and unseen,
// each fitted to the screen's width; and the rest of those asked for, which
// the screen that had them no longer holds, no longer unseen (kept blank:
// they go from this history too, soon). It says the seqs it wrote, from
// one to one past the last; none, lo == hi.
func (h *ScrollbackScreen) Fill(from, to, first uint64, rows [][]Cell) (lo, hi uint64) {
	held := h.scrollback.FirstSeq
	for i, row := range rows {
		seq := first + uint64(i)
		if seq < from || seq >= to || seq < held || seq >= h.scrollback.TotalAdded || !h.Unseen(seq) {
			continue
		}
		r := [][]gcell{h.packRow(row)}
		fitRowsWidth(r, h.Columns, h.blankCell())
		h.scrollback.Top.items[seq-held] = r[0]
		if lo == hi {
			lo = seq
		}
		hi = seq + 1
	}
	h.seen(from, to)
	return lo, hi
}

// seen says the rows from seq from to seq to are no longer unseen.
func (h *ScrollbackScreen) seen(from, to uint64) {
	var out []seqs
	for _, s := range h.unseen {
		if s.to <= from || s.from >= to {
			out = append(out, s)
			continue
		}
		if s.from < from {
			out = append(out, seqs{s.from, from})
		}
		if s.to > to {
			out = append(out, seqs{to, s.to})
		}
	}
	h.unseen = out
}

// Renumber makes the history run from seq first to seq total, as the screen
// this one shadows says its own does after a resize this one could not
// follow: a row it holds at a seq in that run is kept, if keep says it is
// the same row there; every other is a blank row, unseen.
func (h *ScrollbackScreen) Renumber(first, total uint64, keep bool) {
	if total < first {
		first = total
	}
	if n := uint64(h.scrollback.Top.max); total-first > n {
		first = total - n
	}
	old, oldFirst := h.scrollback.Top.items, h.scrollback.FirstSeq
	held := func(seq uint64) bool {
		return keep && seq >= oldFirst && seq-oldFirst < uint64(len(old)) && !h.Unseen(seq)
	}
	rows := make([][]gcell, 0, total-first)
	var unseen []seqs
	for seq := first; seq < total; seq++ {
		if held(seq) {
			rows = append(rows, old[seq-oldFirst])
			continue
		}
		rows = append(rows, blankLine(h.Columns, h.blankCell()))
		if n := len(unseen); n > 0 && unseen[n-1].to == seq {
			unseen[n-1].to++
		} else {
			unseen = append(unseen, seqs{seq, seq + 1})
		}
	}
	h.scrollback.Top.items = rows
	h.scrollback.FirstSeq, h.scrollback.TotalAdded = first, total
	h.unseen = unseen
}

// ResizeAs resizes the shadow as the screen it shadows was resized: given
// that screen's cursors as they were before, by the same reflow on the same
// cells (Decided 7), and then made to agree with the run its history has
// after (first to total). Where the shadow cannot follow the reflow — a new
// width over a history with rows unseen, whose wraps it does not know, or
// under the alternate screen, whose primary it does not hold — its history
// is that run, every row unseen; under the alternate screen, the rows the
// stashed primary pushed into it are unseen. Not ok: its history came out
// otherwise where it should have been the same, and the shadow is not the
// screen's (its owner asks for the whole).
func (h *ScrollbackScreen) ResizeAs(lines, columns int, c Cursors, first, total uint64) (ok bool) {
	width, alt, gaps := columns != h.Columns, h.altActive, h.HasUnseen()
	h.SetCursors(c)
	h.Resize(lines, columns)
	switch {
	case width && (gaps || alt):
		h.Renumber(first, total, false)
	case h.scrollback.FirstSeq == first && h.scrollback.TotalAdded == total:
	case alt:
		h.Renumber(first, total, true)
	default:
		h.Renumber(first, total, !width)
		return false
	}
	return true
}

// ClearHistory forgets the history, as an erase of the saved lines (ED 3)
// or a reset does: what the screen this one shadows did.
func (h *ScrollbackScreen) ClearHistory() { h.clearScrollback() }

// SetAltScreen marks the alternate screen active or not, as the screen this
// one shadows says, without the swap of buffers SetMode makes: the deltas
// carry the cells either way. Active, the primary screen it would stash is
// one it does not hold, a blank one: a resize under it pushes none of its
// rows into the history (the shadowed screen's are said unseen, Renumber).
func (h *ScrollbackScreen) SetAltScreen(on bool) {
	if on == h.altActive {
		return
	}
	h.altActive = on
	h.altRows, h.altSavepoints, h.altWrapNext, h.altSavedCursor = nil, nil, false, nil
	if on {
		h.altRows = makeBlankCells(h.Lines, h.Columns)
	}
}

// Cursors is where a screen's cursors are: the cursor, its pending wrap,
// and those saved (DECSC). A resize that reflows takes each to where what
// was under it went, and where they are says how the rows re-wrap: a
// shadow resizing as its screen did is given them first.
type Cursors struct {
	Cursor     Cursor      `json:"cursor"`
	WrapNext   bool        `json:"wrap_next,omitempty"`
	Savepoints []Savepoint `json:"savepoints,omitempty"`
}

// Cursors is the screen's cursors as they are.
func (s *Screen) Cursors() Cursors {
	return Cursors{Cursor: s.Cursor, WrapNext: s.wrapNext, Savepoints: copySavepoints(s.Savepoints)}
}

// SetCursors puts the screen's cursors where c says.
func (s *Screen) SetCursors(c Cursors) {
	s.Cursor, s.wrapNext, s.Savepoints = c.Cursor, c.WrapNext, copySavepoints(c.Savepoints)
}
