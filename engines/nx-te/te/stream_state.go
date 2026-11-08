package te

import "strings"

// StreamState is a Stream's parser state: where it is in a sequence or a
// character not yet whole, so a stream restored from it reads the rest as
// the one it was taken from would (ScrollbackState.Stream).
type StreamState struct {
	State           int      `json:"state,omitempty"`
	Params          []int    `json:"params,omitempty"`
	Current         string   `json:"current,omitempty"`
	Private         bool     `json:"private,omitempty"`
	CSIPrefix       rune     `json:"csi_prefix,omitempty"`
	CSIIntermediate rune     `json:"csi_intermediate,omitempty"`
	PendingQuote    bool     `json:"pending_quote,omitempty"`
	TakingText      bool     `json:"taking_text,omitempty"`
	OSCEsc          bool     `json:"osc_esc,omitempty"`
	Str             string   `json:"str,omitempty"`
	ParamStrings    []string `json:"param_strings,omitempty"`
	PendingBytes    []byte   `json:"pending_bytes,omitempty"`
	Use8BitControls bool     `json:"use_8bit_controls,omitempty"`
	NotUTF8         bool     `json:"not_utf8,omitempty"`
}

// MarshalState exports where the parser is: nil when it is at rest, between
// sequences and characters, with nothing to carry.
func (st *Stream) MarshalState() *StreamState {
	s := &StreamState{
		State:           int(st.state),
		Params:          append([]int(nil), st.params...),
		Current:         string(st.current),
		Private:         st.private,
		CSIPrefix:       st.csiPrefix,
		CSIIntermediate: st.csiIntermediate,
		PendingQuote:    st.pendingQuote,
		TakingText:      st.takingText,
		OSCEsc:          st.oscEsc,
		Str:             st.strBuf.String(),
		ParamStrings:    append([]string(nil), st.paramStrings...),
		PendingBytes:    append([]byte(nil), st.pendingBytes...),
		Use8BitControls: st.use8BitControls,
		NotUTF8:         !st.useUTF8,
	}
	if s.State == int(stateGround) && len(s.Params) == 0 && s.Current == "" && !s.Private &&
		s.CSIPrefix == 0 && s.CSIIntermediate == 0 && !s.PendingQuote && !s.TakingText && !s.OSCEsc &&
		s.Str == "" && len(s.ParamStrings) == 0 && len(s.PendingBytes) == 0 && !s.Use8BitControls && !s.NotUTF8 {
		return nil
	}
	return s
}

// UnmarshalState puts the parser where s says; nil is at rest.
func (st *Stream) UnmarshalState(s *StreamState) {
	if s == nil {
		return
	}
	st.state = parserState(s.State)
	st.params = append([]int(nil), s.Params...)
	st.current = append(st.current[:0], s.Current...)
	st.private = s.Private
	st.csiPrefix = s.CSIPrefix
	st.csiIntermediate = s.CSIIntermediate
	st.pendingQuote = s.PendingQuote
	st.takingText = s.TakingText
	st.oscEsc = s.OSCEsc
	st.strBuf.Reset()
	st.strBuf.WriteString(s.Str)
	st.paramStrings = append([]string(nil), s.ParamStrings...)
	st.hasColon = strings.ContainsRune(s.Current, ':')
	for _, p := range s.ParamStrings {
		st.hasColon = st.hasColon || strings.ContainsRune(p, ':')
	}
	st.pendingBytes = append([]byte(nil), s.PendingBytes...)
	st.use8BitControls = s.Use8BitControls
	st.useUTF8 = !s.NotUTF8
}
