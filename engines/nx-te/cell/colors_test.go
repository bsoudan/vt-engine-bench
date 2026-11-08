package cell

import (
	"math"
	"testing"
)

// A selected row's shade is a grey of the 256-color ramp a shade off the
// background: lighter on a dark one, darker on a light one, a step or two
// either way, following it; a background not known is taken for a dark
// one's.
func TestTheShadeIsOffTheBackground(t *testing.T) {
	for _, c := range []struct {
		bg   string
		want uint8
	}{
		{"", 236},                   // not known
		{"rgb:0000/0000/0000", 234}, // black: #1c1c1c
		{"rgb:1e1e/1e1e/1e1e", 237}, // GNOME Adwaita dark: #3a3a3a
		{"rgb:2e2e/3434/4040", 239}, // Nord: #4e4e4e
		{"rgb:ffff/ffff/ffff", 254}, // white: #e4e4e4
		{"rgb:fdfd/f6f6/e3e3", 253}, // Solarized light: #dadada
	} {
		got := Colors{Bg: c.bg}.Shade()
		if got != (Color{Mode: ColorANSI256, Index: c.want}) {
			t.Errorf("on %q: %+v, want 256-color %d", c.bg, got, c.want)
		}
		if by := (Colors{Bg: c.bg}).ShadeBy(24); by != got {
			t.Errorf("on %q: ShadeBy(24) is %+v, not Shade's %+v", c.bg, by, got)
		}
		// Lighter on dark, darker on light.
		if l, ok := (Colors{Bg: c.bg}).luma(); ok {
			grey := float64(8+10*int(got.Index-232)) / 255
			if light := l >= 0.5; light && grey >= l || !light && grey <= l {
				t.Errorf("on %q (luma %.2f): grey %.2f is not off it the right way", c.bg, l, grey)
			}
		}
	}
}

// One ramp step off the background (the editor's cursor's line) is a grey
// between the background and its Shade: off it the same way, nearer it,
// and neither — so the line shows, and a selected row's shade still stands
// out from it.
func TestOneRampStepIsNearerTheBackgroundThanTheShade(t *testing.T) {
	for _, c := range []struct {
		bg   string
		want uint8
	}{
		{"", 235},                   // not known: a dark one's
		{"rgb:0000/0000/0000", 232}, // black: #080808
		{"rgb:1e1e/1e1e/1e1e", 235}, // GNOME Adwaita dark: #262626
		{"rgb:2e2e/3434/4040", 237}, // Nord: #3a3a3a
		{"rgb:ffff/ffff/ffff", 255}, // white: #eeeeee
		{"rgb:fdfd/f6f6/e3e3", 255}, // Solarized light: #eeeeee
	} {
		colors := Colors{Bg: c.bg}
		got := colors.ShadeBy(RampStep)
		if got != (Color{Mode: ColorANSI256, Index: c.want}) {
			t.Errorf("on %q: %+v, want 256-color %d", c.bg, got, c.want)
		}
		grey := func(c Color) float64 { return float64(8+10*int(c.Index-232)) / 255 }
		line, shade := grey(got), grey(colors.Shade())
		l, ok := colors.luma()
		if !ok {
			l = 28.0 / 255
		}
		if line == shade || line == l || (line-l)*(shade-l) <= 0 || math.Abs(line-l) >= math.Abs(shade-l) {
			t.Errorf("on %q (luma %.3f): one step %.3f is not between it and its shade %.3f", c.bg, l, line, shade)
		}
	}
}

// A color as xterm spells it is its eight bits a component, whatever its
// digits, rounded; anything else is none. Spec spells it back, sixteen
// bits a component, and reads back the same.
func TestASpecIsAColor(t *testing.T) {
	for s, want := range map[string][4]int{
		"rgb:1c1c/1c1c/1c1c": {0x1c, 0x1c, 0x1c, 1}, "rgb:f/8/0": {0xff, 0x88, 0, 1},
		"rgb:ffff/0000/8080": {0xff, 0, 0x80, 1}, "rgb:abc/80ff/12": {0xab, 0x80, 0x12, 1}, "rgb:abc/8180/12": {0xab, 0x81, 0x12, 1},
		"": {0, 0, 0, 0}, "#ffffff": {0, 0, 0, 0}, "rgb:1/2": {0, 0, 0, 0}, "rgb:12345/0/0": {0, 0, 0, 0}, "rgb:/0/0": {0, 0, 0, 0}, "rgb:g/0/0": {0, 0, 0, 0},
	} {
		c, ok := ParseSpec(s)
		got := [4]int{int(c.RGB >> 16), int(c.RGB >> 8 & 0xff), int(c.RGB & 0xff), map[bool]int{true: 1}[ok]}
		if got != want || ok && c.Mode != ColorTrueColor {
			t.Errorf("ParseSpec(%q) is %v, %+v; want %v", s, got, c, want)
		}
		if !ok {
			continue
		}
		if back, ok := ParseSpec(c.Spec()); !ok || back != c {
			t.Errorf("%q is spelled %q, which reads back as %+v", s, c.Spec(), back)
		}
	}
	if s := (Color{Mode: ColorTrueColor, RGB: 0xf5c211}).Spec(); s != "rgb:f5f5/c2c2/1111" {
		t.Errorf("0xf5c211 is spelled %q", s)
	}
}
