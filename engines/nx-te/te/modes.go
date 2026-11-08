package te

// Terminal modes are stored in Screen.Mode keyed by an int. ANSI modes (set
// via CSI Pn h) use their raw number. DEC private modes (set via CSI ? Pn h)
// share the same map, so to avoid colliding with ANSI numbers they are encoded
// as (number << 5), as the constants below are.

const (
	// ModeLNM is a terminal mode constant.
	ModeLNM = 20
	// ModeIRM is a terminal mode constant.
	ModeIRM = 4

	// ModeDECTCEM is a terminal mode constant.
	ModeDECTCEM = 25 << 5
	// ModeDECSCNM is a terminal mode constant.
	ModeDECSCNM = 5 << 5
	// ModeDECOM is a terminal mode constant.
	ModeDECOM = 6 << 5
	// ModeDECAWM is a terminal mode constant.
	ModeDECAWM = 7 << 5
	// ModeDECCOLM is a terminal mode constant.
	ModeDECCOLM = 3 << 5
	// ModeDECLRMM is a terminal mode constant.
	ModeDECLRMM = 69 << 5
	// ModeDECCKM is a terminal mode constant.
	ModeDECCKM = 1 << 5
	// ModeDECNCSM is a terminal mode constant.
	ModeDECNCSM = 95 << 5
	// ModeDECHCCM is a terminal mode constant.
	ModeDECHCCM = 60 << 5
	// ModeReverseWrapInline is a terminal mode constant.
	ModeReverseWrapInline = 45 << 5
	// ModeReverseWrapExtend is a terminal mode constant.
	ModeReverseWrapExtend = 1045 << 5
	// ModeDECSaveCursor is a terminal mode constant.
	ModeDECSaveCursor = 1048 << 5
	// ModeAltBuf is a terminal mode constant.
	ModeAltBuf = 47 << 5
	// ModeAltBufOpt is a terminal mode constant.
	ModeAltBufOpt = 1047 << 5
	// ModeAltBufCursor is a terminal mode constant.
	ModeAltBufCursor = 1049 << 5
	// ModeAllow80To132 is a terminal mode constant.
	ModeAllow80To132 = 40 << 5
	// ModeMoreFix is a terminal mode constant.
	ModeMoreFix = 41 << 5

	// Mouse tracking and bracketed-paste private modes, for whoever decides
	// how to forward mouse and paste input (the view).
	ModeMouseX10         = 9 << 5    // X10 compatibility mouse reporting
	ModeMouseNormal      = 1000 << 5 // normal mouse tracking (press/release)
	ModeMouseButtonEvent = 1002 << 5 // button-event tracking (drag)
	ModeMouseAnyEvent    = 1003 << 5 // any-event tracking (all motion)
	ModeMouseSGR         = 1006 << 5 // SGR extended mouse coordinate encoding
	ModeFocusEvent       = 1004 << 5 // focus in/out reporting
	ModeBracketedPaste   = 2004 << 5 // bracketed paste

	// ModeColorSchemeReport asks to be told when the terminal's colors change
	// (CSI ? 997 ; 1 n dark, 2 light), as the terminal it shadows tells the
	// console (Screen.SetColors).
	ModeColorSchemeReport = 2031 << 5

	// ModeSyncOutput is synchronized output: while it is set the program is
	// mid-frame, and whoever shows the screen holds it until the frame is
	// done (the view does, cores/vtappview). te only keeps it.
	ModeSyncOutput = 2026 << 5
)

// extensionModes are the private modes numbered 1000 and up (xterm's, and
// those terminals have added since) that te acts on or keeps for whoever
// does. DECRQM reports any other as unrecognized, so a program asking after
// grapheme clustering (2027) doesn't take te for a terminal that has it.
var extensionModes = map[int]bool{
	ModeMouseNormal:       true,
	ModeMouseButtonEvent:  true,
	ModeMouseAnyEvent:     true,
	ModeFocusEvent:        true,
	ModeMouseSGR:          true,
	ModeReverseWrapExtend: true,
	ModeAltBufOpt:         true,
	ModeDECSaveCursor:     true,
	ModeAltBufCursor:      true,
	ModeBracketedPaste:    true,
	ModeColorSchemeReport: true,
	ModeSyncOutput:        true,
}
