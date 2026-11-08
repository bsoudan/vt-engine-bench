package te

import (
	"reflect"
	"strings"
	"testing"
)

// notified feeds each of seqs to a stream, in its own Feed, and is the
// notifications its listener was told, in order.
func notified(t *testing.T, seqs ...string) []Notification {
	t.Helper()
	var got []Notification
	stream := NewStream(&mockScreen{notify: func(n Notification) { got = append(got, n) }})
	for _, s := range seqs {
		if err := stream.Feed(s); err != nil {
			t.Fatalf("feed %q: %v", s, err)
		}
	}
	return got
}

// Each sequence a terminal takes for a desktop notification is one
// (doc/done/ATTACH_PLAN.md, stage 4), ended by BEL or ST; the sequences
// that share their numbers and are not, are not.
func TestNotificationSequences(t *testing.T) {
	t.Parallel()
	const bel = "\x07"
	for _, tc := range []struct {
		name string
		seqs []string
		want []Notification
	}{
		{"OSC 9", []string{ControlOSC + "9;box: password asked for" + bel},
			[]Notification{{Body: "box: password asked for"}}},
		{"OSC 9 with a ';' in its text, ended by ST", []string{ControlOSC + "9;done; 3 failed" + ControlST},
			[]Notification{{Body: "done; 3 failed"}}},
		{"OSC 9;4, ConEmu's progress", []string{ControlOSC + "9;4;1;50" + ControlST}, nil},
		{"OSC 9;9, ConEmu's working directory", []string{ControlOSC + "9;9;C:\\Users" + ControlST}, nil},
		{"OSC 777 notify", []string{ControlOSC + "777;notify;make;built; 0 warnings" + bel},
			[]Notification{{Title: "make", Body: "built; 0 warnings"}}},
		{"OSC 777 preexec, a shell's", []string{ControlOSC + "777;preexec" + bel}, nil},
		{"OSC 99, a title", []string{ControlOSC + "99;;Hello world" + ControlST},
			[]Notification{{Title: "Hello world"}}},
		{"OSC 99 in chunks, a title then a body", []string{
			ControlOSC + "99;i=1:d=0;Hello" + ControlST,
			ControlOSC + "99;i=1:d=0; world" + ControlST,
			ControlOSC + "99;i=1:p=body;This is cool" + ControlST},
			[]Notification{{ID: "1", Title: "Hello world", Body: "This is cool"}}},
		{"OSC 99 base64", []string{ControlOSC + "99;e=1:p=body;aGksIHRoZXJl" + ControlST},
			[]Notification{{Body: "hi, there"}}},
		{"OSC 99 chunks of another id drop the first's", []string{
			ControlOSC + "99;i=a:d=0;lost" + ControlST,
			ControlOSC + "99;i=b;kept" + ControlST},
			[]Notification{{ID: "b", Title: "kept"}}},
		{"OSC 99 close", []string{ControlOSC + "99;i=x:p=close;" + ControlST},
			[]Notification{{ID: "x", Close: true}}},
		{"OSC 99 queries, unanswered", []string{
			ControlOSC + "99;i=q:p=?;" + ControlST,
			ControlOSC + "99;i=q:p=alive;" + ControlST}, nil},
		{"OSC 99 buttons end it", []string{
			ControlOSC + "99;i=2:d=0;Proceed?" + ControlST,
			ControlOSC + "99;i=2:p=buttons;Yes\u2028No" + ControlST},
			[]Notification{{ID: "2", Title: "Proceed?"}}},
		{"OSC 99 with nothing to say", []string{ControlOSC + "99;i=3;" + ControlST}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := notified(t, tc.seqs...); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("told %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A program sending chunks forever builds a capped notification, cut at a
// rune's start.
func TestAChunkedNotificationIsCapped(t *testing.T) {
	t.Parallel()
	seqs := make([]string, 0, 3000)
	for range 3000 {
		seqs = append(seqs, ControlOSC+"99;i=1:d=0;é"+ControlST)
	}
	seqs = append(seqs, ControlOSC+"99;i=1;!"+ControlST)
	got := notified(t, seqs...)
	if len(got) != 1 {
		t.Fatalf("told %d notifications, want one", len(got))
	}
	if len(got[0].Title) > maxNotification || !strings.HasPrefix(got[0].Title, "éé") ||
		strings.ToValidUTF8(got[0].Title, "?") != got[0].Title {
		t.Fatalf("its title is %d bytes; want at most %d, valid UTF-8, of é", len(got[0].Title), maxNotification)
	}
}

// Text is the line a notification is said in.
func TestNotificationText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n    Notification
		want string
	}{
		{Notification{Body: "b"}, "b"},
		{Notification{Title: "t"}, "t"},
		{Notification{Title: "t", Body: "b"}, "t: b"},
	} {
		if got := tc.n.Text(); got != tc.want {
			t.Errorf("%+v.Text() = %q, want %q", tc.n, got, tc.want)
		}
	}
}
