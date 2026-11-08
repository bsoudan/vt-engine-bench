package te

import (
	"strings"
	"testing"
)

type counter struct {
	count int
}

func (c *counter) Call(_ ...interface{}) {
	c.count++
}

type argCheck struct {
	count int
	args  []interface{}
}

func (a *argCheck) Call(args ...interface{}) {
	a.count++
	a.args = args
}

func toInterfaces(params ...int) []interface{} {
	out := make([]interface{}, len(params))
	for i, param := range params {
		out[i] = param
	}
	return out
}

type mockScreen struct {
	bell                   func()
	backspace              func()
	tab                    func()
	linefeed               func()
	nextLine               func()
	carriageReturn         func()
	shiftOut               func()
	shiftIn                func()
	reset                  func()
	index                  func()
	reverseIndex           func()
	setTabStop             func()
	startProtectedArea     func()
	endProtectedArea       func()
	setCharacterProtection func(int)
	clearTabStop           func(...int)
	saveCursor             func()
	restoreCursor          func()
	alignmentDisplay       func()
	insertCharacters       func(...int)
	cursorUp               func(...int)
	cursorDown             func(...int)
	cursorForward          func(...int)
	cursorBack             func(...int)
	cursorDown1            func(...int)
	cursorUp1              func(...int)
	cursorToColumn         func(...int)
	cursorToColumnAbs      func(...int)
	cursorPosition         func(...int)
	cursorBackTab          func(...int)
	cursorForwardTab       func(...int)
	scrollUp               func(...int)
	scrollDown             func(...int)
	repeatLast             func(...int)
	eraseInDisplay         func(int, bool, ...int)
	eraseInLine            func(int, bool, ...int)
	insertLines            func(...int)
	deleteLines            func(...int)
	deleteCharacters       func(...int)
	eraseCharacters        func(...int)
	selectiveEraseRect     func(int, int, int, int)
	reportDeviceAttrs      func(int, bool, rune, ...int)
	cursorToLine           func(...int)
	reportDeviceStatus     func(int, bool, rune, ...int)
	reportTermVersion      func()
	reportMode             func(int, bool)
	requestStatusString    func(string)
	softReset              func()
	saveModes              func([]int)
	restoreModes           func([]int)
	forwardIndex           func()
	backIndex              func()
	insertColumns          func(int)
	deleteColumns          func(int)
	eraseRectangle         func(int, int, int, int)
	fillRectangle          func(rune, int, int, int, int)
	copyRectangle          func(int, int, int, int, int, int)
	setSelectionData       func(string, string)
	querySelectionData     func(string)
	notify                 func(Notification)
	setColor               func(int, string)
	queryColor             func(int)
	resetColor             func(int, bool)
	setDynamicColor        func(int, string)
	queryDynamicColor      func(int)
	setSpecialColor        func(int, string)
	querySpecialColor      func(int)
	resetSpecialColor      func(int, bool)
	resetDynamicColor      func(int, bool)
	setTitleMode           func([]int, bool)
	setConformance         func(int, int)
	windowOp               func([]int)
	setMargins             func(...int)
	setLeftRightMargins    func(int, int)
	selectGraphic          func([]int, bool)
	draw                   func(string)
	debug                  func(...interface{})
	setMode                func([]int, bool)
	resetMode              func([]int, bool)
	defineCharset          func(string, string)
	setTitle               func(string)
	setIconName            func(string)
}

func (m *mockScreen) Bell() {
	if m.bell != nil {
		m.bell()
	}
}
func (m *mockScreen) Backspace() {
	if m.backspace != nil {
		m.backspace()
	}
}
func (m *mockScreen) Tab() {
	if m.tab != nil {
		m.tab()
	}
}
func (m *mockScreen) LineFeed() {
	if m.linefeed != nil {
		m.linefeed()
	}
}
func (m *mockScreen) CarriageReturn() {
	if m.carriageReturn != nil {
		m.carriageReturn()
	}
}
func (m *mockScreen) NextLine() {
	if m.nextLine != nil {
		m.nextLine()
	}
}
func (m *mockScreen) ShiftOut() {
	if m.shiftOut != nil {
		m.shiftOut()
	}
}
func (m *mockScreen) ShiftIn() {
	if m.shiftIn != nil {
		m.shiftIn()
	}
}
func (m *mockScreen) Reset() {
	if m.reset != nil {
		m.reset()
	}
}
func (m *mockScreen) Index() {
	if m.index != nil {
		m.index()
	}
}
func (m *mockScreen) ReverseIndex() {
	if m.reverseIndex != nil {
		m.reverseIndex()
	}
}
func (m *mockScreen) SetTabStop() {
	if m.setTabStop != nil {
		m.setTabStop()
	}
}
func (m *mockScreen) StartProtectedArea() {
	if m.startProtectedArea != nil {
		m.startProtectedArea()
	}
}
func (m *mockScreen) EndProtectedArea() {
	if m.endProtectedArea != nil {
		m.endProtectedArea()
	}
}
func (m *mockScreen) SetCharacterProtection(mode int) {
	if m.setCharacterProtection != nil {
		m.setCharacterProtection(mode)
	}
}
func (m *mockScreen) ClearTabStop(how ...int) {
	if m.clearTabStop != nil {
		m.clearTabStop(how...)
	}
}
func (m *mockScreen) SaveCursor() {
	if m.saveCursor != nil {
		m.saveCursor()
	}
}
func (m *mockScreen) RestoreCursor() {
	if m.restoreCursor != nil {
		m.restoreCursor()
	}
}

func (m *mockScreen) SaveCursorDEC() {
	if m.saveCursor != nil {
		m.saveCursor()
	}
}

func (m *mockScreen) RestoreCursorDEC() {
	if m.restoreCursor != nil {
		m.restoreCursor()
	}
}
func (m *mockScreen) AlignmentDisplay() {
	if m.alignmentDisplay != nil {
		m.alignmentDisplay()
	}
}
func (m *mockScreen) InsertCharacters(count ...int) {
	if m.insertCharacters != nil {
		m.insertCharacters(count...)
	}
}
func (m *mockScreen) CursorUp(count ...int) {
	if m.cursorUp != nil {
		m.cursorUp(count...)
	}
}
func (m *mockScreen) CursorDown(count ...int) {
	if m.cursorDown != nil {
		m.cursorDown(count...)
	}
}
func (m *mockScreen) CursorForward(count ...int) {
	if m.cursorForward != nil {
		m.cursorForward(count...)
	}
}
func (m *mockScreen) CursorBack(count ...int) {
	if m.cursorBack != nil {
		m.cursorBack(count...)
	}
}
func (m *mockScreen) CursorDown1(count ...int) {
	if m.cursorDown1 != nil {
		m.cursorDown1(count...)
	}
}
func (m *mockScreen) CursorUp1(count ...int) {
	if m.cursorUp1 != nil {
		m.cursorUp1(count...)
	}
}
func (m *mockScreen) CursorToColumn(column ...int) {
	if m.cursorToColumn != nil {
		m.cursorToColumn(column...)
	}
}
func (m *mockScreen) CursorToColumnAbsolute(column ...int) {
	if m.cursorToColumnAbs != nil {
		m.cursorToColumnAbs(column...)
	}
}
func (m *mockScreen) CursorPosition(params ...int) {
	if m.cursorPosition != nil {
		m.cursorPosition(params...)
	}
}
func (m *mockScreen) CursorBackTab(count ...int) {
	if m.cursorBackTab != nil {
		m.cursorBackTab(count...)
	}
}
func (m *mockScreen) CursorForwardTab(count ...int) {
	if m.cursorForwardTab != nil {
		m.cursorForwardTab(count...)
	}
}
func (m *mockScreen) ScrollUp(count ...int) {
	if m.scrollUp != nil {
		m.scrollUp(count...)
	}
}
func (m *mockScreen) ScrollDown(count ...int) {
	if m.scrollDown != nil {
		m.scrollDown(count...)
	}
}
func (m *mockScreen) RepeatLast(count ...int) {
	if m.repeatLast != nil {
		m.repeatLast(count...)
	}
}
func (m *mockScreen) EraseInDisplay(how int, private bool, rest ...int) {
	if m.eraseInDisplay != nil {
		m.eraseInDisplay(how, private, rest...)
	}
}
func (m *mockScreen) EraseInLine(how int, private bool, rest ...int) {
	if m.eraseInLine != nil {
		m.eraseInLine(how, private, rest...)
	}
}
func (m *mockScreen) InsertLines(count ...int) {
	if m.insertLines != nil {
		m.insertLines(count...)
	}
}
func (m *mockScreen) DeleteLines(count ...int) {
	if m.deleteLines != nil {
		m.deleteLines(count...)
	}
}
func (m *mockScreen) DeleteCharacters(count ...int) {
	if m.deleteCharacters != nil {
		m.deleteCharacters(count...)
	}
}
func (m *mockScreen) EraseCharacters(count ...int) {
	if m.eraseCharacters != nil {
		m.eraseCharacters(count...)
	}
}
func (m *mockScreen) ReportDeviceAttributes(mode int, private bool, prefix rune, rest ...int) {
	if m.reportDeviceAttrs != nil {
		m.reportDeviceAttrs(mode, private, prefix, rest...)
	}
}
func (m *mockScreen) CursorToLine(line ...int) {
	if m.cursorToLine != nil {
		m.cursorToLine(line...)
	}
}
func (m *mockScreen) ReportDeviceStatus(mode int, private bool, prefix rune, rest ...int) {
	if m.reportDeviceStatus != nil {
		m.reportDeviceStatus(mode, private, prefix, rest...)
	}
}

func (m *mockScreen) ReportTerminalVersion() {
	if m.reportTermVersion != nil {
		m.reportTermVersion()
	}
}

func (m *mockScreen) ReportMode(mode int, private bool) {
	if m.reportMode != nil {
		m.reportMode(mode, private)
	}
}

func (m *mockScreen) RequestStatusString(query string) {
	if m.requestStatusString != nil {
		m.requestStatusString(query)
	}
}

func (m *mockScreen) SoftReset() {
	if m.softReset != nil {
		m.softReset()
	}
}

func (m *mockScreen) SaveModes(modes []int) {
	if m.saveModes != nil {
		m.saveModes(modes)
	}
}

func (m *mockScreen) RestoreModes(modes []int) {
	if m.restoreModes != nil {
		m.restoreModes(modes)
	}
}

func (m *mockScreen) ForwardIndex() {
	if m.forwardIndex != nil {
		m.forwardIndex()
	}
}

func (m *mockScreen) BackIndex() {
	if m.backIndex != nil {
		m.backIndex()
	}
}

func (m *mockScreen) InsertColumns(count int) {
	if m.insertColumns != nil {
		m.insertColumns(count)
	}
}

func (m *mockScreen) DeleteColumns(count int) {
	if m.deleteColumns != nil {
		m.deleteColumns(count)
	}
}

func (m *mockScreen) EraseRectangle(top, left, bottom, right int) {
	if m.eraseRectangle != nil {
		m.eraseRectangle(top, left, bottom, right)
	}
}

func (m *mockScreen) SelectiveEraseRectangle(top, left, bottom, right int) {
	if m.selectiveEraseRect != nil {
		m.selectiveEraseRect(top, left, bottom, right)
	}
}

func (m *mockScreen) FillRectangle(ch rune, top, left, bottom, right int) {
	if m.fillRectangle != nil {
		m.fillRectangle(ch, top, left, bottom, right)
	}
}

func (m *mockScreen) CopyRectangle(srcTop, srcLeft, srcBottom, srcRight, dstTop, dstLeft int) {
	if m.copyRectangle != nil {
		m.copyRectangle(srcTop, srcLeft, srcBottom, srcRight, dstTop, dstLeft)
	}
}

func (m *mockScreen) SetSelectionData(selection, data string) {
	if m.setSelectionData != nil {
		m.setSelectionData(selection, data)
	}
}

func (m *mockScreen) QuerySelectionData(selection string) {
	if m.querySelectionData != nil {
		m.querySelectionData(selection)
	}
}

func (m *mockScreen) Notify(n Notification) {
	if m.notify != nil {
		m.notify(n)
	}
}

func (m *mockScreen) SetColor(index int, value string) {
	if m.setColor != nil {
		m.setColor(index, value)
	}
}

func (m *mockScreen) QueryColor(index int) {
	if m.queryColor != nil {
		m.queryColor(index)
	}
}

func (m *mockScreen) ResetColor(index int, all bool) {
	if m.resetColor != nil {
		m.resetColor(index, all)
	}
}

func (m *mockScreen) SetDynamicColor(index int, value string) {
	if m.setDynamicColor != nil {
		m.setDynamicColor(index, value)
	}
}

func (m *mockScreen) QueryDynamicColor(index int) {
	if m.queryDynamicColor != nil {
		m.queryDynamicColor(index)
	}
}

func (m *mockScreen) SetSpecialColor(index int, value string) {
	if m.setSpecialColor != nil {
		m.setSpecialColor(index, value)
	}
}

func (m *mockScreen) QuerySpecialColor(index int) {
	if m.querySpecialColor != nil {
		m.querySpecialColor(index)
	}
}

func (m *mockScreen) ResetSpecialColor(index int, all bool) {
	if m.resetSpecialColor != nil {
		m.resetSpecialColor(index, all)
	}
}

func (m *mockScreen) ResetDynamicColor(index int, all bool) {
	if m.resetDynamicColor != nil {
		m.resetDynamicColor(index, all)
	}
}

func (m *mockScreen) SetTitleMode(params []int, reset bool) {
	if m.setTitleMode != nil {
		m.setTitleMode(params, reset)
	}
}

func (m *mockScreen) SetConformance(level int, sevenBit int) {
	if m.setConformance != nil {
		m.setConformance(level, sevenBit)
	}
}

func (m *mockScreen) WindowOp(params []int) {
	if m.windowOp != nil {
		m.windowOp(params)
	}
}
func (m *mockScreen) SetMargins(params ...int) {
	if m.setMargins != nil {
		m.setMargins(params...)
	}
}
func (m *mockScreen) SetLeftRightMargins(left, right int) {
	if m.setLeftRightMargins != nil {
		m.setLeftRightMargins(left, right)
	}
}
func (m *mockScreen) SelectGraphicRendition(attrs []int, private bool) {
	if m.selectGraphic != nil {
		m.selectGraphic(attrs, private)
	}
}
func (m *mockScreen) Draw(data string) {
	if m.draw != nil {
		m.draw(data)
	}
}
func (m *mockScreen) Debug(params ...interface{}) {
	if m.debug != nil {
		m.debug(params...)
	}
}
func (m *mockScreen) SetMode(modes []int, private bool) {
	if m.setMode != nil {
		m.setMode(modes, private)
	}
}
func (m *mockScreen) ResetMode(modes []int, private bool) {
	if m.resetMode != nil {
		m.resetMode(modes, private)
	}
}
func (m *mockScreen) DefineCharset(code, mode string) {
	if m.defineCharset != nil {
		m.defineCharset(code, mode)
	}
}
func (m *mockScreen) SetTitle(param string) {
	if m.setTitle != nil {
		m.setTitle(param)
	}
}
func (m *mockScreen) SetIconName(param string) {
	if m.setIconName != nil {
		m.setIconName(param)
	}
}

func TestLinefeedControls(t *testing.T) {
	t.Parallel()
	count := 0
	stream := NewStream(&mockScreen{linefeed: func() { count++ }})
	if err := stream.Feed("\n" + "\x0b" + "\x0c"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 linefeeds, got %d", count)
	}
}

func TestUnknownCSISequences(t *testing.T) {
	t.Parallel()
	args := argCheck{}
	screen := &mockScreen{debug: args.Call}
	stream := NewStream(screen)
	if err := stream.Feed(ControlCSI + "6;z"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if args.count != 1 {
		t.Fatalf("expected 1 debug call, got %d", args.count)
	}
	if len(args.args) != 2 || args.args[0].(int) != 6 || args.args[1].(int) != 0 {
		t.Fatalf("unexpected args: %#v", args.args)
	}
}

func TestCSIParamParsing(t *testing.T) {
	t.Parallel()
	args := argCheck{}
	screen := &mockScreen{cursorPosition: func(params ...int) { args.Call(toInterfaces(params...)...) }}
	stream := NewStream(screen)
	if err := stream.Feed(ControlCSI + ";" + "f"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if args.count != 1 || args.args[0].(int) != 0 || args.args[1].(int) != 0 {
		t.Fatalf("unexpected args: %#v", args.args)
	}

	args.count = 0
	if err := stream.Feed(ControlCSI + "999999999999999;99999999999999" + "f"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if args.args[0].(int) != 9999 || args.args[1].(int) != 9999 {
		t.Fatalf("expected overflow capped")
	}
}

func TestInterruptCSI(t *testing.T) {
	t.Parallel()
	seen := []string{}
	screen := &mockScreen{draw: func(data string) { seen = append(seen, data) }}
	stream := NewStream(screen)
	if err := stream.Feed(ControlCSI + "10;" + "\x1a" + "10" + "f"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(seen) != 2 || seen[0] != "\x1a" || seen[1] != "10"+"f" {
		t.Fatalf("unexpected seen: %#v", seen)
	}
}

func TestControlCharactersInCSI(t *testing.T) {
	t.Parallel()
	args := argCheck{}
	screen := &mockScreen{cursorPosition: func(params ...int) { args.Call(toInterfaces(params...)...) }}
	stream := NewStream(screen)
	if err := stream.Feed(ControlCSI + "10;\t\t\n\r\n10" + "f"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if args.args[0].(int) != 10 || args.args[1].(int) != 10 {
		t.Fatalf("unexpected args: %#v", args.args)
	}
}

func TestOSCSequences(t *testing.T) {
	t.Parallel()
	screen := NewScreen(10, 1)
	stream := NewStream(screen)
	if err := stream.Feed("\x1b]" + "1;foo" + "\x1b\\"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if screen.IconName != "foo" {
		t.Fatalf("expected icon name")
	}
	if err := stream.Feed("\x1b]" + "2;bar" + "\x1b\\"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if screen.Title != "bar" {
		t.Fatalf("expected title")
	}
	if err := stream.Feed("\x1b]" + "0;baz" + "\x1b\\"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if screen.IconName != "baz" || screen.Title != "baz" {
		t.Fatalf("expected title and icon name")
	}
}

func TestCompatibilityAPI(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(nil)
	stream.Attach(screen)
	stream.Attach(NewScreen(80, 24))
	if err := stream.Feed("привет"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	stream.Detach(screen)
}

func TestDefineCharsetNoop(t *testing.T) {
	t.Parallel()
	screen := NewScreen(3, 3)
	stream := NewStream(screen)
	if err := stream.Feed(ControlESC + "(B"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if screen.Display()[0] != "   " {
		t.Fatalf("unexpected display")
	}
}

func TestNonUTF8Shifts(t *testing.T) {
	t.Parallel()
	calls := 0
	screen := &mockScreen{shiftIn: func() { calls++ }, shiftOut: func() { calls++ }}
	stream := NewStream(screen)
	stream.useUTF8 = false
	if err := stream.Feed("\x0f" + "\x0e"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestSetResetModePrivate(t *testing.T) {
	t.Parallel()
	args := []interface{}{}
	screen := &mockScreen{setMode: func(modes []int, private bool) { args = append(args, modes, private) }}
	stream := NewStream(screen)
	if err := stream.Feed(ControlCSI + "?9;2h"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(args) == 0 {
		t.Fatalf("expected set mode")
	}

	resetArgs := []interface{}{}
	screen = &mockScreen{resetMode: func(modes []int, private bool) { resetArgs = append(resetArgs, modes, private) }}
	stream = NewStream(screen)
	if err := stream.Feed(ControlCSI + "?9;2l"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(resetArgs) == 0 {
		t.Fatalf("expected reset mode")
	}
}

func TestNonCSISequences(t *testing.T) {
	t.Parallel()
	args := argCheck{}
	screen := &mockScreen{cursorUp: func(count ...int) { args.Call(toInterfaces(count...)...) }}
	stream := NewStream(screen)
	if err := stream.Feed(ControlESC + "[5" + "A"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if args.count != 1 || args.args[0].(int) != 5 {
		t.Fatalf("unexpected args: %#v", args.args)
	}
}

func TestSetTitleIconFromOSC(t *testing.T) {
	t.Parallel()
	screen := NewScreen(10, 1)
	stream := NewStream(screen)
	if err := stream.Feed(ControlOSC + "1;foo" + ControlST); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if screen.IconName != "foo" {
		t.Fatalf("expected icon name")
	}
	if err := stream.Feed(ControlOSC + "2;bar" + ControlST); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if screen.Title != "bar" {
		t.Fatalf("expected title")
	}
}

func TestDrawAfterOSC(t *testing.T) {
	t.Parallel()
	screen := NewScreen(5, 1)
	stream := NewStream(screen)
	if err := stream.Feed(ControlOSC + "0;bar" + ControlST); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if err := stream.Feed("➜"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if screen.Buffer[0][0].Data != "➜" {
		t.Fatalf("expected symbol drawn")
	}
}

func TestStreamDetachNoListener(t *testing.T) {
	t.Parallel()
	stream := NewStream(nil)
	if err := stream.Feed("foo"); err == nil {
		t.Fatalf("expected error")
	}
	stream.Detach(nil)
}

func TestXTVERSIONDispatch(t *testing.T) {
	t.Parallel()
	called := 0
	screen := &mockScreen{reportTermVersion: func() { called++ }}
	stream := NewStream(screen)
	if err := stream.Feed(ControlCSI + ">q"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if err := stream.Feed(ControlCSI + ">0q"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if called != 2 {
		t.Fatalf("ReportTerminalVersion called %d times, want 2", called)
	}
}

func TestXTVERSIONReply(t *testing.T) {
	t.Parallel()
	var reply string
	screen := NewScreen(10, 3)
	screen.WriteProcessInput = func(s string) { reply = s }
	screen.TerminalName = "nxterm(v0.1)"
	screen.ReportTerminalVersion()
	want := ControlDCS + ">|nxterm(v0.1)" + ControlST
	if reply != want {
		t.Fatalf("reply=%q want %q", reply, want)
	}
}

func TestXTVERSIONNoReplyWhenUnset(t *testing.T) {
	t.Parallel()
	called := false
	screen := NewScreen(10, 3)
	screen.WriteProcessInput = func(string) { called = true }
	screen.ReportTerminalVersion()
	if called {
		t.Fatalf("reply emitted with empty TerminalName")
	}
}

// XTGETTCAP: each capability the screen has is answered, name and value in
// hex, and the first it has not ends the reply.
func TestXTGETTCAPReply(t *testing.T) {
	t.Parallel()
	var reply strings.Builder
	screen := NewScreen(10, 3)
	screen.WriteProcessInput = func(s string) { reply.WriteString(s) }
	screen.Capabilities = map[string]string{"Ms": "\x1b]52", "Tc": ""}
	stream := NewStream(screen)
	// Ms, Tc, then a name it has not (colors), then one it would have had.
	if err := stream.Feed(ControlDCS + "+q4d73;5463;636f6c6f7273;4d73" + ControlST); err != nil {
		t.Fatalf("feed: %v", err)
	}
	want := ControlDCS + "1+r4d73=1b5d3532" + ControlST +
		ControlDCS + "1+r5463=" + ControlST +
		ControlDCS + "0+r636f6c6f7273" + ControlST
	if reply.String() != want {
		t.Fatalf("reply=%q want %q", reply.String(), want)
	}
}

func TestXTGETTCAPNoReplyWhenUnset(t *testing.T) {
	t.Parallel()
	called := false
	screen := NewScreen(10, 3)
	screen.WriteProcessInput = func(string) { called = true }
	if err := NewStream(screen).Feed(ControlDCS + "+q4d73" + ControlST); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if called {
		t.Fatalf("reply emitted with no Capabilities")
	}
}

func (m *mockScreen) RequestTermcap(string) {}

func (m *mockScreen) EraseInDisplaySelective(how int) { m.EraseInDisplay(how, true) }

func (m *mockScreen) SetCursorStyle(int) {}

func (m *mockScreen) SetActiveStatusDisplay(int) {}

func (m *mockScreen) isModeSet(int) bool { return false }
