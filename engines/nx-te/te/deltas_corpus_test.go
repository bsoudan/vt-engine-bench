package te

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"nx/cell"
)

// The invariant over te's corpora (doc/done/DELTA_PLAN.md, Values: "for
// every byte stream in te's corpora ... the deltas applied to a grid and a
// history equal the screen and its history after each cut, and the same
// deltas applied to a plain canvas equal the view's shadow's canvas").
//
// Every screen te's own tests make — the esctest and pyte ports, the
// stream's, the reflow's, the history's, the fuzz seeds — is watched
// (screenMade): its deltas are cut as it is made, at its scrolls (every
// one, or every second, third or fourth, so a cut holds several scrolls and
// the cells between), and once more when every test has run. Each delta goes
// through its bytes and its JSON, and is applied to a grid with a history
// and to a canvas; after each cut the grid is the screen, the canvas's
// screen is too, and the history the grid kept and the canvas's rows above
// its screen are the screen's history, row for row, as far as both hold it.
// What moved the history other than a delta — a resize, a reflow, a
// restored snapshot — the shadows re-take from the screen, as a view
// re-takes its own reflow (deltas_test.go checks that path). A failure names
// the test that made the screen, and fails the run.
func TestMain(m *testing.M) {
	flag.Parse()
	measuring := false
	for _, name := range []string{"test.bench", "test.fuzz"} {
		if f := flag.Lookup(name); f != nil && f.Value.String() != "" {
			measuring = true
		}
	}
	if !measuring {
		screenMade = watch
	}
	code := m.Run()
	if screenMade != nil {
		screenMade = nil
		if failed := corpusFinish(); len(failed) > 0 {
			fmt.Fprintf(os.Stderr, "--- FAIL: the deltas of %d screens did not rebuild them (deltas_corpus_test.go):\n", len(failed))
			for i, f := range failed {
				if i == 20 {
					fmt.Fprintf(os.Stderr, "... and %d more\n", len(failed)-i)
					break
				}
				fmt.Fprintln(os.Stderr, f)
			}
			code = 1
		}
	}
	os.Exit(code)
}

// corpus is every screen watched.
var corpus struct {
	sync.Mutex
	screens []*watched
}

// watched is a screen of a test's, its deltas, and the shadows they are
// applied to.
type watched struct {
	test   string // the test that made it
	s      *Screen
	h      *ScrollbackScreen // nil: a screen with no history
	d      *Deltas
	every  int // a cut at every this many scrolls
	calls  int
	cuts   int
	failed string

	grid      [][]Cell // the grid the deltas are applied to
	hist      [][]Cell // the rows its history scrolls kept
	histFirst uint64   // the seq of hist[0]
	canvas    [][]Cell // the canvas the deltas are applied to
	top       int      // its screen's top
	canvasSeq uint64   // the seq of canvas[0]
	whole     bool     // a whole delta has been applied: the canvas is numbered
	// unseen is the seqs of the rows a delta kept unseen: blank on the
	// shadows, where the screen's history has what was written.
	unseen map[uint64]bool
}

func watch(s *Screen, h *ScrollbackScreen) {
	w := &watched{test: testName(), s: s, h: h}
	corpus.Lock()
	w.every = 1 + len(corpus.screens)%4
	corpus.screens = append(corpus.screens, w)
	corpus.Unlock()
	w.d = NewDeltas(s)
	hook := s.Scrolled
	s.Scrolled = func(top, bottom, n int, history bool) {
		w.calls++
		if w.calls%w.every == 0 {
			w.cut() // the screen as it is before the scroll
		}
		hook(top, bottom, n, history)
	}
	w.cut()
}

// corpusFinish cuts every screen watched a last time, and says those whose
// deltas failed to rebuild them.
func corpusFinish() []string {
	corpus.Lock()
	defer corpus.Unlock()
	var failed []string
	for _, w := range corpus.screens {
		w.cut()
		if w.failed != "" {
			failed = append(failed, w.failed)
		}
	}
	return failed
}

// testName is the test whose code is making a screen.
func testName() string {
	pcs := make([]uintptr, 64)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs)])
	for {
		f, more := frames.Next()
		name := f.Function[strings.LastIndex(f.Function, "/")+1:]
		if _, after, ok := strings.Cut(name, "."); ok {
			name, _, _ = strings.Cut(after, ".")
			for _, p := range []string{"Test", "Fuzz", "Benchmark", "Example"} {
				if strings.HasPrefix(name, p) {
					return name
				}
			}
		}
		if !more {
			return "a screen made outside a test"
		}
	}
}

func (w *watched) fail(format string, args ...any) {
	w.failed = fmt.Sprintf("%s, cut %d: %s", w.test, w.cuts, fmt.Sprintf(format, args...))
}

// cut cuts a delta, sends it through its bytes and JSON, applies it to the
// shadows, and checks them.
func (w *watched) cut() {
	if w.failed != "" {
		return
	}
	w.cuts++
	d := w.d.Cut()
	b, err := d.MarshalBinary()
	if err != nil {
		w.fail("encoded: %v", err)
		return
	}
	var sent cell.Delta
	if err := sent.UnmarshalBinary(b); err != nil || !reflect.DeepEqual(sent, d) {
		w.fail("through its bytes (%v):\n%#v\nwent\n%#v", err, sent, d)
		return
	}
	if validText(d) {
		j, err := json.Marshal(d)
		var read cell.Delta
		if err == nil {
			err = json.Unmarshal(j, &read)
		}
		if err != nil || !reflect.DeepEqual(read, d) {
			w.fail("through its JSON (%v):\n%s", err, j)
			return
		}
	}
	var keep func([]Cell, bool)
	if w.h != nil {
		keep = func(row []Cell, unseen bool) {
			if unseen {
				if w.unseen == nil {
					w.unseen = map[uint64]bool{}
				}
				w.unseen[w.histFirst+uint64(len(w.hist))] = true
			}
			w.hist = append(w.hist, row)
		}
	}
	if w.grid, err = sent.Apply(w.grid, keep); err != nil {
		w.fail("applied: %v", err)
		return
	}
	if w.canvas, w.top, err = sent.ApplyCanvas(w.canvas, w.top); err != nil {
		w.fail("on a canvas: %v", err)
		return
	}
	if d.Whole {
		w.whole = true
		w.canvasSeq = w.total() - uint64(w.top)
	}
	w.check()
}

// total is the seq the screen's top row will have once it scrolls off.
func (w *watched) total() uint64 {
	if w.h == nil {
		return 0
	}
	return w.h.TotalAdded()
}

func (w *watched) check() {
	if !reflect.DeepEqual(w.grid, w.s.Rows()) {
		w.fail("the grid is not the screen:\ngrid\n%sscreen\n%s", showRows(w.grid), showRows(w.s.Rows()))
		return
	}
	if !reflect.DeepEqual(w.canvas[w.top:], w.s.Rows()) {
		w.fail("the canvas's screen is not the screen:\ncanvas\n%sscreen\n%s", showRows(w.canvas[w.top:]), showRows(w.s.Rows()))
		return
	}
	if w.h == nil {
		return
	}
	total, first, kept := w.h.TotalAdded(), w.h.FirstSeq(), w.h.ScrollbackRows()
	if w.histFirst+uint64(len(w.hist)) != total { // moved by other than a delta
		w.hist, w.histFirst, w.unseen = cloneRows(kept), first, nil
		w.canvasSeq = total + 1 // and the canvas re-taken with it
	}
	if w.canvasSeq+uint64(w.top) != total {
		w.canvas = append(cloneRows(kept), cloneRows(w.s.Rows())...)
		w.top, w.canvasSeq = len(kept), first
	}
	if first > w.histFirst { // what the screen forgot, the shadows may too
		k := min(first-w.histFirst, uint64(len(w.hist)))
		w.hist, w.histFirst = w.hist[k:], w.histFirst+k
	}
	if first > w.canvasSeq {
		k := min(first-w.canvasSeq, uint64(w.top))
		w.canvas, w.top, w.canvasSeq = w.canvas[k:], w.top-int(k), w.canvasSeq+k
	}
	// A row kept unseen is blank on the shadows (Decided 6), as wide as
	// the delta that said it: the screen's history has it, to be asked for.
	wantAt := func(seq uint64, got []Cell) []Cell {
		if w.unseen[seq] {
			return cell.BlankRow(len(got))
		}
		return kept[seq-first]
	}
	for seq := max(first, w.histFirst); seq < total; seq++ {
		got := w.hist[seq-w.histFirst]
		if want := wantAt(seq, got); !reflect.DeepEqual(got, want) {
			w.fail("history row %d kept as\n%swant\n%s", seq, showRows([][]Cell{got}), showRows([][]Cell{want}))
			return
		}
	}
	for seq := max(first, w.canvasSeq); seq < total; seq++ {
		got := w.canvas[seq-w.canvasSeq]
		if want := wantAt(seq, got); !reflect.DeepEqual(got, want) {
			w.fail("canvas row %d is\n%swant\n%s", seq, showRows([][]Cell{got}), showRows([][]Cell{want}))
			return
		}
	}
}

// validText says d's text is UTF-8, which its JSON says as it is.
func validText(d cell.Delta) bool {
	for _, sp := range d.Spans {
		for _, r := range sp.Runs {
			if !utf8.ValidString(r.T) {
				return false
			}
			for _, c := range r.C {
				if !utf8.ValidString(c) {
					return false
				}
			}
		}
	}
	return true
}

func cloneRows(rows [][]Cell) [][]Cell {
	out := make([][]Cell, len(rows))
	for i, r := range rows {
		out[i] = append([]Cell(nil), r...)
	}
	return out
}

// dumpRows is rows as a failure shows them: the text, an attribute as
// upper case, a cell with no text as a dot.
func showRows(rows [][]Cell) string {
	var b strings.Builder
	for _, row := range rows {
		b.WriteString("|")
		for _, c := range row {
			switch {
			case c.Data == "":
				b.WriteString("·")
			case c.Attr != (Attr{}) || c.Wrapped || c.Empty:
				b.WriteString(strings.ToUpper(c.Data))
			default:
				b.WriteString(c.Data)
			}
		}
		b.WriteString("|\n")
	}
	return b.String()
}
