package te

import "testing"

// TestHotModesFollowMap checks that the modes isModeSet answers from
// Screen.hot agree with the Mode map after every way the map is written: a
// set and a reset, ANSI and private, a save and restore, a soft reset, a
// full reset, DECCOLM's side effects, and a state restored from a snapshot.
func TestHotModesFollowMap(t *testing.T) {
	hot := []int{ModeDECLRMM, ModeDECOM, ModeDECAWM, ModeIRM, ModeDECSCNM}
	check := func(s *Screen, after string) {
		t.Helper()
		for _, m := range hot {
			_, inMap := s.Mode[m]
			if got := s.isModeSet(m); got != inMap {
				t.Fatalf("after %s: isModeSet(%d) = %v, Mode has it: %v", after, m, got, inMap)
			}
		}
	}
	s := NewScreen(20, 5)
	check(s, "NewScreen")
	for _, m := range []int{69, 6, 7, 5} {
		s.SetMode([]int{m}, true)
		check(s, "DECSET")
		s.ResetMode([]int{m}, true)
		check(s, "DECRST")
		s.SetMode([]int{m}, true)
		check(s, "DECSET again")
	}
	s.SetMode([]int{4}, false)
	check(s, "SM 4")
	s.ResetMode([]int{4}, false)
	check(s, "RM 4")
	s.SetMode([]int{4}, false)
	check(s, "SM 4 again")
	s.SaveModes([]int{69, 6, 7, 5})
	s.ResetMode([]int{69, 6, 7, 5}, true)
	check(s, "DECRST of saved")
	s.RestoreModes([]int{69, 6, 7, 5})
	check(s, "restore")
	s.SetMode([]int{3}, true)
	check(s, "DECCOLM set")
	s.ResetMode([]int{3}, true)
	check(s, "DECCOLM reset")
	s.SoftReset()
	check(s, "DECSTR")
	s.SetMode([]int{69, 6, 4}, true)
	s.Reset()
	check(s, "RIS")

	s.SetMode([]int{69, 6, 7, 5}, true)
	s.SetMode([]int{4}, false)
	st := s.MarshalState()
	restored := NewScreen(20, 5)
	restored.UnmarshalState(st)
	check(restored, "UnmarshalState")
	if !restored.isModeSet(ModeDECLRMM) || !restored.isModeSet(ModeIRM) {
		t.Fatalf("restored screen lost its modes")
	}
}
