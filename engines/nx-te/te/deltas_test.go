package te

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"nx/cell"
)

// fed is a screen with history, as a vtapp's, its deltas cut, and its stream.
func cutting(cols, rows, history int) (*ScrollbackScreen, *Stream, *Deltas) {
	s := NewScrollbackScreen(cols, rows, history)
	s.Reflow = true
	s.WriteProcessInput = func(string) {}
	d := NewDeltas(s.Screen)
	d.Cut() // the first, whole: what follows is what changed
	return s, NewStream(s), d
}

// numbered is n lines, "L<i>" each from first.
func numbered(first, n int) string {
	var b strings.Builder
	for i := first; i < first+n; i++ {
		fmt.Fprintf(&b, "L%d\r\n", i)
	}
	return b.String()
}

// spannedRows is the rows a delta's spans are on.
func spannedRows(d cell.Delta) []int {
	var rows []int
	for _, sp := range d.Spans {
		if !slices.Contains(rows, sp.Row) {
			rows = append(rows, sp.Row)
		}
	}
	return rows
}

// Each full-width scroll a screen makes is told, as it happens, and is in
// the delta as a scroll; the cells are only the rows that came in. A scroll
// inside left and right margins is not told: its cells say it.
func TestEveryScrollIsADeltasScroll(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		before  string // written, then cut
		bytes   string
		scrolls []cell.Scroll
		rows    []int // the rows with cells
	}{
		{"a line feed at the foot", "\x1b[6;1H", "\nnew", []cell.Scroll{{Bottom: 5, N: 1, History: true}}, []int{5}},
		{"a line wrapping at the foot", "\x1b[6;1H", strings.Repeat("w", 12), []cell.Scroll{{Bottom: 5, N: 1, History: true}}, []int{4, 5}},
		{"a next line", "\x1b[6;1H", "\x1bEnew", []cell.Scroll{{Bottom: 5, N: 1, History: true}}, []int{5}},
		{"an index in a region", "\x1b[2;4r\x1b[4;1H", "\x1bDnew", []cell.Scroll{{Top: 1, Bottom: 3, N: 1}}, []int{3}},
		{"a reverse index", "\x1b[1;1H", "\x1bMnew", []cell.Scroll{{Bottom: 5, N: -1}}, []int{0}},
		{"a scroll up", "", "\x1b[2S", []cell.Scroll{{Bottom: 5, N: 2}}, nil},
		{"a scroll down in a region", "\x1b[3;5r", "\x1b[3T", []cell.Scroll{{Top: 2, Bottom: 4, N: -3}}, nil},
		{"lines inserted", "\x1b[2;1H", "\x1b[2L", []cell.Scroll{{Top: 1, Bottom: 5, N: -2}}, nil},
		{"lines deleted in a region", "\x1b[2;5r\x1b[3;1H", "\x1b[M", []cell.Scroll{{Top: 2, Bottom: 4, N: 1}}, nil},
		{"two feeds, one scroll", "\x1b[6;1H", "\na\nb", []cell.Scroll{{Bottom: 5, N: 2, History: true}}, []int{4, 5}},
		{"in the alt screen, none kept", "\x1b[?1049h\x1b[6;1H", "\nnew", []cell.Scroll{{Bottom: 5, N: 1}}, []int{5}},
		{"inside left and right margins, cells", "\x1b[?69h\x1b[2;5s\x1b[6;3H", "\n", nil, []int{0, 1, 2, 3, 4, 5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, st, d := cutting(10, 6, 20)
			for i := range 6 {
				st.FeedBytes(fmt.Appendf(nil, "\x1b[%d;1Hrow%d", i+1, i))
			}
			st.FeedBytes([]byte(tc.before))
			before := d.Cut()
			st.FeedBytes([]byte(tc.bytes))
			delta := d.Cut()
			if !reflect.DeepEqual(delta.Scrolls, tc.scrolls) {
				t.Errorf("scrolls %+v, want %+v", delta.Scrolls, tc.scrolls)
			}
			if got := spannedRows(delta); !reflect.DeepEqual(got, tc.rows) {
				t.Errorf("cells on rows %v, want %v", got, tc.rows)
			}
			if !reflect.DeepEqual(d.model.Held(), s.Rows()) {
				t.Errorf("the model is not the screen after the cut")
			}
			_ = before
		})
	}
}

// A cat: lines scrolling through the screen are one history scroll and the
// rows that came in, and the rows that left are not carried — every shadow
// has them, written deltas ago.
func TestACatIsAHistoryScrollAndTheNewRows(t *testing.T) {
	t.Parallel()
	_, st, d := cutting(20, 6, 100)
	st.FeedBytes([]byte(numbered(0, 6)))
	d.Cut()
	st.FeedBytes([]byte(numbered(6, 3)))
	delta := d.Cut()
	if want := []cell.Scroll{{Bottom: 5, N: 3, History: true}}; !reflect.DeepEqual(delta.Scrolls, want) {
		t.Fatalf("scrolls %+v, want %+v", delta.Scrolls, want)
	}
	if got := spannedRows(delta); !reflect.DeepEqual(got, []int{2, 3, 4}) {
		t.Fatalf("cells on rows %v, want the three that came in", got)
	}
}

// A burst of more rows than the screen within one cut says the rows that
// were written and scrolled off unseen (Decided 6), and carries none of
// them; a grid with a history the deltas are applied to keeps them blank,
// and the rest as the screen does.
func TestABurstSaysTheRowsNoDeltaShowed(t *testing.T) {
	t.Parallel()
	s, st, d := cutting(20, 6, 100)
	var grid [][]Cell
	var hist [][]Cell
	apply := func(delta cell.Delta) {
		t.Helper()
		var err error
		if grid, err = delta.Apply(grid, func(r []Cell, _ bool) { hist = append(hist, r) }); err != nil {
			t.Fatal(err)
		}
	}
	d.Reset()
	grid = cloneRows(s.Rows())
	st.FeedBytes([]byte(numbered(0, 3)))
	apply(d.Cut())
	st.FeedBytes([]byte(numbered(3, 20))) // 23 lines and the cursor's row: 18 scrolls
	delta := d.Cut()
	if len(delta.Scrolls) != 1 || delta.Scrolls[0].N != 18 {
		t.Fatalf("scrolls %+v, want one of 18", delta.Scrolls)
	}
	// The first six rows leaving are the screen the last delta left —
	// three lines and three rows then blank, written since (3, 4, 5) —
	// and every row after them is one no delta showed.
	if want := []cell.Rows{{From: 3, N: 15}}; !reflect.DeepEqual(delta.Scrolls[0].Unseen, want) {
		t.Fatalf("rows unseen %+v, want %+v", delta.Scrolls[0].Unseen, want)
	}
	if b, _ := delta.MarshalBinary(); len(b) > 400 {
		t.Fatalf("the burst's delta is %d bytes: the rows unseen ride it", len(b))
	}
	apply(delta)
	if !reflect.DeepEqual(grid, s.Rows()) {
		t.Fatalf("the grid is not the screen:\n%s", showRows(grid))
	}
	want := cloneRows(s.ScrollbackRows())
	for i := 3; i < 18; i++ {
		want[i] = cell.BlankRow(20)
	}
	if !reflect.DeepEqual(hist, want) {
		t.Fatalf("the history kept\n%swant\n%s", showRows(hist), showRows(want))
	}
}

// A shadow that applies the deltas (ScrollbackScreen.Apply) says the rows
// it never saw, and is given them when it asks (Fill): its history is then
// the screen's.
func TestAShadowSaysItsRowsUnseenAndIsFilled(t *testing.T) {
	t.Parallel()
	s, st, d := cutting(20, 6, 100)
	sh := NewScrollbackScreen(20, 6, 100)
	d.Reset()
	st.FeedBytes([]byte(numbered(0, 3)))
	if err := sh.Apply(d.Cut()); err != nil {
		t.Fatal(err)
	}
	st.FeedBytes([]byte(numbered(3, 20)))
	if err := sh.Apply(d.Cut()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sh.Rows(), s.Rows()) || sh.TotalAdded() != s.TotalAdded() {
		t.Fatalf("the shadow's screen\n%s, its history to %d; want\n%s, %d", showRows(sh.Rows()), sh.TotalAdded(), showRows(s.Rows()), s.TotalAdded())
	}
	lo, hi, ok := sh.UnseenIn(0, 100)
	if !ok || lo != 3 || hi != 18 || !sh.HasUnseen() || sh.Unseen(2) || !sh.Unseen(3) {
		t.Fatalf("unseen %d..%d (%v), want 3..18", lo, hi, ok)
	}
	// Asked 0..20, given 5..18: 3 and 4 the screen no longer has, and
	// are no longer unseen; 5..17 written.
	if lo, hi := sh.Fill(0, 20, 5, s.ScrollbackRows()[5:18]); lo != 5 || hi != 18 {
		t.Fatalf("filled %d..%d, want 5..18", lo, hi)
	}
	if sh.HasUnseen() {
		lo, hi, _ := sh.UnseenIn(0, 100)
		t.Fatalf("still unseen %d..%d", lo, hi)
	}
	want := cloneRows(s.ScrollbackRows())
	want[3], want[4] = cell.BlankRow(20), cell.BlankRow(20)
	if !reflect.DeepEqual(sh.ScrollbackRows(), want) {
		t.Fatalf("the shadow's history\n%swant\n%s", showRows(sh.ScrollbackRows()), showRows(want))
	}
	// Renumbered as the screen's own after a resize it could not follow:
	// the rows at the same seqs kept, the rest unseen.
	sh.Renumber(10, 25, true)
	if sh.FirstSeq() != 10 || sh.TotalAdded() != 25 || sh.Unseen(17) || !sh.Unseen(18) || !sh.Unseen(24) {
		t.Fatalf("renumbered %d..%d, 17 unseen %v, 18 %v", sh.FirstSeq(), sh.TotalAdded(), sh.Unseen(17), sh.Unseen(18))
	}
	sh.Renumber(10, 25, false)
	if lo, hi, _ := sh.UnseenIn(0, 100); lo != 10 || hi != 25 {
		t.Fatalf("renumbered with nothing kept, unseen %d..%d", lo, hi)
	}
	sh.ClearHistory()
	if sh.HasUnseen() || sh.Scrollback() != 0 {
		t.Fatal("cleared, the history still has rows")
	}
}

// A step is output, a resize, or a cut.
type deltaStep struct {
	bytes      string
	rows, cols int // a resize, when not zero
	cut        bool
}

// view is what a vtapp's deltas are applied to, standing in for its view:
// a ScrollbackScreen with no stream (Decided 8) whose rows are the deltas',
// resized by the same reflow as the vtapp's screen, and a canvas
// (Decided 9) the view's frontend would hold.
type view struct {
	s         *ScrollbackScreen
	canvas    [][]Cell
	top       int
	canvasSeq uint64 // the seq of canvas[0]
}

func newView(cols, rows, history int) *view {
	s := NewScrollbackScreen(cols, rows, history)
	s.Reflow = true
	return &view{s: s}
}

// apply applies a delta: to the screen's rows and history (Apply, which
// makes a whole one of another size the program's resize, DECCOLM), and
// the canvas.
func (v *view) apply(t *testing.T, d cell.Delta) {
	t.Helper()
	if err := v.s.Apply(d); err != nil {
		t.Fatal(err)
	}
	var err error
	if v.canvas, v.top, err = d.ApplyCanvas(v.canvas, v.top); err != nil {
		t.Fatal(err)
	}
	if d.Whole {
		v.canvasSeq = v.s.TotalAdded() - uint64(v.top)
	}
}

// resize is the view's side of a resize (Decided 7): the screen reflowed
// as the vtapp's was, given its cursors before and its history's run
// after (ResizeAs, what the vtapp's Geometry entry says); then, the width
// the same and the history followed, the canvas as it was, its screen's
// top moved (a Scrolled); else a whole delta of the canvas.
func (v *view) resize(t *testing.T, rows, cols int, c Cursors, first, total uint64) {
	t.Helper()
	widthWas, alt := v.s.Columns, v.s.IsAltScreenActive()
	if !v.s.ResizeAs(rows, cols, c, first, total) {
		t.Fatalf("resized to %dx%d, the view's history is %d..%d, the vtapp's %d..%d", cols, rows, v.s.FirstSeq(), v.s.TotalAdded(), first, total)
	}
	if cols == widthWas && !alt {
		v.top = int(v.s.TotalAdded() - v.canvasSeq)
		return
	}
	all := append(cloneRows(v.s.ScrollbackRows()), cloneRows(v.s.Rows())...)
	var err error
	if v.canvas, _, err = cell.Whole(all).ApplyCanvas(v.canvas, v.top); err != nil {
		t.Fatal(err)
	}
	v.top, v.canvasSeq = len(v.s.ScrollbackRows()), v.s.FirstSeq()
}

// same says the view is the vtapp's screen: its rows, its history as far
// as the vtapp keeps it, and the canvas the same, its rows past its end
// blank (a height grown, its rows not yet sent).
func (v *view) same(t *testing.T, vtapp *ScrollbackScreen, when string) {
	t.Helper()
	if !reflect.DeepEqual(v.s.Rows(), vtapp.Rows()) {
		t.Fatalf("%s, the view's screen\n%swant\n%s", when, showRows(v.s.Rows()), showRows(vtapp.Rows()))
	}
	if v.s.TotalAdded() != vtapp.TotalAdded() {
		t.Fatalf("%s, the view's history ends at %d, the vtapp's at %d", when, v.s.TotalAdded(), vtapp.TotalAdded())
	}
	first, total := vtapp.FirstSeq(), vtapp.TotalAdded()
	if v.s.FirstSeq() > first {
		t.Fatalf("%s, the view's history starts at %d, the vtapp's at %d", when, v.s.FirstSeq(), first)
	}
	// A row the view never saw is blank in its history and on its canvas
	// (Decided 6): the vtapp has it, to be asked for.
	want := append(cloneRows(vtapp.ScrollbackRows()), vtapp.Rows()...)
	got := v.s.ScrollbackRows()[first-v.s.FirstSeq():]
	for seq := first; seq < total; seq++ {
		if v.s.Unseen(seq) {
			want[seq-first] = cell.BlankRow(len(got[seq-first])) // as wide as the delta that said it
		}
	}
	if !slices.EqualFunc(got, want[:total-first], slices.Equal) {
		t.Fatalf("%s, the view's history\n%swant\n%s", when, showRows(got), showRows(want[:total-first]))
	}
	for seq := max(first, v.canvasSeq); seq < total+uint64(vtapp.Lines); seq++ {
		row := cell.BlankRow(vtapp.Columns)
		if i := int(seq - v.canvasSeq); i < len(v.canvas) && i < v.top+vtapp.Lines {
			row = v.canvas[i]
		}
		if w := want[seq-first]; !reflect.DeepEqual(row, w) {
			t.Fatalf("%s, canvas row %d\n%swant\n%s", when, seq, showRows([][]Cell{row}), showRows([][]Cell{w}))
		}
	}
}

// run puts a vtapp and a view through steps: the vtapp's screen fed and
// cut, its deltas applied to the view, the alternate screen said as the
// vtapp's Modes entry says it, a history cleared (ED 3, a reset) at its
// point among the deltas as the Cleared entry says it; a resize cut
// before, the model reset after (Decided 7), or the screen sent whole
// where the view cannot follow (Deltas.Resized). After every cut and every
// resize the view is the vtapp's screen, its rows unseen blank.
func runDeltaSteps(t *testing.T, steps []deltaStep) {
	t.Helper()
	vtapp, st, d := cutting(20, 6, 30)
	v := newView(20, 6, 30)
	unseen := false // a delta has said a row unseen: the view's history may have gaps
	take := func(delta cell.Delta) {
		for _, s := range delta.Scrolls {
			unseen = unseen || s.Unseen != nil
		}
		v.apply(t, delta)
		v.s.SetAltScreen(vtapp.IsAltScreenActive())
	}
	vtapp.Cleared = func() {
		take(d.Cut())
		v.s.ClearHistory()
	}
	cut := func(when string) {
		take(d.Cut())
		v.same(t, vtapp, when)
	}
	d.Reset()
	v.s.setRows(cloneRows(vtapp.Rows()))
	v.canvas, v.top = cloneRows(vtapp.Rows()), 0
	for i, p := range steps {
		switch {
		case p.cut:
			cut(fmt.Sprintf("cut %d", i))
		case p.rows != 0:
			cut(fmt.Sprintf("before resize %d", i))
			c, width, alt := vtapp.Cursors(), p.cols != vtapp.Columns, vtapp.IsAltScreenActive()
			vtapp.Resize(p.rows, p.cols)
			d.Resized(width, alt, unseen)
			if alt || width && unseen {
				unseen = true // the view's history renumbered, every row unseen
			}
			v.resize(t, p.rows, p.cols, c, vtapp.FirstSeq(), vtapp.TotalAdded())
			when := fmt.Sprintf("resize %d to %dx%d", i, p.cols, p.rows)
			if alt || width && unseen {
				cut(when + ", and the screen whole")
				continue
			}
			v.same(t, vtapp, when)
		default:
			st.FeedBytes([]byte(p.bytes))
		}
	}
	cut("at the end")
}

// The deltas and the resizes between them make the vtapp's screen in a
// view, its history and its canvas, after every cut and every resize: a
// width change reflows both the same, a height change moves rows between
// the screen and the history at the same seqs.
func TestDeltasAndResizesMakeTheScreen(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("0123456789", 5) + "\r\n"
	cut := deltaStep{cut: true}
	for _, tc := range []struct {
		name  string
		steps []deltaStep
	}{
		{"lines", []deltaStep{{bytes: numbered(0, 4)}, cut, {bytes: numbered(4, 9)}, cut}},
		{"a burst past the history", []deltaStep{{bytes: numbered(0, 50)}, cut, {bytes: numbered(50, 3)}}},
		{"wider", []deltaStep{{bytes: long + numbered(0, 12)}, cut, {rows: 6, cols: 31}, {bytes: "more\r\n"}, cut}},
		{"narrower", []deltaStep{{bytes: numbered(0, 8) + long}, {rows: 6, cols: 9}, {bytes: long}, cut, {rows: 6, cols: 40}}},
		{"taller", []deltaStep{{bytes: numbered(0, 20)}, cut, {rows: 11, cols: 20}, {bytes: numbered(20, 2)}, cut}},
		{"shorter", []deltaStep{{bytes: numbered(0, 20)}, {rows: 3, cols: 20}, {bytes: numbered(20, 2)}, cut, {rows: 9, cols: 20}}},
		{"shorter with blank rows below", []deltaStep{{bytes: "\x1b[2J\x1b[Htwo\r\nlines"}, cut, {rows: 3, cols: 20}, {bytes: "x"}}},
		{"wider then taller, a full history", []deltaStep{{bytes: numbered(0, 40) + long}, cut, {rows: 6, cols: 30}, {rows: 12, cols: 30}, {bytes: numbered(40, 2)}}},
		{"a region scrolling", []deltaStep{{bytes: numbered(0, 8) + "\x1b[2;5r\x1b[5;1H"}, cut, {bytes: "\na\nb\nc"}, cut, {bytes: "\x1b[r"}, {rows: 6, cols: 25}}},
		{"colors and erases", []deltaStep{{bytes: "\x1b[41m" + numbered(0, 8) + "\x1b[2K\x1b[0m"}, cut, {bytes: "\x1b[?5h" + numbered(8, 2)}, cut}},
		{"the alt screen", []deltaStep{{bytes: numbered(0, 8) + "\x1b[?1049h"}, cut, {bytes: "alt" + numbered(0, 9)}, cut, {bytes: "\x1b[?1049l"}, cut, {rows: 6, cols: 30}}},
		{"wide glyphs", []deltaStep{{bytes: strings.Repeat("表示", 15) + "\r\n"}, cut, {rows: 6, cols: 7}, {bytes: "表x表\r\n"}}},
		{"an erase of the saved lines", []deltaStep{{bytes: numbered(0, 12)}, cut, {bytes: "\x1b[3J" + numbered(12, 2)}, cut}},
		{"an erase of the saved lines among lines", []deltaStep{{bytes: numbered(0, 12)}, cut, {bytes: numbered(12, 3) + "\x1b[3J" + numbered(15, 2)}, cut}},
		{"a reset", []deltaStep{{bytes: numbered(0, 12)}, cut, {bytes: numbered(12, 3) + "\x1bc" + numbered(15, 9)}, cut}},
		{"the alt screen resized", []deltaStep{{bytes: numbered(0, 10) + "\x1b[?1049h"}, cut, {bytes: "alt" + numbered(0, 9)}, cut, {rows: 4, cols: 20}, {bytes: "x"}, {rows: 4, cols: 30}, {rows: 8, cols: 30}, {bytes: "\x1b[?1049l"}, cut, {bytes: numbered(20, 3)}}},
		{"a burst, then wider", []deltaStep{{bytes: numbered(0, 3)}, cut, {bytes: long + numbered(3, 20)}, cut, {rows: 6, cols: 31}, {bytes: "more\r\n"}, cut, {rows: 5, cols: 31}}},
		{"a program's own resize", []deltaStep{{bytes: numbered(0, 8) + "\x1b[?40h"}, cut, {bytes: numbered(8, 4) + "\x1b[?3h" + numbered(12, 2)}, cut, {bytes: numbered(14, 2)}}},
	} {
		t.Run(tc.name, func(t *testing.T) { runDeltaSteps(t, tc.steps) })
	}
}

// The same for any output, cut anywhere, resized at any width and height.
func FuzzDeltasAndResizes(f *testing.F) {
	f.Add([]byte("a\x1b[31mred\x1b[0m\r\nline\r\n"+numbered(0, 9)), uint16(4), uint8(12), uint8(3))
	f.Add([]byte("\x1b[?1049h表示\x1b[2;3r"+numbered(0, 9)+"\x1b[?1049l"), uint16(17), uint8(9), uint8(9))
	f.Add([]byte(strings.Repeat("w", 70)+"\x1b[5S\x1b[2L"+numbered(0, 40)), uint16(31), uint8(30), uint8(1))
	f.Fuzz(func(t *testing.T, out []byte, at uint16, cols, rows uint8) {
		cut := int(at) % (len(out) + 1)
		runDeltaSteps(t, []deltaStep{
			{bytes: string(out[:cut])}, {cut: true},
			{bytes: string(out[cut:])},
			{rows: 2 + int(rows)%12, cols: 2 + int(cols)%60},
			{bytes: "end\r\n"}, {cut: true},
			{rows: 6, cols: 20},
		})
	})
}

// A cut on a 200×50 screen: a line scrolled in, and a keystroke's echo.
func BenchmarkDeltasCut(b *testing.B) {
	for _, tc := range []struct{ name, bytes string }{
		{"a line", "0000000 the quick brown fox jumps over the lazy dog\r\n"},
		{"an echo", "x"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			s, st, d := cutting(200, 50, 1000)
			st.FeedBytes([]byte(numbered(0, 60)))
			d.Cut()
			_ = s
			for b.Loop() {
				st.FeedBytes([]byte(tc.bytes))
				d.Cut()
			}
		})
	}
}
