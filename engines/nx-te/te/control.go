package te

const (
	// ControlESC is escape (ESC).
	ControlESC = "\x1b"
	// ControlCSI is the CSI introducer, in its 7-bit form (ESC [).
	ControlCSI = "\x1b["
	// ControlST is the string terminator, in its 7-bit form (ESC \).
	ControlST = "\x1b\\"
	// ControlOSC is the OSC introducer, in its 7-bit form (ESC ]).
	ControlOSC = "\x1b]"
	// ControlDCS is the device control string introducer (ESC P).
	ControlDCS = ControlESC + "P"
)
