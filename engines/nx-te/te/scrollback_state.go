package te

import "nx/cell"

// ScrollbackState is a fully serializable snapshot of a ScrollbackScreen's state,
// including the embedded Screen and the scrollback.
type ScrollbackState struct {
	Screen     ScreenState `json:"screen"`
	TopItems   cell.Grid   `json:"top_items,omitempty"`
	TotalAdded uint64      `json:"total_added,omitempty"`
	// FirstSeq is the seq of TopItems[0] (or equal to TotalAdded when
	// TopItems is empty), said, so a round trip does not derive a shifted
	// seq space from the buffer's length.
	FirstSeq uint64 `json:"first_seq,omitempty"`
	// Stream is where the parser feeding the screen was, when the snapshot
	// is taken between two reads: a read ends where it likes, in the middle
	// of a sequence or a character, and the rest comes after the snapshot.
	// The owner of the stream fills it (Stream.MarshalState); a screen
	// restored from it is fed through a stream restored from it. Nil, the
	// parser was at rest.
	Stream *StreamState `json:"stream,omitempty"`
}

// MarshalState exports the complete history screen state.
func (h *ScrollbackScreen) MarshalState() *ScrollbackState {
	return &ScrollbackState{
		Screen:     *h.Screen.MarshalState(),
		TopItems:   trimBuffer(h.unpackRows(h.scrollback.Top.items)),
		TotalAdded: h.scrollback.TotalAdded,
		FirstSeq:   h.scrollback.FirstSeq,
	}
}

// UnmarshalState restores history screen state from a serializable struct
// into a screen made with NewScrollbackScreen, which keeps the scrollback it was
// made with: of more rows than that, the newest are kept. After calling
// this, the caller must re-set WriteProcessInput and create a new Stream.
func (h *ScrollbackScreen) UnmarshalState(st *ScrollbackState) {
	h.Screen.scrolledOff = h.keepScrolledOff
	h.Screen.moreRows = h.historyRows
	h.Screen.UnmarshalState(&st.Screen)

	rows, first := st.TopItems, st.FirstSeq
	if over := len(rows) - h.scrollback.Top.max; over > 0 {
		rows, first = rows[over:], first+uint64(over)
	}
	items := deepCopyBuffer(rows)
	// Re-pad history rows that trimBuffer shortened on the way out (the Screen
	// is restored above, so Columns and the default cell are available).
	padRowsToColumns(items, h.Screen.Columns, h.Screen.defaultCell())
	h.scrollback.Top.items = h.packRows(items)
	h.scrollback.TotalAdded = st.TotalAdded
	h.scrollback.FirstSeq = first
	h.unseen = nil // a snapshot's history is whole
}
