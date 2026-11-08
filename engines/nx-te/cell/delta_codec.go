package cell

import (
	"fmt"
	"math"

	"nx/wire"
)

// A Delta's bytes (MarshalBinary) are its JSON's runs with varints: what a
// module boundary copies rather than encodes (doc/done/DELTA_PLAN.md,
// Decided 15). They are put and got through the wire's Writer and Reader
// (Put, Get), so a codec that carries a delta puts its own fields and the
// delta's in one list.
//
//	delta:  flags(1: Whole) Cols Rows, table, n scroll..., n span...
//	table:  n entry...
//	entry:  bits, then Fg, Bg, UnderlineColor where bits say, UnderlineStyle
//	scroll: Top Bottom N(signed) flags(1: History, 2: Unseen), [Unseen: n (From N)...]
//	span:   Row Col Before n run...
//	run:    A<<2|kind, then T: len text; N: n; C: n (len text)...
//
// Every number is a uvarint but N's, a varint. An entry is a cell without
// its text; a field added to Cell or Attr is added here
// (TestADeltaCarriesEveryField says so).

// The bits of an entry.
const (
	bitBold = 1 << iota
	bitFaint
	bitItalics
	bitUnderline
	bitStrikethrough
	bitReverse
	bitBlink
	bitConceal
	bitProtected
	bitISOProtected
	bitOverline
	bitWrapped
	bitEmpty
	bitFg
	bitBg
	bitUnderlineColor
	bitUnderlineStyle
)

// The kinds of run, in its bytes.
const (
	binText  = 0
	binNone  = 1
	binCells = 2
)

// Size is about how many bytes d is (AppendBinary), worked out from what
// it holds without making them: a run its text and a few, a run of rows
// unseen a few, an entry of the table a few. The
// same delta is the same size wherever it is worked out, so a journal of
// deltas may count its entries by it on both of its ends.
func (d Delta) Size() int {
	n := 4 + 4*len(d.Attrs)
	for _, s := range d.Scrolls {
		n += 6 + 4*len(s.Unseen)
	}
	for _, sp := range d.Spans {
		n += 4
		for _, r := range sp.Runs {
			n += 3 + len(r.T)
			for _, c := range r.C {
				n += 1 + len(c)
			}
		}
	}
	return n
}

// MarshalBinary is d as bytes.
func (d Delta) MarshalBinary() ([]byte, error) { return d.AppendBinary(nil) }

// AppendBinary appends d's bytes to b.
func (d Delta) AppendBinary(b []byte) ([]byte, error) {
	w := wire.Writer(b)
	d.Put(&w)
	return w, nil
}

// UnmarshalBinary reads a delta's bytes. Bytes that are no delta are an
// error; a delta read that does not fit what it is applied to is Apply's.
func (d *Delta) UnmarshalBinary(b []byte) error {
	r := wire.NewReader(b)
	var out Delta
	out.Get(r)
	if err := r.End(); err != nil {
		return fmt.Errorf("cell: a delta's bytes: %w", err)
	}
	*d = out
	return nil
}

// Put puts d's bytes on w: a codec that carries a delta puts it among its
// own fields.
func (d Delta) Put(w *wire.Writer) {
	flags := byte(0)
	if d.Whole {
		flags |= 1
	}
	w.Byte(flags)
	w.Int(d.Cols)
	w.Int(d.Rows)
	putTable(w, d.Attrs)
	w.Int(len(d.Scrolls))
	for _, s := range d.Scrolls {
		w.Int(s.Top)
		w.Int(s.Bottom)
		w.SignedInt(s.N)
		f := byte(0)
		if s.History {
			f |= 1
		}
		if s.Unseen != nil {
			f |= 2
		}
		w.Byte(f)
		if s.Unseen != nil {
			w.Int(len(s.Unseen))
			for _, r := range s.Unseen {
				w.Int(r.From)
				w.Int(r.N)
			}
		}
	}
	w.Int(len(d.Spans))
	for _, sp := range d.Spans {
		w.Int(sp.Row)
		w.Int(sp.Col)
		w.Int(sp.Before)
		putRuns(w, sp.Runs)
	}
}

// Get gets what Put put into d, r keeping what it could not read; d is
// whatever was read by then.
func (d *Delta) Get(r *wire.Reader) {
	var out Delta
	out.Whole = r.Byte()&1 != 0
	out.Cols, out.Rows = r.Int(maxCells), r.Int(maxCells)
	out.Attrs = getTable(r)
	if n := r.Count(); n > 0 && r.Err() == nil {
		out.Scrolls = make([]Scroll, 0, n)
		for range n {
			s := Scroll{Top: r.Int(maxCells), Bottom: r.Int(maxCells), N: r.SignedInt(maxCells)}
			f := r.Byte()
			s.History = f&1 != 0
			if f&2 != 0 {
				s.Unseen = getUnseen(r)
			}
			if r.Err() != nil {
				break
			}
			out.Scrolls = append(out.Scrolls, s)
		}
	}
	if n := r.Count(); n > 0 && r.Err() == nil {
		out.Spans = make([]Span, 0, n)
		for range n {
			sp := Span{Row: r.Int(maxCells), Col: r.Int(maxCells), Before: r.Int(maxCells)}
			sp.Runs = getRuns(r)
			if r.Err() != nil {
				break
			}
			out.Spans = append(out.Spans, sp)
		}
	}
	*d = out
}

func putColor(w *wire.Writer, c Color) {
	w.Byte(byte(c.Mode))
	w.Byte(c.Index)
	w.Uvarint(uint64(c.RGB))
}

func getColor(r *wire.Reader) Color {
	c := Color{Mode: ColorMode(r.Byte()), Index: r.Byte()}
	v := r.Uvarint()
	if v > math.MaxUint32 {
		r.Fail(wire.ErrBytes)
	}
	c.RGB = uint32(v)
	return c
}

func putTable(w *wire.Writer, cells []Cell) {
	w.Int(len(cells))
	for _, c := range cells {
		a := c.Attr
		bits := uint64(0)
		for _, f := range []struct {
			on  bool
			bit uint64
		}{
			{a.Bold, bitBold}, {a.Faint, bitFaint}, {a.Italics, bitItalics}, {a.Underline, bitUnderline},
			{a.Strikethrough, bitStrikethrough}, {a.Reverse, bitReverse}, {a.Blink, bitBlink},
			{a.Conceal, bitConceal}, {a.Protected, bitProtected}, {a.ISOProtected, bitISOProtected},
			{a.Overline, bitOverline}, {c.Wrapped, bitWrapped}, {c.Empty, bitEmpty},
			{a.Fg != (Color{}), bitFg}, {a.Bg != (Color{}), bitBg},
			{a.UnderlineColor != (Color{}), bitUnderlineColor}, {a.UnderlineStyle != 0, bitUnderlineStyle},
		} {
			if f.on {
				bits |= f.bit
			}
		}
		w.Uvarint(bits)
		if bits&bitFg != 0 {
			putColor(w, a.Fg)
		}
		if bits&bitBg != 0 {
			putColor(w, a.Bg)
		}
		if bits&bitUnderlineColor != 0 {
			putColor(w, a.UnderlineColor)
		}
		if bits&bitUnderlineStyle != 0 {
			w.Byte(a.UnderlineStyle)
		}
	}
}

func getTable(r *wire.Reader) []Cell {
	n := r.Count()
	if n == 0 || r.Err() != nil {
		return nil
	}
	cells := make([]Cell, 0, n)
	for range n {
		bits := r.Uvarint()
		var c Cell
		a := &c.Attr
		a.Bold, a.Faint, a.Italics = bits&bitBold != 0, bits&bitFaint != 0, bits&bitItalics != 0
		a.Underline, a.Strikethrough, a.Reverse = bits&bitUnderline != 0, bits&bitStrikethrough != 0, bits&bitReverse != 0
		a.Blink, a.Conceal, a.Protected = bits&bitBlink != 0, bits&bitConceal != 0, bits&bitProtected != 0
		a.ISOProtected, a.Overline = bits&bitISOProtected != 0, bits&bitOverline != 0
		c.Wrapped, c.Empty = bits&bitWrapped != 0, bits&bitEmpty != 0
		if bits&bitFg != 0 {
			a.Fg = getColor(r)
		}
		if bits&bitBg != 0 {
			a.Bg = getColor(r)
		}
		if bits&bitUnderlineColor != 0 {
			a.UnderlineColor = getColor(r)
		}
		if bits&bitUnderlineStyle != 0 {
			a.UnderlineStyle = r.Byte()
		}
		cells = append(cells, c)
	}
	return cells
}

func putRuns(w *wire.Writer, runs []Run) {
	w.Int(len(runs))
	for _, r := range runs {
		switch {
		case r.T != "":
			w.Uvarint(uint64(r.A)<<2 | binText)
			w.String(r.T)
		case r.C != nil:
			w.Uvarint(uint64(r.A)<<2 | binCells)
			w.Int(len(r.C))
			for _, s := range r.C {
				w.String(s)
			}
		default:
			w.Uvarint(uint64(r.A)<<2 | binNone)
			w.Int(r.N)
		}
	}
}

func getRuns(r *wire.Reader) []Run {
	n := r.Count()
	if n == 0 || r.Err() != nil {
		return nil // none is nil, as the JSON reads it
	}
	runs := make([]Run, 0, n)
	for range n {
		head := r.Uvarint()
		if head>>2 > maxCells {
			r.Fail(wire.ErrBytes)
		}
		run := Run{A: int(head >> 2)}
		switch head & 3 {
		case binText:
			run.T = r.String()
			if run.T == "" {
				r.Fail(wire.ErrBytes) // a text run has text: none is an N run
			}
		case binNone:
			run.N = r.Int(maxCells)
		case binCells:
			k := r.Count()
			if r.Err() != nil {
				return nil
			}
			run.C = make([]string, 0, k)
			for range k {
				run.C = append(run.C, r.String())
			}
		default:
			r.Fail(wire.ErrBytes)
		}
		if r.Err() != nil {
			return nil
		}
		runs = append(runs, run)
	}
	return runs
}

// getUnseen is a scroll's runs of rows unseen: at least one, each two
// numbers.
func getUnseen(r *wire.Reader) []Rows {
	n := r.Count()
	if n == 0 {
		r.Fail(wire.ErrBytes) // said, so some
	}
	if r.Err() != nil {
		return nil
	}
	out := make([]Rows, 0, n)
	for range n {
		out = append(out, Rows{From: r.Int(maxCells), N: r.Int(maxCells)})
	}
	return out
}
