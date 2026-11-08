package cell

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// A grid comes back from its JSON as it went: text of every kind,
// attributes, transparent and wide cells, wrapped and empty ones, no grid,
// no rows, a row that is none and one that is empty.
func TestAGridComesBackAsItWent(t *testing.T) {
	red := Attr{Fg: Color{Mode: ColorANSI16, Index: 1}, Bold: true}
	rgb := Attr{Bg: Color{Mode: ColorANSI256, Index: 200}, Underline: true, UnderlineStyle: 3}
	row := func(cells ...Cell) []Cell { return cells }
	for _, tc := range []struct {
		name string
		grid Grid
	}{
		{"no grid", nil},
		{"no rows", Grid{}},
		{"plain", Grid{BlankRow(5), row(Cell{Data: "h"}, Cell{Data: "i"})}},
		{"attributes", Grid{row(Cell{Data: "a", Attr: red}, Cell{Data: "b", Attr: red}, Cell{Data: "c"}, Cell{Data: "d", Attr: rgb})}},
		{"wide and combining", Grid{row(Cell{Data: "表"}, Cell{}, Cell{Data: "é"}, Cell{Data: "👍🏽"}, Cell{Data: "x"})}},
		{"transparent", Grid{row(Cell{}, Cell{}, Cell{Data: "x"}, Cell{}, Cell{Attr: red})}},
		{"wrapped and empty", Grid{row(Cell{Data: "w"}, Cell{Data: "x", Wrapped: true}), row(Cell{Data: "y"}, Cell{Data: " ", Empty: true})}},
		{"a row that is none, and one that is empty", Grid{nil, {}, BlankRow(2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.grid)
			if err != nil {
				t.Fatal(err)
			}
			var got Grid
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("%s: %v", b, err)
			}
			if !reflect.DeepEqual(got, tc.grid) {
				t.Fatalf("came back\n%#v\nwent\n%#v\nas %s", got, tc.grid, b)
			}
		})
	}
}

// A grid's JSON reads as its text: a run of cells alike is its text, its
// attributes said once in the table, and it is a fraction of an object per
// cell.
func TestAGridsJSONIsItsText(t *testing.T) {
	cyan := Attr{Bold: true, Fg: Color{Mode: ColorANSI16, Index: 14}}
	var band []Cell
	for _, part := range []struct {
		text string
		a    Attr
	}{{"• ", Attr{}}, {"«1▪sh»", cyan}, {" •" + strings.Repeat("·", 160) + "• main@box •", Attr{}}} {
		for _, r := range part.text {
			band = append(band, Cell{Data: string(r), Attr: part.a})
		}
	}
	runs, err := json.Marshal(Grid{band})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runs), `{"T":"«1▪sh»","A":1}`) || strings.Count(string(runs), `"Bold":true`) != 1 {
		t.Errorf("the band's JSON does not read as its runs:\n%s", runs)
	}
	perCell, _ := json.Marshal([][]Cell{band})
	if len(runs)*5 > len(perCell) {
		t.Errorf("runs are %d bytes, an object per cell %d: want a fifth or less", len(runs), len(perCell))
	}
}

// Whatever grid is made, it comes back as it went; whatever JSON comes, it
// is read without panicking.
func FuzzGrid(f *testing.F) {
	f.Add([]byte("hello\x00world"), []byte{1, 2, 0, 3})
	f.Add([]byte("表é"), []byte{0, 0, 9})
	f.Fuzz(func(t *testing.T, text, shape []byte) {
		if !utf8.Valid(text) {
			return // JSON says invalid UTF-8 as U+FFFD, in this form or any
		}
		// shape says each cell: its attribute, whether it ends its row, and
		// whether it has no text.
		var g Grid
		var row []Cell
		chars := []rune(string(text))
		for i, s := range shape {
			c := Cell{Attr: Attr{Bold: s&1 != 0, Fg: Color{Mode: ColorANSI16, Index: s >> 5}}, Wrapped: s&2 != 0}
			if s&4 == 0 && len(chars) > 0 {
				c.Data = string(chars[i%len(chars)])
				if s&8 != 0 {
					c.Data += string(chars[(i+1)%len(chars)]) // two characters: a C run
				}
			}
			row = append(row, c)
			if s&16 != 0 {
				g, row = append(g, row), nil
			}
		}
		g = append(g, row)
		b, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}
		var got Grid
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("%s: %v", b, err)
		}
		if !reflect.DeepEqual(got, g) {
			t.Fatalf("came back\n%#v\nwent\n%#v\nas %s", got, g, b)
		}
		var any Grid
		_ = json.Unmarshal(text, &any)
	})
}
