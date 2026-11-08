package te

import (
	"slices"

	"nx/cell"
)

// Deltas cuts a screen's deltas (cell.Delta; doc/done/DELTA_PLAN.md): what
// changed on it since the last cut, as the scrolls that happened and then
// the cells that differ. It keeps a model, the screen as every shadow of it
// has it — the last delta applied — in rows of its own, and the screen's
// hook (Screen.Scrolled) advances the model as each full-width scroll
// happens: so the scrolls pass on as scrolls, and a row a history scroll
// pushes off is judged against the model as it then stands: one that is
// not what the shadows have — a row written and scrolled off between two
// cuts — is said unseen (cell.Scroll.Unseen), and does not ride the scroll
// (Decided 6): a shadow keeps it blank, and asks for it if it is wanted. A
// cut compares the model with the screen for the cells, the one comparison
// (cell.Differ); the model is the screen after.
//
// The model is compared, not tracked (Decided 5): what the screen writes
// in place is never marked, so no write is missed; only that rows moved
// comes from the hook, and a move it does not tell costs rows, never a
// wrong cell. A screen whose size is not the model's — a resize not told
// (Reset), a program's DECCOLM — is cut whole, the rows its history took
// since the last cut said unseen at its top, so a shadow's history counts
// them.
//
// One owner, as the screen has: Deltas is not safe for concurrent use.
type Deltas struct {
	screen *Screen
	model  cell.Differ
	// kept is the rows the screen's history took since the last cut, as
	// its history scrolls told them.
	kept int
}

// NewDeltas cuts s's deltas: it takes s's Scrolled hook. Its first cut is
// whole.
func NewDeltas(s *Screen) *Deltas {
	d := &Deltas{screen: s}
	s.Scrolled = d.scrolled
	return d
}

// Cut is the delta since the last cut, or the whole screen for the first;
// the model is the screen after it. Empty (cell.Delta.Empty) when nothing
// changed. A whole one begins with a history scroll of the rows the
// history took since the last cut, every one unseen.
func (d *Deltas) Cut() cell.Delta {
	out := d.model.Delta(d.screen.Buffer, nil)
	if out.Whole && d.kept > 0 && out.Rows > 0 {
		s := cell.Scroll{Bottom: out.Rows - 1, N: d.kept, History: true, Unseen: []cell.Rows{{N: d.kept}}}
		out.Scrolls = append([]cell.Scroll{s}, out.Scrolls...)
	}
	d.kept = 0
	return out
}

// Reset says every shadow has the screen as it is now, without a delta:
// after a resize, which each shadow applies itself, by the same reflow on
// the same cells (Decided 7). Cut what came before first.
func (d *Deltas) Reset() {
	d.model.Reset(d.screen.Buffer)
	d.kept = 0
}

// Whole says no shadow can be trusted to have the screen as it is now:
// the next cut is the screen whole.
func (d *Deltas) Whole() {
	d.model.Reset(nil)
	d.kept = 0
}

// Resized says the screen was resized, what came before cut: a shadow
// resizing itself as it was (ScrollbackScreen.ResizeAs) has the screen
// after it by the same reflow (Reset), but under the alternate screen,
// whose primary a shadow does not hold, or at a new width where its
// history may have rows unseen, whose wraps it does not know: then the
// next cut is the screen whole.
func (d *Deltas) Resized(width, alt, unseen bool) {
	if alt || width && unseen {
		d.Whole()
		return
	}
	d.Reset()
}

// scrolled is the screen's hook: a scroll about to happen, applied to the
// model now. The rows leaving a history scroll are the screen's top rows
// as they are now; each is said unseen where the model's differs.
func (d *Deltas) scrolled(top, bottom, n int, history bool) {
	if history {
		d.kept += n
	}
	held := d.model.Held()
	if len(held) != d.screen.Lines || len(held) == 0 || len(held[0]) != d.screen.Columns {
		return // a size the model is not: the cut is whole
	}
	var unseen []cell.Rows
	if history {
		for i := range n {
			if slices.Equal(d.screen.Buffer[top+i], held[top+i]) {
				continue
			}
			if k := len(unseen); k > 0 && unseen[k-1].From+unseen[k-1].N == i {
				unseen[k-1].N++
			} else {
				unseen = append(unseen, cell.Rows{From: i, N: 1})
			}
		}
	}
	d.model.Scroll(cell.Scroll{Top: top, Bottom: bottom, N: n, History: history, Unseen: unseen})
}
