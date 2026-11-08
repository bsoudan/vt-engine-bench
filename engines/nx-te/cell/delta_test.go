package cell

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

var (
	red  = Attr{Fg: Color{Mode: ColorANSI16, Index: 1}, Bold: true}
	deep = Attr{Bg: Color{Mode: ColorTrueColor, RGB: 0x102030}, Underline: true, UnderlineStyle: 3, UnderlineColor: Color{Mode: ColorANSI256, Index: 200}}
)

// textRow is a row of text, cols wide, blank past it, in a.
func textRow(text string, cols int, a Attr) []Cell {
	row := BlankRow(cols)
	x := 0
	for _, r := range text {
		if x == cols {
			break
		}
		row[x] = Cell{Data: string(r), Attr: a}
		x++
	}
	return row
}

// gridOf is rows of text, cols wide.
func gridOf(cols int, lines ...string) [][]Cell {
	g := make([][]Cell, len(lines))
	for i, l := range lines {
		g[i] = textRow(l, cols, Attr{})
	}
	return g
}

func cloneGrid(g [][]Cell) [][]Cell {
	if g == nil {
		return nil
	}
	out := make([][]Cell, len(g))
	for i, row := range g {
		out[i] = append([]Cell(nil), row...)
	}
	return out
}

func dump(g [][]Cell) string {
	var b strings.Builder
	for _, row := range g {
		b.WriteString("|")
		for _, c := range row {
			switch {
			case c.Data == "":
				b.WriteString("·")
			case c.Attr != (Attr{}):
				b.WriteString(strings.ToUpper(c.Data))
			default:
				b.WriteString(c.Data)
			}
		}
		b.WriteString("|\n")
	}
	return b.String()
}

// roundTrip says d comes back from its JSON and its bytes as it went.
func roundTrip(t *testing.T, d Delta) {
	t.Helper()
	j, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var fromJSON Delta
	if err := json.Unmarshal(j, &fromJSON); err != nil {
		t.Fatalf("%s: %v", j, err)
	}
	if !reflect.DeepEqual(fromJSON, d) {
		t.Fatalf("came back from its JSON\n%#v\nwent\n%#v\nas %s", fromJSON, d, j)
	}
	b, err := d.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	var fromBytes Delta
	if err := fromBytes.UnmarshalBinary(b); err != nil {
		t.Fatalf("% x: %v", b, err)
	}
	// Its size, what a journal counts it for: the same after either codec,
	// and about its bytes.
	if s := d.Size(); s != fromJSON.Size() || s != fromBytes.Size() || s > 2*len(b)+16 || 2*s+16 < len(b) {
		t.Fatalf("a delta of %d bytes is size %d (%d from its JSON, %d from its bytes)", len(b), s, fromJSON.Size(), fromBytes.Size())
	}
	if !reflect.DeepEqual(fromBytes, d) {
		t.Fatalf("came back from its bytes\n%#v\nwent\n%#v", fromBytes, d)
	}
}

// Every shape of delta comes back from its JSON and its bytes as it went.
func TestADeltaComesBackAsItWent(t *testing.T) {
	table := []Cell{{}, {Attr: red}, {Attr: deep, Wrapped: true}, {Empty: true}}
	runs := []Run{{T: "abc"}, {T: "表", A: 1}, {N: 2, A: 2}, {C: []string{"é", "👍🏽"}, A: 3}}
	for _, tc := range []struct {
		name string
		d    Delta
	}{
		{"none", Delta{}},
		{"a size alone", Delta{Cols: 80, Rows: 24}},
		{"whole", Delta{Whole: true, Cols: 10, Rows: 2, Attrs: table, Spans: []Span{{Runs: runs}}}},
		{"a span", Delta{Cols: 10, Rows: 3, Attrs: table[:1], Spans: []Span{{Row: 2, Col: 4, Runs: []Run{{T: "hi"}}}}}},
		{"every run", Delta{Cols: 20, Rows: 3, Attrs: table, Spans: []Span{{Row: 1, Col: 1, Runs: runs}, {Row: 2, Runs: runs[2:]}}}},
		{"a scroll up and one down", Delta{Cols: 5, Rows: 6, Scrolls: []Scroll{{Top: 1, Bottom: 4, N: 2}, {Top: 2, Bottom: 5, N: -1}}}},
		{"a history scroll", Delta{Cols: 5, Rows: 6, Scrolls: []Scroll{{Bottom: 5, N: 3, History: true}}}},
		{"rows unseen", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Bottom: 1, N: 7, History: true, Unseen: []Rows{{From: 1, N: 1}, {From: 3, N: 4}}}}}},
		{"cells before a scroll", Delta{Cols: 4, Rows: 4, Scrolls: []Scroll{{Bottom: 3, N: 1}, {Top: 1, Bottom: 2, N: -1}}, Attrs: table[:2], Spans: []Span{{Row: 3, Before: 2, Runs: []Run{{T: "a"}}}, {Row: 0, Before: 1, Runs: []Run{{T: "b", A: 1}}}, {Row: 1, Runs: []Run{{N: 1}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) { roundTrip(t, tc.d) })
	}
}

// A field added to Cell or Attr is carried by the bytes: every field set,
// one at a time, comes back. The counts say a field was added, and that the
// codec must be told of it.
func TestADeltaCarriesEveryField(t *testing.T) {
	if n := reflect.TypeFor[Cell]().NumField(); n != 4 {
		t.Fatalf("Cell has %d fields, the codec knows 4: carry the new one (delta_codec.go)", n)
	}
	if n := reflect.TypeFor[Attr]().NumField(); n != 15 {
		t.Fatalf("Attr has %d fields, the codec knows 15: carry the new one (delta_codec.go)", n)
	}
	var cells []Cell
	set := func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Bool:
			v.SetBool(true)
		case reflect.Uint8:
			v.SetUint(5)
		case reflect.Struct: // a Color
			v.Field(0).SetUint(uint64(ColorTrueColor))
			v.Field(1).SetUint(7)
			v.Field(2).SetUint(0xabcdef)
		}
	}
	for i := 1; i < reflect.TypeFor[Cell]().NumField(); i++ { // not Data: an entry has none
		var c Cell
		if i == 1 {
			for j := range reflect.TypeFor[Attr]().NumField() {
				c = Cell{}
				set(reflect.ValueOf(&c.Attr).Elem().Field(j))
				cells = append(cells, c)
			}
			continue
		}
		set(reflect.ValueOf(&c).Elem().Field(i))
		cells = append(cells, c)
	}
	d := Delta{Cols: len(cells), Rows: 1, Attrs: cells}
	for i := range cells {
		d.Spans = append(d.Spans, Span{Col: i, Runs: []Run{{N: 1, A: i}}})
	}
	roundTrip(t, d)
}

// The JSON reads as the grid's: runs of text, the scrolls beside them.
func TestADeltasJSONReadsAsItsText(t *testing.T) {
	prev := gridOf(12, "one", "two", "three")
	next := gridOf(12, "two", "three", "four is red")
	next[2][0].Attr = red
	var dd Differ
	dd.Reset(prev)
	d := dd.Delta(next, []Scroll{{Bottom: 2, N: 1}})
	j, _ := json.Marshal(d)
	want := `{"Cols":12,"Rows":3,"Scrolls":[{"Bottom":2,"N":1}],"Attrs":[{"Attr":{"Fg":{"Mode":1,"Index":1},"Bold":true}},{}],"Spans":[{"Row":2,"Runs":[{"T":"f"},{"T":"our is red","A":1}]}]}`
	if string(j) != want {
		t.Fatalf("got  %s\nwant %s", j, want)
	}
}

// grids is a grid before and after, and the scrolls that happened between:
// each case's delta, made by the Differ, makes the after of the before, on
// a grid with a history and on a canvas.
var grids = []struct {
	name    string
	prev    [][]Cell
	scrolls []Scroll
	next    [][]Cell
	history [][]Cell // what a history scroll keeps
}{
	{"no change", gridOf(4, "ab", "cd"), nil, gridOf(4, "ab", "cd"), nil},
	{"a cell", gridOf(4, "ab", "cd"), nil, gridOf(4, "ab", "cx"), nil},
	{"two cells far apart", gridOf(20, "a", "b"), nil, gridOf(20, "a", "b                 z"), nil},
	{"a full scroll", gridOf(4, "1", "2", "3"), []Scroll{{Bottom: 2, N: 1, History: true}}, gridOf(4, "2", "3", "4"), gridOf(4, "1")},
	{"a scroll in a region", gridOf(4, "s", "1", "2", "3", "t"), []Scroll{{Top: 1, Bottom: 3, N: 1}}, gridOf(4, "s", "2", "3", "x", "t"), nil},
	{"a scroll down", gridOf(4, "1", "2", "3"), []Scroll{{Bottom: 2, N: -1}}, gridOf(4, "0", "1", "2"), nil},
	{"a scroll past the region", gridOf(4, "1", "2"), []Scroll{{Bottom: 1, N: 5}}, gridOf(4, "", ""), nil},
	{"a history scroll past the screen, rows unseen", gridOf(3, "1", "2"), []Scroll{{Bottom: 1, N: 4, History: true, Unseen: []Rows{{From: 1, N: 1}, {From: 3, N: 1}}}}, gridOf(3, "5", "6"), [][]Cell{textRow("1", 3, Attr{}), BlankRow(3), BlankRow(3), BlankRow(3)}},
	{"a row unseen made blank", gridOf(3, "1", "2"), []Scroll{{Bottom: 1, N: 1, History: true, Unseen: []Rows{{N: 1}}}}, gridOf(3, "2", "3"), [][]Cell{BlankRow(3)}},
	{"a wide glyph's second half", [][]Cell{{{Data: "表"}, {}, {Data: " "}}}, nil, [][]Cell{{{Data: "表"}, {Attr: red}, {Data: " "}}}, nil},
	{"another size", gridOf(4, "ab"), nil, gridOf(5, "ab", "cd"), nil},
}

// A delta applied makes the grid after; the history scroll keeps the rows
// that left; the same delta on a canvas makes the canvas's screen the grid
// after, what left on the canvas above it.
func TestADeltaMakesTheGridAfter(t *testing.T) {
	for _, tc := range grids {
		t.Run(tc.name, func(t *testing.T) {
			var dd Differ
			dd.Reset(tc.prev)
			d := dd.Delta(tc.next, tc.scrolls)
			roundTrip(t, d)
			if !reflect.DeepEqual(dd.Held(), tc.next) {
				t.Fatalf("the Differ holds\n%s, not the grid after\n%s", dump(dd.Held()), dump(tc.next))
			}
			var kept [][]Cell
			got, err := d.Apply(cloneGrid(tc.prev), func(row []Cell, unseen bool) {
				if unseen && !slices.Equal(row, BlankRow(len(row))) {
					t.Fatalf("a row unseen kept as %q, not blank", dump([][]Cell{row}))
				}
				kept = append(kept, row)
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.next) {
				t.Fatalf("applied, the grid is\n%swant\n%s", dump(got), dump(tc.next))
			}
			if !reflect.DeepEqual(kept, tc.history) {
				t.Fatalf("kept\n%swant\n%s", dump(kept), dump(tc.history))
			}
			// On a canvas: some rows above the screen, which stay.
			above := gridOf(len(tc.prev[0]), "h1", "h2")
			given := append(cloneGrid(above), cloneGrid(tc.prev)...)
			canvas, top, err := d.ApplyCanvas(given, len(above))
			if err != nil {
				t.Fatal(err)
			}
			// The canvas given is untouched: it may be another holder's.
			if was := append(cloneGrid(above), tc.prev...); !reflect.DeepEqual(given, was) {
				t.Fatalf("the canvas given was written:\n%swas\n%s", dump(given), dump(was))
			}
			if d.Whole {
				above = nil
			}
			if want := append(append(cloneGrid(above), tc.history...), tc.next...); !reflect.DeepEqual(canvas, want) {
				t.Fatalf("on a canvas\n%swant\n%s", dump(canvas), dump(want))
			}
			if top != len(canvas)-len(tc.next) {
				t.Fatalf("the screen's top is %d of %d rows", top, len(canvas))
			}
		})
	}
}

// A delta cut from a canvas's screen where it is: a canvas that ends past
// the screen (a height that shrank) or before it (one that grew) is cut or
// padded to end where the screen does.
func TestACanvasEndsWhereTheScreenDoes(t *testing.T) {
	d := Delta{Cols: 2, Rows: 2, Spans: []Span{{Row: 1, Runs: []Run{{T: "z"}}}}}
	rows, top, err := d.ApplyCanvas(gridOf(2, "a", "b", "c", "d"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := gridOf(2, "a", "b", "z"); !reflect.DeepEqual(rows, want) || top != 1 {
		t.Fatalf("got top %d\n%swant\n%s", top, dump(rows), dump(want))
	}
	rows, top, err = d.ApplyCanvas(gridOf(2, "a", "b"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := gridOf(2, "a", "b", "z"); !reflect.DeepEqual(rows, want) || top != 1 {
		t.Fatalf("got top %d\n%swant\n%s", top, dump(rows), dump(want))
	}
}

// Two deltas merged, applied, are the two applied in turn: the grid and the
// history, for every pair of the cases, the cells of the first before the
// scrolls of the second. A whole second is the whole; its history is its own.
func TestMergedDeltasApplyAsTheTwo(t *testing.T) {
	for _, a := range grids {
		for _, b := range grids {
			if len(a.next) != len(b.prev) || len(a.next[0]) != len(b.prev[0]) {
				continue
			}
			t.Run(a.name+", then "+b.name, func(t *testing.T) {
				var dd Differ
				dd.Reset(a.prev)
				first := dd.Delta(a.next, a.scrolls)
				// The second from what the first left, changed: so the
				// first's cells are under the second's scrolls.
				from := cloneGrid(a.next)
				from[0][0] = Cell{Data: "M", Attr: deep}
				second := dd.Delta(from, nil)
				dd.Reset(from)
				third := dd.Delta(b.next, b.scrolls)
				m, err := first.Merge(second)
				if err != nil {
					t.Fatal(err)
				}
				m, err = m.Merge(third)
				if err != nil {
					t.Fatal(err)
				}
				roundTrip(t, m)

				var inTurn, merged [][]Cell
				g := cloneGrid(a.prev)
				for _, d := range []Delta{first, second, third} {
					if d.Whole {
						inTurn = nil
					}
					if g, err = d.Apply(g, func(r []Cell, _ bool) { inTurn = append(inTurn, r) }); err != nil {
						t.Fatal(err)
					}
				}
				got, err := m.Apply(cloneGrid(a.prev), func(r []Cell, _ bool) { merged = append(merged, r) })
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, g) {
					t.Fatalf("merged\n%sin turn\n%s", dump(got), dump(g))
				}
				if !reflect.DeepEqual(merged, inTurn) {
					t.Fatalf("merged kept\n%sin turn\n%s", dump(merged), dump(inTurn))
				}
			})
		}
	}
}

// A delta that does not fit what it is given is ErrDelta, and leaves it as
// it was: a size that is not the grid's, a scroll or a span outside it, a
// run naming no entry, rows off a scroll that keeps none.
func TestADeltaThatDoesNotFitIsRefused(t *testing.T) {
	grid := gridOf(3, "ab", "cd")
	for _, tc := range []struct {
		name string
		d    Delta
	}{
		{"another size", Delta{Cols: 4, Rows: 2}},
		{"a scroll below", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Top: 1, Bottom: 2, N: 1}}}},
		{"a scroll of none", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Bottom: 1}}}},
		{"a history scroll of a region", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Top: 1, Bottom: 1, N: 1, History: true}}}},
		{"rows unseen no history", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Bottom: 1, N: 1, Unseen: []Rows{{N: 1}}}}}},
		{"rows unseen past the scroll", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Bottom: 1, N: 2, History: true, Unseen: []Rows{{From: 1, N: 2}}}}}},
		{"rows unseen out of order", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Bottom: 1, N: 4, History: true, Unseen: []Rows{{From: 2, N: 1}, {From: 1, N: 1}}}}}},
		{"no rows unseen", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Bottom: 1, N: 4, History: true, Unseen: []Rows{{From: 2}}}}}},
		{"a span past the edge", Delta{Cols: 3, Rows: 2, Spans: []Span{{Col: 2, Runs: []Run{{T: "ab"}}}}}},
		{"a span below", Delta{Cols: 3, Rows: 2, Spans: []Span{{Row: 2, Runs: []Run{{T: "a"}}}}}},
		{"no entry", Delta{Cols: 3, Rows: 2, Attrs: []Cell{{}}, Spans: []Span{{Runs: []Run{{T: "a", A: 1}}}}}},
		{"spans out of order", Delta{Cols: 3, Rows: 2, Scrolls: []Scroll{{Bottom: 1, N: 1}}, Spans: []Span{{Runs: []Run{{T: "a"}}}, {Before: 1, Runs: []Run{{T: "a"}}}}}},
		{"too big", Delta{Whole: true, Cols: 1 << 20, Rows: 1 << 20}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := cloneGrid(grid)
			if _, err := tc.d.Apply(g, nil); !errors.Is(err, ErrDelta) {
				t.Fatalf("applied: %v", err)
			}
			if _, _, err := tc.d.ApplyCanvas(g, 0); !errors.Is(err, ErrDelta) {
				t.Fatalf("on a canvas: %v", err)
			}
			if !reflect.DeepEqual(g, grid) {
				t.Fatalf("refused, the grid is\n%s", dump(g))
			}
		})
	}
}

// The Differ's scrolls told one after another, the same rows the same way,
// are one scroll; its rows unseen at their places in it, runs that meet
// one run.
func TestScrollsToldInARowAreOne(t *testing.T) {
	var dd Differ
	dd.Reset(gridOf(2, "a", "b"))
	dd.Scroll(Scroll{Bottom: 1, N: 1, History: true})
	dd.Scroll(Scroll{Bottom: 1, N: 1, History: true, Unseen: []Rows{{N: 1}}})
	dd.Scroll(Scroll{Bottom: 1, N: 2, History: true, Unseen: []Rows{{N: 1}}})
	dd.Scroll(Scroll{Bottom: 1, N: 1, History: true, Unseen: []Rows{{N: 1}}})
	dd.Scroll(Scroll{Bottom: 1, N: -1})
	dd.Scroll(Scroll{Top: 5, Bottom: 6, N: 1}) // outside: dropped
	d := dd.Delta(gridOf(2, "", ""), nil)
	want := []Scroll{{Bottom: 1, N: 5, History: true, Unseen: []Rows{{From: 1, N: 2}, {From: 4, N: 1}}}, {Bottom: 1, N: -1}}
	if !reflect.DeepEqual(d.Scrolls, want) {
		t.Fatalf("got %+v\nwant %+v", d.Scrolls, want)
	}
}

// fuzzGrid is a grid of rows×cols made from b: each byte a cell's text,
// attribute and kind.
func fuzzGrid(b []byte, cols, rows int) [][]Cell {
	g := blankGrid(cols, rows)
	for i, c := range b {
		if i >= cols*rows {
			break
		}
		cell := &g[i/cols][i%cols]
		switch c % 8 {
		case 0, 1, 2:
			cell.Data = string(rune('a' + c%26))
		case 3:
			cell.Data = string(rune('A'+c%26)) + "́"
		case 4:
			cell.Data = ""
		case 5:
			cell.Data, cell.Attr = string(rune('a'+c%26)), red
		case 6:
			cell.Data, cell.Wrapped = "w", true
		}
	}
	return g
}

// Whatever two grids and scrolls, the Differ's delta applied to the first
// makes the second, on a grid and on a canvas, and comes back from its JSON
// and its bytes; two deltas merged apply as the two; and whatever bytes
// come, they are read, and what is read applied, without panicking.
func FuzzDelta(f *testing.F) {
	f.Add([]byte("hello world"), []byte("hellO world, and on"), []byte{1, 2, 3}, uint8(5), uint8(3))
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7}, []byte{7, 6, 5, 4, 3, 2, 1, 0}, []byte{0, 0, 9, 200}, uint8(4), uint8(4))
	f.Fuzz(func(t *testing.T, a, b, ops []byte, cols, rows uint8) {
		c, r := 1+int(cols)%12, 1+int(rows)%8
		prev, next := fuzzGrid(a, c, r), fuzzGrid(b, c, r)
		var scrolls []Scroll
		for i := 0; i+2 < len(ops); i += 3 {
			top, bottom := int(ops[i])%r, int(ops[i+1])%r
			if top > bottom {
				top, bottom = bottom, top
			}
			n := int(int8(ops[i+2])) % 6
			if n == 0 {
				continue
			}
			s := Scroll{Top: top, Bottom: bottom, N: n}
			if ops[i+2]&0x40 != 0 && n > 0 {
				s = Scroll{Bottom: r - 1, N: n, History: true}
				if ops[i]&1 != 0 {
					s.Unseen = []Rows{{N: 1}}
				}
			}
			scrolls = append(scrolls, s)
		}
		var dd Differ
		dd.Reset(prev)
		d := dd.Delta(next, scrolls)
		roundTrip(t, d)
		got, err := d.Apply(cloneGrid(prev), nil)
		if err != nil || !reflect.DeepEqual(got, next) {
			t.Fatalf("%v: applied\n%swant\n%s", err, dump(got), dump(next))
		}
		given := cloneGrid(prev)
		canvas, top, err := d.ApplyCanvas(given, 0)
		if err != nil || !reflect.DeepEqual(canvas[top:], next) {
			t.Fatalf("%v: on a canvas\n%swant\n%s", err, dump(canvas), dump(next))
		}
		if !reflect.DeepEqual(given, prev) {
			t.Fatalf("the canvas given was written:\n%swas\n%s", dump(given), dump(prev))
		}
		// Then back, merged.
		back := dd.Delta(prev, nil)
		m, err := d.Merge(back)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := m.Apply(cloneGrid(prev), nil); err != nil || !reflect.DeepEqual(got, prev) {
			t.Fatalf("%v: merged there and back\n%swant\n%s", err, dump(got), dump(prev))
		}
		// Any bytes.
		var any Delta
		if any.UnmarshalBinary(ops) == nil {
			_, _ = any.Apply(cloneGrid(prev), func([]Cell, bool) {})
			_, _, _ = any.ApplyCanvas(cloneGrid(prev), 0)
		}
		if json.Unmarshal(a, &any) == nil {
			_, _ = any.Apply(cloneGrid(prev), func([]Cell, bool) {})
		}
	})
}

// The benchmarks' grid: 200×50 of text in a few colors.
func benchGrid(seed int) [][]Cell {
	g := make([][]Cell, 50)
	for y := range g {
		a := Attr{}
		if y%5 == 0 {
			a = red
		}
		g[y] = textRow(fmt.Sprintf("%07d the quick brown fox jumps over the lazy dog %d %s", y+seed, seed, strings.Repeat("0123456789", 14)), 200, a)
	}
	return g
}

// The deltas the benchmarks take: a whole screen, a line scrolled in, a
// keystroke's echo.
func benchDeltas() map[string]Delta {
	var dd Differ
	g := benchGrid(0)
	whole := dd.Delta(g, nil)
	scrolled := append(cloneGrid(g[1:]), textRow("a new line", 200, Attr{}))
	scroll := dd.Delta(scrolled, []Scroll{{Bottom: 49, N: 1, History: true}})
	typed := cloneGrid(scrolled)
	typed[49][10] = Cell{Data: "x"}
	echo := dd.Delta(typed, nil)
	return map[string]Delta{"whole": whole, "scroll": scroll, "echo": echo}
}

func BenchmarkDeltaCodec(b *testing.B) {
	for _, name := range []string{"whole", "scroll", "echo"} {
		d := benchDeltas()[name]
		bin, _ := d.MarshalBinary()
		js, _ := json.Marshal(d)
		b.Run("binary/encode/"+name, func(b *testing.B) {
			b.SetBytes(int64(len(bin)))
			for b.Loop() {
				_, _ = d.MarshalBinary()
			}
		})
		b.Run("binary/decode/"+name, func(b *testing.B) {
			b.SetBytes(int64(len(bin)))
			for b.Loop() {
				var out Delta
				_ = out.UnmarshalBinary(bin)
			}
		})
		b.Run("json/encode/"+name, func(b *testing.B) {
			b.SetBytes(int64(len(js)))
			for b.Loop() {
				_, _ = json.Marshal(d)
			}
		})
		b.Run("json/decode/"+name, func(b *testing.B) {
			b.SetBytes(int64(len(js)))
			for b.Loop() {
				var out Delta
				_ = json.Unmarshal(js, &out)
			}
		})
	}
}

func BenchmarkDeltaApply(b *testing.B) {
	for _, name := range []string{"whole", "scroll", "echo"} {
		d := benchDeltas()[name]
		b.Run(name, func(b *testing.B) {
			g := benchGrid(0)
			for b.Loop() {
				g, _ = d.Apply(g, nil)
				if name == "scroll" { // the grid it fits again
					g[49] = textRow("", 200, Attr{})
				}
			}
		})
	}
}

// The Differ on a stream of 200×50 frames, each from the one before: the
// same; a cell typed and taken back; a line scrolled in, told or not; every
// row new.
func BenchmarkDiffer(b *testing.B) {
	g := benchGrid(0)
	typed := cloneGrid(g)
	typed[49][10] = Cell{Data: "x"}
	// A screen scrolling: frame k is lines k..k+49.
	lines := benchGrid(0)
	for k := 1; k < 4; k++ {
		lines = append(lines, benchGrid(50*k)...)
	}
	frames := make([][][]Cell, 128)
	for k := range frames {
		frames[k] = lines[k : k+50]
	}
	for _, tc := range []struct {
		name    string
		frames  [][][]Cell
		scrolls []Scroll
	}{
		{"unchanged", [][][]Cell{g}, nil},
		{"echo", [][][]Cell{typed, g}, nil},
		{"scroll told", frames, []Scroll{{Bottom: 49, N: 1}}},
		{"scroll not told", frames, nil},
		{"every row", [][][]Cell{benchGrid(1), g}, nil},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var dd Differ
			dd.Reset(tc.frames[0])
			i := 0
			for b.Loop() {
				i++
				if i == len(tc.frames) && tc.scrolls != nil {
					dd.Reset(tc.frames[0]) // the last frame to the first is no scroll
					i = 1
				}
				_ = dd.Delta(tc.frames[i%len(tc.frames)], tc.scrolls)
			}
		})
	}
}
