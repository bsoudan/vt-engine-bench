// Package cell is the cell model shared by everything that paints or composites
// a grid: a Cell is one glyph with its attributes and colors. It is what the
// "surface" interface carries and what the compositor and the chrome draw with,
// and it depends on nothing — the terminal emulator (package te) builds its
// screens out of these same types, so a tier with no emulation (the Shell, the
// chrome) need not import one.
package cell

import "strings"

// ColorMode identifies a color encoding mode.
type ColorMode uint8

const (
	// ColorDefault selects default terminal colors.
	ColorDefault ColorMode = iota
	// ColorANSI16 selects the 16-color palette.
	ColorANSI16
	// ColorANSI256 selects the 256-color palette.
	ColorANSI256
	// ColorTrueColor selects 24-bit true color.
	ColorTrueColor
)

// Color describes a terminal color: Mode says which palette, Index the entry
// in it (the 16 or the 256), and RGB the color itself for true color, packed
// 0xRRGGBB. The zero Color is the default color.
type Color struct {
	Mode  ColorMode `json:"Mode,omitempty"`
	Index uint8     `json:"Index,omitempty"`
	RGB   uint32    `json:"RGB,omitempty"`
}

// Attr describes character attributes and colors.
type Attr struct {
	Fg            Color `json:"Fg,omitzero"`
	Bg            Color `json:"Bg,omitzero"`
	Bold          bool  `json:"Bold,omitempty"`
	Faint         bool  `json:"Faint,omitempty"`
	Italics       bool  `json:"Italics,omitempty"`
	Underline     bool  `json:"Underline,omitempty"`
	Strikethrough bool  `json:"Strikethrough,omitempty"`
	Reverse       bool  `json:"Reverse,omitempty"`
	Blink         bool  `json:"Blink,omitempty"`
	Conceal       bool  `json:"Conceal,omitempty"`
	Protected     bool  `json:"Protected,omitempty"`    // DECSCA: erasure by DECSED/DECSEL leaves it
	ISOProtected  bool  `json:"ISOProtected,omitempty"` // SPA/EPA: ISO 6429 guarded area, which ED/EL respect too
	// UnderlineStyle is the underline variant when Underline is set:
	// 1=single, 2=double, 3=curly, 4=dotted, 5=dashed (SGR 4:n / 21).
	UnderlineStyle uint8 `json:"UnderlineStyle,omitempty"`
	// UnderlineColor is the SGR 58 underline color; default (zero) means the
	// underline uses the text foreground color.
	UnderlineColor Color `json:"UnderlineColor,omitzero"`
	// Overline is SGR 53.
	Overline bool `json:"Overline,omitempty"`
}

// Cell represents a single screen cell.
type Cell struct {
	Data string `json:"Data,omitempty"`
	Attr Attr   `json:"Attr,omitzero"`
	// Wrapped, on a row's last cell, says the terminal wrapped the line here:
	// the text goes on at the start of the next row, rather than ending with
	// a line break. It is the row's, kept on the cell so it goes wherever the
	// row goes — scrolled, into the history, in a snapshot — and goes when
	// the cell is written over or erased. A copy joins what it joins.
	Wrapped bool `json:"Wrapped,omitempty"`
	// Empty says the cell holds no text, though it shows as a blank: space
	// at the end of a wrapped row that the line never filled — the columns
	// a widening added, the ones characters deleted out of the row left, the
	// one a wide glyph that did not fit left behind. A copy of a wrapped row
	// leaves the Empty cells at its end out, so it is the line as written
	// whatever has been done to the row's width since.
	Empty bool `json:"Empty,omitempty"`
}

// Blank says the cell shows nothing and keeps nothing: a space in the default
// attributes, not the end of a wrapped row nor Empty space. What an erase
// leaves is blank, so a renderer need not write it and a snapshot need not
// carry it.
func (c Cell) Blank() bool {
	return c.Data == " " && c.Attr == (Attr{}) && !c.Wrapped && !c.Empty
}

// BlankRow is a row of cols blank cells: a space in default attributes each.
func BlankRow(cols int) []Cell {
	row := make([]Cell, cols)
	for i := range row {
		row[i] = Cell{Data: " "}
	}
	return row
}

// Text is a row's text as it reads: each cell's Data in turn. A cell with
// nothing in it (the second column of a wide glyph, a transparent cell)
// adds nothing.
func Text(row []Cell) string {
	var b strings.Builder
	for _, c := range row {
		b.WriteString(c.Data)
	}
	return b.String()
}

// Lines is a grid's text, a line to a row (Text).
func Lines(grid [][]Cell) []string {
	out := make([]string, len(grid))
	for i, row := range grid {
		out[i] = Text(row)
	}
	return out
}

// CursorStyle is the cursor's style as a program asks for it, by DECSCUSR's
// number: 0 the terminal's default (no style asked for), 1 and 2 a block, 3
// and 4 an underline, 5 and 6 a bar; the odd ones blink, as does 0, the
// default being xterm's blinking block. The zero CursorStyle is the default.
type CursorStyle uint8

// CursorShape is the shape a CursorStyle draws.
type CursorShape uint8

// The cursor's shapes.
const (
	CursorBlock CursorShape = iota
	CursorUnderline
	CursorBar
)

// CursorStyleOf is DECSCUSR's n as a CursorStyle: one it doesn't define is
// the default.
func CursorStyleOf(n int) CursorStyle {
	if n < 0 || n > 6 {
		return 0
	}
	return CursorStyle(n)
}

// Shape is the shape s draws: the default is a block.
func (s CursorStyle) Shape() CursorShape {
	switch s {
	case 3, 4:
		return CursorUnderline
	case 5, 6:
		return CursorBar
	}
	return CursorBlock
}

// Blink says whether s asks for the cursor to blink. A frontend blinks it
// only if its desktop or terminal allows it.
func (s CursorStyle) Blink() bool { return s == 0 || s%2 == 1 }

// Steady is s's shape without the blink: a steady block for the default. A
// cursor showing a stalled link is drawn so, by both frontends: the
// terminal's (cores/tty) and the window's (gui.Steady).
func (s CursorStyle) Steady() CursorStyle {
	return [...]CursorStyle{CursorBlock: 2, CursorUnderline: 4, CursorBar: 6}[s.Shape()]
}

// String is s as a test or a failure says it: "default", or its shape and
// whether it blinks ("steady bar").
func (s CursorStyle) String() string {
	if s == 0 {
		return "default"
	}
	shape := [...]string{CursorBlock: "block", CursorUnderline: "underline", CursorBar: "bar"}[s.Shape()]
	if s.Blink() {
		return "blinking " + shape
	}
	return "steady " + shape
}

// CursorGlyph is the cell a view draws where its cursor is, in place of the
// real cursor, while the cursor says something: the link to what it shows
// lost (an X, reverse red), or the prefix pending, a command coming (a ?,
// bold reverse bright cyan). ok is false for the real cursor.
func CursorGlyph(connected, armed bool) (c Cell, ok bool) {
	switch {
	case !connected:
		return Cell{Data: "X", Attr: Attr{Reverse: true, Fg: Color{Mode: ColorANSI16, Index: 1}}}, true
	case armed:
		return Cell{Data: "?", Attr: Attr{Bold: true, Reverse: true, Fg: Color{Mode: ColorANSI16, Index: 14}}}, true
	}
	return Cell{}, false
}
