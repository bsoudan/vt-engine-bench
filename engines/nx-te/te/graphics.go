package te

const (
	// SgrFg256 selects 256-color foreground mode in SGR.
	SgrFg256 = 38
	// SgrBg256 selects 256-color background mode in SGR.
	SgrBg256 = 48
	// SgrUnderlineColor selects the underline color in SGR (58;2;r;g;b /
	// 58;5;n), SgrDefaultUnderlineColor (59) resets it.
	SgrUnderlineColor        = 58
	SgrDefaultUnderlineColor = 59
	// sgrUnderlineStyleBase encodes a colon-form underline style (SGR 4:n) as
	// an internal SGR code base+n that SelectGraphicRendition interprets. It is
	// above the 9999 cap the parser clamps params to, so it can never collide
	// with a value from the wire.
	sgrUnderlineStyleBase = 10000
)
