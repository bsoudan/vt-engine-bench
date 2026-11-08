package te

import "testing"

// A query of a selection (OSC 52 ?) is answered at once with what the
// program put there itself, when nothing holds queries; with a hook set, it
// goes to the hook, unanswered, and is answered only when the hook's owner
// says so.
func TestASelectionQueryGoesToItsHook(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, query, selection string
	}{
		{"the clipboard", "52;c;?", "c"},
		{"BEL-terminated, bare", "52;?", "s0"},
		{"primary", "52;p;?", "p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			screen := NewScreen(80, 24)
			stream := NewStream(screen)
			var wrote []string
			screen.WriteProcessInput = func(s string) { wrote = append(wrote, s) }
			_ = stream.Feed(ControlOSC + "52;" + tc.selection + ";b3du" + ControlST) // "own"
			want := ControlOSC + "52;" + tc.selection + ";b3du" + ControlST

			// No hook: answered at once.
			_ = stream.Feed(ControlOSC + tc.query + ControlST)
			if len(wrote) != 1 || wrote[0] != want {
				t.Fatalf("without a hook the query was answered %q, want [%q]", wrote, want)
			}

			// A hook: told, not answered.
			wrote = nil
			var asked []string
			screen.SelectionQuery = func(sel string) { asked = append(asked, sel) }
			_ = stream.Feed(ControlOSC + tc.query + "\x07")
			if len(wrote) != 0 {
				t.Fatalf("with a hook the query was answered %q", wrote)
			}
			if len(asked) != 1 || asked[0] != tc.selection {
				t.Fatalf("the hook was told %q, want [%q]", asked, tc.selection)
			}
			// Its owner answers, with what the program holds then.
			screen.AnswerSelectionData(asked[0])
			if len(wrote) != 1 || wrote[0] != want {
				t.Fatalf("the answer was %q, want [%q]", wrote, want)
			}
		})
	}
}

// Each selection a program sets (OSC 52) is told to SelectionSet as it is
// parsed, after it is stored, by its name and the data as sent: the same
// data twice is two sets, the empty name is "s0" (tmux's) and several
// letters are the name as given (xterm's "cp").
func TestEachSelectionSetGoesToItsHook(t *testing.T) {
	t.Parallel()
	screen := NewScreen(80, 24)
	stream := NewStream(screen)
	type set struct{ sel, data, stored string }
	var got []set
	screen.SelectionSet = func(sel, data string) {
		got = append(got, set{sel, data, screen.SelectionData(sel)})
	}
	_ = stream.Feed(ControlOSC + "52;c;aGk=" + ControlST)
	_ = stream.Feed(ControlOSC + "52;c;aGk=\x07")
	_ = stream.Feed(ControlOSC + "52;;dG11eA==" + ControlST)
	_ = stream.Feed(ControlOSC + "52;cp;eHRlcm0=" + ControlST)
	_ = stream.Feed(ControlOSC + "52;c;?" + ControlST) // a query is no set
	want := []set{
		{"c", "aGk=", "aGk="},
		{"c", "aGk=", "aGk="},
		{"s0", "dG11eA==", "dG11eA=="},
		{"cp", "eHRlcm0=", "eHRlcm0="},
	}
	if len(got) != len(want) {
		t.Fatalf("the hook was told %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the hook was told %+v, want %+v", got, want)
		}
	}
}
