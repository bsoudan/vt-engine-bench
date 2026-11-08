package te

import "strings"

// Text is the text of a run of cells in reading order, as a selection copies
// it: rows[0] from column from on, whole rows between, the last row up to
// column to, inclusive. A wide character is its glyph once (its second
// column holds nothing), trailing blanks are dropped from each line, and
// rows are joined by line breaks — except where the terminal wrapped a line
// (its last cell is Wrapped), which is joined to the next as it was written:
// its blanks kept, and the Empty cells at its end, which the line never
// filled, left out. So a line copies the same after the screen is widened, or
// characters are deleted from it, as before.
func Text(rows [][]Cell, from, to int) string {
	var b strings.Builder
	for i, row := range rows {
		start, end := 0, len(row)-1
		if i == 0 {
			start = max(from, 0)
		}
		if i == len(rows)-1 {
			end = min(to, len(row)-1)
		}
		wrapped := i < len(rows)-1 && end == len(row)-1 && end >= 0 && row[end].Wrapped
		if wrapped {
			for end >= start && row[end].Empty {
				end--
			}
		}
		var line strings.Builder
		for c := start; c <= end; c++ {
			line.WriteString(row[c].Data) // a wide glyph's second column is ""
		}
		if wrapped {
			b.WriteString(line.String())
			continue
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
