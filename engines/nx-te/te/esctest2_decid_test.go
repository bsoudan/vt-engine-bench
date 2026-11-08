package te

import "testing"

// From esctest2/esctest/tests/decid.py::test_DECID_Basic
func TestEsctestDecidTestDECIDBasic(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	response := esctestCaptureResponse(screen, func() {
		esctestWrite(t, stream, ControlESC+"Z")
	})
	params := esctestParseCSI(t, response, '?')
	if len(params) == 0 {
		t.Fatalf("expected response params")
	}
}
