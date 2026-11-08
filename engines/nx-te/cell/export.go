package cell

// What a differ outside this package needs of a delta's making and
// applying, for one that compares cells in another form (te's, which
// stores a cell in eight bytes) and so cannot use Differ: the table and
// runs a span is built from, the checks and the order Apply uses, and
// which rows a scroll says are unseen. The logic stays here, once.

// Table numbers the entries a delta's or grid's runs name: each cell
// without its text, once, in the order first seen.
type Table struct{ t *table }

// NewTable is an empty table.
func NewTable() *Table { return &Table{newTable(nil)} }

// Runs is row as runs of t's entries, added as they are new.
func (t *Table) Runs(row []Cell) []Run { return runsOf(row, t.t) }

// Cells is the entries so far, as a delta's Attrs carries them: nil for none.
func (t *Table) Cells() []Cell { return t.t.cells }

// SpanGap is how many cells alike may lie between two that differ in a row
// and still be one span.
const SpanGap = spanGap

// Check says whether d is well made (ErrDelta when not).
func (d Delta) Check() error { return d.check() }

// Each does d's scrolls and spans in their order: each span after the
// scrolls before it.
func (d Delta) Each(scroll func(Scroll), span func(Span)) { d.each(scroll, span) }

// Entry is the table entry a run names: a cell without text.
func (d Delta) Entry(a int) (Cell, error) { return entryOf(d.Attrs, a) }

// IsUnseen says whether row i of the rows s moves off is one Unseen says.
func (s Scroll) IsUnseen(i int) bool { return s.unseen(i) }

// Check says whether s is well made for a grid of cols×rows (ErrDelta when
// not): what Delta.Check asks of each scroll.
func (s Scroll) Check(cols, rows int) error { return s.checkIn(cols, rows) }
