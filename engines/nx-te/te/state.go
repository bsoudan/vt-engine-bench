package te

import (
	"maps"
	"slices"

	"nx/cell"
)

// ScreenState is a fully serializable snapshot of a Screen's state.
// All fields are exported and use JSON-friendly types (map[int]bool
// instead of map[int]struct{}); the rows are cell.Grid, which says a row
// as runs of text, as a frame carries it.
type ScreenState struct {
	Columns int       `json:"columns"`
	Lines   int       `json:"lines"`
	Buffer  cell.Grid `json:"buffer"`

	AltBuffer      cell.Grid   `json:"alt_buffer,omitempty"`
	AltActive      bool        `json:"alt_active,omitempty"`
	AltSavepoints  []Savepoint `json:"alt_savepoints,omitempty"`
	AltWrapNext    bool        `json:"alt_wrap_next,omitempty"`
	AltSavedCursor *Cursor     `json:"alt_saved_cursor,omitempty"`

	Cursor     Cursor       `json:"cursor"`
	Mode       map[int]bool `json:"mode,omitempty"`
	TabStops   map[int]bool `json:"tab_stops,omitempty"`
	SavedModes map[int]bool `json:"saved_modes,omitempty"`
	Margins    *Margins     `json:"margins,omitempty"`

	LeftMargin          int  `json:"left_margin,omitempty"`
	RightMargin         int  `json:"right_margin,omitempty"`
	CursorStyle         int  `json:"cursor_style,omitempty"`
	CharProtectionMode  int  `json:"char_protection_mode,omitempty"`
	StatusDisplay       int  `json:"status_display,omitempty"`
	ConformanceLevel    int  `json:"conformance_level,omitempty"`
	ConformanceExplicit bool `json:"conformance_explicit,omitempty"`
	IndexedColors       int  `json:"indexed_colors,omitempty"`

	Charset int    `json:"charset,omitempty"`
	G0      []rune `json:"g0,omitempty"`
	G1      []rune `json:"g1,omitempty"`

	Savepoints   []Savepoint `json:"savepoints,omitempty"`
	SavedColumns *int        `json:"saved_columns,omitempty"`

	WrapNext  bool   `json:"wrap_next,omitempty"`
	LastDrawn string `json:"last_drawn,omitempty"`

	Title          string   `json:"title,omitempty"`
	IconName       string   `json:"icon_name,omitempty"`
	TitleStack     []string `json:"title_stack,omitempty"`
	IconStack      []string `json:"icon_stack,omitempty"`
	TitleHexInput  bool     `json:"title_hex_input,omitempty"`
	TitleHexOutput bool     `json:"title_hex_output,omitempty"`

	ColorPalette  map[int]string    `json:"color_palette,omitempty"`
	DynamicColors map[int]string    `json:"dynamic_colors,omitempty"`
	SpecialColors map[int]string    `json:"special_colors,omitempty"`
	SelectionData map[string]string `json:"selection_data,omitempty"`

	WindowPosX        int  `json:"window_pos_x,omitempty"`
	WindowPosY        int  `json:"window_pos_y,omitempty"`
	WindowPixelWidth  int  `json:"window_pixel_width,omitempty"`
	WindowPixelHeight int  `json:"window_pixel_height,omitempty"`
	ScreenPixelWidth  int  `json:"screen_pixel_width,omitempty"`
	ScreenPixelHeight int  `json:"screen_pixel_height,omitempty"`
	CharPixelWidth    int  `json:"char_pixel_width,omitempty"`
	CharPixelHeight   int  `json:"char_pixel_height,omitempty"`
	WindowIconified   bool `json:"window_iconified,omitempty"`
}

// MarshalState exports the complete screen state to a serializable struct.
// WriteProcessInput (a function pointer) is not included and must be
// re-set by the caller after UnmarshalState.
func (s *Screen) MarshalState() *ScreenState {
	return &ScreenState{
		Columns: s.Columns,
		Lines:   s.Lines,
		Buffer:  trimBuffer(s.unpackRows(s.rows)),

		AltBuffer:      trimBuffer(s.unpackRows(s.altRows)),
		AltActive:      s.altActive,
		AltSavepoints:  copySavepoints(s.altSavepoints),
		AltWrapNext:    s.altWrapNext,
		AltSavedCursor: clonePtr(s.altSavedCursor),

		Cursor:     s.Cursor,
		Mode:       setToBoolMap(s.Mode),
		TabStops:   setToBoolMap(s.TabStops),
		SavedModes: maps.Clone(s.savedModes),
		Margins:    clonePtr(s.Margins),

		LeftMargin:          s.leftMargin,
		RightMargin:         s.rightMargin,
		CursorStyle:         s.cursorStyle,
		CharProtectionMode:  s.charProtectionMode,
		StatusDisplay:       s.statusDisplay,
		ConformanceLevel:    s.conformanceLevel,
		ConformanceExplicit: s.conformanceExplicit,
		IndexedColors:       s.indexedColors,

		Charset: s.Charset,
		G0:      slices.Clone(s.G0),
		G1:      slices.Clone(s.G1),

		Savepoints:   copySavepoints(s.Savepoints),
		SavedColumns: clonePtr(s.SavedColumns),

		WrapNext:  s.wrapNext,
		LastDrawn: s.lastDrawn,

		Title:          s.Title,
		IconName:       s.IconName,
		TitleStack:     slices.Clone(s.titleStack),
		IconStack:      slices.Clone(s.iconStack),
		TitleHexInput:  s.titleHexInput,
		TitleHexOutput: s.titleHexOutput,

		ColorPalette:  maps.Clone(s.colorPalette),
		DynamicColors: maps.Clone(s.dynamicColors),
		SpecialColors: maps.Clone(s.specialColors),
		SelectionData: maps.Clone(s.selectionData),

		WindowPosX:        s.windowPosX,
		WindowPosY:        s.windowPosY,
		WindowPixelWidth:  s.windowPixelWidth,
		WindowPixelHeight: s.windowPixelHeight,
		ScreenPixelWidth:  s.screenPixelWidth,
		ScreenPixelHeight: s.screenPixelHeight,
		CharPixelWidth:    s.charPixelWidth,
		CharPixelHeight:   s.charPixelHeight,
		WindowIconified:   s.windowIconified,
	}
}

// UnmarshalState restores screen state from a serializable struct.
// After calling this, the caller must re-set WriteProcessInput and
// create a new Stream if needed. All rows are marked dirty.
func (s *Screen) UnmarshalState(st *ScreenState) {
	s.Columns = st.Columns
	s.Lines = st.Lines
	rows := deepCopyBuffer(st.Buffer) // packed below, once the modes that fix the default cell are restored
	altRows := deepCopyBuffer(st.AltBuffer)
	s.altActive = st.AltActive
	s.altSavepoints = copySavepoints(st.AltSavepoints)
	s.altWrapNext = st.AltWrapNext
	s.altSavedCursor = clonePtr(st.AltSavedCursor)

	s.Cursor = st.Cursor
	s.Mode = boolToSetMap(st.Mode)
	s.modesChanged()
	s.TabStops = boolToSetMap(st.TabStops)
	s.savedModes = maps.Clone(st.SavedModes)
	s.Margins = clonePtr(st.Margins)

	s.leftMargin = st.LeftMargin
	s.rightMargin = st.RightMargin
	s.cursorStyle = st.CursorStyle
	s.charProtectionMode = st.CharProtectionMode
	s.statusDisplay = st.StatusDisplay
	s.conformanceLevel = st.ConformanceLevel
	s.conformanceExplicit = st.ConformanceExplicit
	s.indexedColors = st.IndexedColors

	s.Charset = st.Charset
	s.G0 = slices.Clone(st.G0)
	s.G1 = slices.Clone(st.G1)

	s.Savepoints = copySavepoints(st.Savepoints)
	s.SavedColumns = clonePtr(st.SavedColumns)

	s.wrapNext = st.WrapNext
	s.lastDrawn = st.LastDrawn

	s.Title = st.Title
	s.IconName = st.IconName
	s.titleStack = slices.Clone(st.TitleStack)
	s.iconStack = slices.Clone(st.IconStack)
	s.titleHexInput = st.TitleHexInput
	s.titleHexOutput = st.TitleHexOutput

	s.colorPalette = maps.Clone(st.ColorPalette)
	s.dynamicColors = maps.Clone(st.DynamicColors)
	s.specialColors = maps.Clone(st.SpecialColors)
	s.selectionData = maps.Clone(st.SelectionData)

	s.windowPosX = st.WindowPosX
	s.windowPosY = st.WindowPosY
	s.windowPixelWidth = st.WindowPixelWidth
	s.windowPixelHeight = st.WindowPixelHeight
	s.screenPixelWidth = st.ScreenPixelWidth
	s.screenPixelHeight = st.ScreenPixelHeight
	s.charPixelWidth = st.CharPixelWidth
	s.charPixelHeight = st.CharPixelHeight
	s.windowIconified = st.WindowIconified

	// Re-pad rows that trimBuffer shortened on the way out back to full width
	// with the default cell (which depends on DECSCNM, restored above).
	blank := s.defaultCell()
	padRowsToColumns(rows, s.Columns, blank)
	padRowsToColumns(altRows, s.Columns, blank)
	s.rows = s.packRows(rows)
	s.altRows = s.packRows(altRows)
}

// --- copy helpers ---

func deepCopyBuffer(buf [][]Cell) [][]Cell {
	if buf == nil {
		return nil
	}
	out := make([][]Cell, len(buf))
	for i, row := range buf {
		out[i] = append([]Cell(nil), row...)
	}
	return out
}

// trimBuffer returns a deep copy of buf with each row's trailing blank cells
// (Cell.Blank) removed: most of a screen is blank, and UnmarshalState
// re-pads with the default cell. A cell that keeps anything — a colored
// background, a protected cell, a wrapped row's end, Empty space — is kept,
// so a copy of the row is the same from a snapshot.
func trimBuffer(buf [][]Cell) [][]Cell {
	if buf == nil {
		return nil
	}
	out := make([][]Cell, len(buf))
	for i, row := range buf {
		last := len(row) - 1
		for last >= 0 && row[last].Blank() {
			last--
		}
		out[i] = append([]Cell(nil), row[:last+1]...)
	}
	return out
}

// padRowsToColumns re-pads rows that trimBuffer shortened back to full width
// with the default cell.
func padRowsToColumns(buf [][]Cell, cols int, blank Cell) {
	for i, row := range buf {
		for len(row) < cols {
			row = append(row, blank)
		}
		buf[i] = row
	}
}

func setToBoolMap(m map[int]struct{}) map[int]bool {
	if m == nil {
		return nil
	}
	out := make(map[int]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

func boolToSetMap(m map[int]bool) map[int]struct{} {
	if m == nil {
		return nil
	}
	out := make(map[int]struct{}, len(m))
	for k, v := range m {
		if v {
			out[k] = struct{}{}
		}
	}
	return out
}

// copySavepoints is a deep copy: each savepoint's character sets and saved
// column its own.
func copySavepoints(sp []Savepoint) []Savepoint {
	out := slices.Clone(sp)
	for i, s := range out {
		out[i].G0 = slices.Clone(s.G0)
		out[i].G1 = slices.Clone(s.G1)
		out[i].SavedCol = clonePtr(s.SavedCol)
	}
	return out
}

// clonePtr is a pointer to a copy of what p points at; nil for nil.
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
