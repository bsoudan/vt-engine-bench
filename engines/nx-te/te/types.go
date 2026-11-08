package te

import "nx/cell"

// The cell model is package cell's; these aliases keep it spelled te.Cell here,
// since the emulator's screens are built out of it.
type (
	ColorMode = cell.ColorMode
	Color     = cell.Color
	Attr      = cell.Attr
	Cell      = cell.Cell
)

const (
	ColorDefault   = cell.ColorDefault
	ColorANSI16    = cell.ColorANSI16
	ColorANSI256   = cell.ColorANSI256
	ColorTrueColor = cell.ColorTrueColor
)

// Cursor tracks cursor position and attributes.
type Cursor struct {
	Row    int
	Col    int
	Attr   Attr
	Hidden bool
}
