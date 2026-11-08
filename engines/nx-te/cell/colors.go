package cell

import (
	"fmt"
	"strconv"
	"strings"
)

// Colors are the default foreground and background of the terminal a grid is
// drawn into, and its 16 ANSI colors, as xterm spells a color
// ("rgb:rrrr/gggg/bbbb", with one to four hex digits a component); "" is one
// not known. A program asks for them (OSC 10 and 11; OSC 4 for the 16) to
// tell a light theme from a dark one, or to draw in the theme's own colors,
// and a shadow screen can only answer with the real terminal's: the console
// asks it, a window knows its own, and each hands the answer on to every
// screen it shadows.
type Colors struct {
	Fg, Bg string
	ANSI   [16]string `json:",omitzero"`
}

// Light says the background is known and nearer white than black, by its
// luma: a light theme. A background not known is taken for a dark one.
func (c Colors) Light() bool {
	l, ok := c.luma()
	return ok && l >= 0.5
}

// luma is the background's luma, 0 black to 1 white, and whether it is
// known.
func (c Colors) luma() (float64, bool) {
	bg, ok := ParseSpec(c.Bg)
	r, g, b := float64(bg.RGB>>16&0xff), float64(bg.RGB>>8&0xff), float64(bg.RGB&0xff)
	return (0.2126*r + 0.7152*g + 0.0722*b) / 255, ok
}

// ParseSpec is a color as xterm spells it in its OSC color sequences
// ("rgb:rrrr/gggg/bbbb", one to four hex digits a component, each scaled
// to eight bits, rounded) as a true color, and whether s is one. The one
// reader of a spec: the terminal's colors as it answers (Colors), a
// picture's margin (cores/view), a program's answer (te).
func ParseSpec(s string) (Color, bool) {
	rest, ok := strings.CutPrefix(s, "rgb:")
	parts := strings.Split(rest, "/")
	if !ok || len(parts) != 3 {
		return Color{}, false
	}
	var rgb uint32
	for _, p := range parts {
		n, err := strconv.ParseUint(p, 16, 16)
		if err != nil || len(p) < 1 || len(p) > 4 {
			return Color{}, false
		}
		most := uint64(1)<<(4*len(p)) - 1 // one to four hex digits
		rgb = rgb<<8 | uint32((n*255+most/2)/most)
	}
	return Color{Mode: ColorTrueColor, RGB: rgb}, true
}

// Spec is the color's red, green and blue (RGB, a true color's) as xterm
// spells a color, sixteen bits a component: "rgb:f5f5/c2c2/1111".
// ParseSpec reads it back.
func (c Color) Spec() string {
	r, g, b := c.RGB>>16&0xff, c.RGB>>8&0xff, c.RGB&0xff
	return fmt.Sprintf("rgb:%02x%02x/%02x%02x/%02x%02x", r, r, g, g, b, b)
}

// Shade is the ground of a selected row: a grey from the 256-color ramp a
// shade off the background — lighter on a dark one, darker on a light one —
// so the row reads as the background raised, in any terminal with 256
// colors. A background not known is taken for a dark one: 236.
func (c Colors) Shade() Color { return c.ShadeBy(24) }

// RampStep is one step of the 256-color grey ramp, of 255.
const RampStep = 10

// ShadeBy is a grey from the 256-color ramp off the background by off, of
// 255 — lighter on a dark one, darker on a light one — the nearest the ramp
// has. Shade is ShadeBy(24), a row's ground; ShadeBy(RampStep) is one step
// off, nearer the background: the editor's cursor's line. A background not
// known is taken for a dark one, #1c1c1c (234): Shade is 236 off it.
func (c Colors) ShadeBy(off int) Color {
	l, ok := c.luma()
	if !ok {
		l = 28.0 / 255
	}
	target := l*255 + float64(off)
	if l >= 0.5 {
		target = l*255 - float64(off)
	}
	// The ramp is 232 to 255, grey 8 to 238 in steps of 10.
	k := min(max(int((target-8)/10+0.5), 0), 23)
	return Color{Mode: ColorANSI256, Index: uint8(232 + k)}
}

// Empty is cell (r, c) of an area nothing draws — a session's area with
// every vtapp in the background, or what a vtapp's program has not drawn yet: a
// centered × in a checkerboard, in a grey from the 256-color ramp a step or
// two off the terminal's background (238 on a dark one, 250 on a light).
func (c Colors) Empty(r, col int) Cell {
	if (r+col)%2 != 0 {
		return Cell{Data: " "}
	}
	grey := uint8(238)
	if c.Light() {
		grey = 250
	}
	return Cell{Data: "×", Attr: Attr{Fg: Color{Mode: ColorANSI256, Index: grey}}}
}
