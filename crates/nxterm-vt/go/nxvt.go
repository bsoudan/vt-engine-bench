// Benchmark-only C entry points over nxterm's terminal engine (nx/te), built as
// a C archive by ../build.rs and linked into the Rust benchmark. Like the
// Ghostty shim it exposes only what the benchmark and the compatibility
// fixtures use; it is not a general binding.
package main

/*
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>

typedef struct {
	uint8_t r, g, b;
	uint8_t valid; // 0 for the default color
	uint8_t bold;
	uint8_t mode;  // nx/cell.ColorMode: 0 default, 1 ANSI 16, 2 ANSI 256, 3 true color
	uint8_t index; // the palette entry for modes 1 and 2
} nxvt_color;
*/
import "C"

import (
	"strings"
	"sync"
	"unsafe"

	"nx/cell"
	"nx/te"
)

const history = 1000 // scrollback lines, as the other engines are configured

type term struct {
	screen *te.ScrollbackScreen
	stream *te.Stream
}

var (
	mu    sync.Mutex
	terms = map[uintptr]*term{}
	next  uintptr = 1
)

func get(h C.uintptr_t) *term {
	mu.Lock()
	defer mu.Unlock()
	return terms[uintptr(h)]
}

//export nxvt_new
func nxvt_new(cols, rows C.int) C.uintptr_t {
	screen := te.NewScrollbackScreen(int(cols), int(rows), history)
	t := &term{screen: screen, stream: te.NewStream(screen)}
	mu.Lock()
	defer mu.Unlock()
	h := next
	next++
	terms[h] = t
	return C.uintptr_t(h)
}

//export nxvt_free
func nxvt_free(h C.uintptr_t) {
	mu.Lock()
	defer mu.Unlock()
	delete(terms, uintptr(h))
}

//export nxvt_feed
func nxvt_feed(h C.uintptr_t, data *C.char, n C.size_t) {
	t := get(h)
	t.stream.FeedBytes(C.GoBytes(unsafe.Pointer(data), C.int(n)))
}

//export nxvt_resize
func nxvt_resize(h C.uintptr_t, cols, rows C.int) {
	get(h).screen.Resize(int(rows), int(cols))
}

//export nxvt_cursor
func nxvt_cursor(h C.uintptr_t, col, row *C.int) {
	c := get(h).screen.Cursor
	*col, *row = C.int(c.Col), C.int(c.Row)
}

// rowText is te.Screen.Display's reading of a row, with a space for every
// empty cell so a column position means the same as in the other engines.
func rowText(s *te.Screen, row []cell.Cell) string {
	var b strings.Builder
	col := 0
	for col < len(row) {
		c := row[col]
		if c.Data == "" {
			b.WriteByte(' ')
			col++
			continue
		}
		b.WriteString(c.Data)
		if w := s.Widths.Width(c.Data); w > 1 {
			col += w
		} else {
			col++
		}
	}
	return b.String()
}

func color(c cell.Color, bold bool) C.nxvt_color {
	out := C.nxvt_color{mode: C.uint8_t(c.Mode), index: C.uint8_t(c.Index)}
	if bold {
		out.bold = 1
	}
	if c.Mode != cell.ColorDefault {
		out.valid = 1
	}
	if c.Mode == cell.ColorTrueColor {
		out.r, out.g, out.b = C.uint8_t(c.RGB>>16), C.uint8_t(c.RGB>>8), C.uint8_t(c.RGB)
	}
	return out
}

// nxvt_snapshot returns the visible rows joined by '\n' (free with
// nxvt_free_string) and the colors of the top-left cell.
//
//export nxvt_snapshot
func nxvt_snapshot(h C.uintptr_t, fg, bg *C.nxvt_color) *C.char {
	s := get(h).screen.Screen
	rows := make([]string, len(s.Buffer))
	for i, row := range s.Buffer {
		rows[i] = rowText(s, row)
	}
	if len(s.Buffer) > 0 && len(s.Buffer[0]) > 0 {
		a := s.Buffer[0][0].Attr
		*fg, *bg = color(a.Fg, a.Bold), color(a.Bg, a.Bold)
	}
	return C.CString(strings.Join(rows, "\n"))
}

// nxvt_history returns the scrollback rows, oldest first, joined by '\n'.
//
//export nxvt_history
func nxvt_history(h C.uintptr_t) *C.char {
	t := get(h)
	var rows []string
	for _, row := range t.screen.ScrollbackRows() {
		rows = append(rows, rowText(t.screen.Screen, row))
	}
	return C.CString(strings.Join(rows, "\n"))
}

//export nxvt_free_string
func nxvt_free_string(s *C.char) {
	C.free(unsafe.Pointer(s))
}

func main() {}
