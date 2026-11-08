package nxlink

import "time"

// JudgeTimes are a Judge's thresholds: its feed's own, scaled to how often
// it hears from the link.
type JudgeTimes struct {
	Stall   time.Duration // a healthy link waited on this long is stalled
	Cap     time.Duration // the most an unhealthy link is waited on before it is
	Heal    time.Duration // degraded, with no trouble for this long: healthy
	Late    time.Duration // a response later than this, at least, may be late
	History time.Duration // how far back the least response is the baseline
}

// Judge is how a link's health is judged, from what its feed hears of it
// (README: link state): the transport's from its heartbeats, a view's from
// its input and what comes back of it. Each feed calls it from its own loop
// with what it saw, and reads Health after; the Judge keeps the baseline,
// the thresholds and the heal timer.
//
//   - healthy to degraded on trouble: LateCount of the latest LateWindow
//     responses late — later than three times the baseline, the least
//     response in History, and never under Late — or a miss (Missed);
//   - to stalled on a wait longer than Wait: Stall, or, once unhealthy,
//     twice the worst gap recovered from since, up to Cap — a link that has
//     shown gaps that long is not stalled by one;
//   - stalled to degraded on anything arriving: a link that has just
//     stalled has yet to show it is steady;
//   - degraded to healthy after Heal with no trouble (Tick).
type Judge struct {
	T JudgeTimes

	state       Health
	since       time.Time
	lastTrouble time.Time
	worst       time.Duration // the longest gap recovered from since it was last healthy
	lates       []bool        // whether each of the latest responses was late
	samples     []sample      // back to History, for the baseline
	changes     []ConnState   // since Changes was last asked
}

type sample struct {
	at time.Time
	d  time.Duration
}

// LateWindow is how many of the latest responses lateness is judged over,
// and LateCount how many of those late are trouble.
const (
	LateWindow = 6
	LateCount  = 2
)

// Health is the link's health, and since when.
func (j *Judge) Health() (Health, time.Time) { return j.state, j.since }

// Changes is every change of health since it was last asked, in order.
func (j *Judge) Changes() []ConnState {
	c := j.changes
	j.changes = nil
	return c
}

// IsLate says whether a response of d is late: later than three times the
// baseline, and never under T.Late. With no baseline yet, none is.
func (j *Judge) IsLate(d time.Duration) bool {
	base, ok := j.Baseline()
	return ok && d > max(3*base, j.T.Late)
}

// Sample keeps a response of d, at now, for the baseline.
func (j *Judge) Sample(now time.Time, d time.Duration) {
	j.samples = append(j.samples, sample{now, d})
	for len(j.samples) > 1 && now.Sub(j.samples[0].at) > j.T.History {
		j.samples = j.samples[1:]
	}
}

// Baseline is the least response kept, and whether there is one.
func (j *Judge) Baseline() (time.Duration, bool) {
	var least time.Duration
	for i, s := range j.samples {
		if i == 0 || s.d < least {
			least = s.d
		}
	}
	return least, len(j.samples) > 0
}

// Answered takes whether a response, at now, was late: LateCount of the
// latest LateWindow late is trouble.
func (j *Judge) Answered(now time.Time, late bool) {
	j.lates = append(j.lates, late)
	if len(j.lates) > LateWindow {
		j.lates = j.lates[1:]
	}
	if !late {
		return
	}
	n := 0
	for _, l := range j.lates {
		if l {
			n++
		}
	}
	if n >= LateCount {
		j.Missed(now)
	}
}

// Missed is trouble at now, a sign the link is unreliable: a healthy link
// is degraded by it, and an unhealthy one kept from healing for Heal.
func (j *Judge) Missed(now time.Time) {
	j.lastTrouble = now
	if j.state == ConnHealthy {
		j.become(ConnDegraded, now)
	}
}

// Arrived is something come at now after a wait of gap: a gap recovered
// from, which a link not healthy remembers the worst of, and the end of a
// stall — degraded again, not healthy.
func (j *Judge) Arrived(now time.Time, gap time.Duration) {
	if j.state == ConnHealthy {
		return
	}
	j.worst = max(j.worst, gap)
	if j.state == ConnStalled {
		j.lastTrouble = now
		j.become(ConnDegraded, now)
	}
}

// Wait is how long a wait stalls the link: T.Stall, or, once unhealthy,
// twice the worst gap recovered from since, up to T.Cap.
func (j *Judge) Wait() time.Duration {
	if j.state == ConnHealthy {
		return j.T.Stall
	}
	return min(max(j.T.Stall, 2*j.worst), j.T.Cap)
}

// Silent is a wait, at now, of waited, begun at since: as long as Wait, it
// stalls the link, as of since.
func (j *Judge) Silent(now time.Time, waited time.Duration, since time.Time) {
	if waited >= j.Wait() && j.state != ConnStalled {
		j.lastTrouble = now
		j.become(ConnStalled, since)
	}
}

// Tick heals the link, degraded with no trouble for T.Heal, at now.
func (j *Judge) Tick(now time.Time) {
	if j.state == ConnDegraded && now.Sub(j.lastTrouble) > j.T.Heal {
		j.become(ConnHealthy, now)
	}
}

// HealAt is when a degraded link heals, if no trouble comes first.
func (j *Judge) HealAt() time.Time { return j.lastTrouble.Add(j.T.Heal) }

func (j *Judge) become(h Health, since time.Time) {
	if h == j.state {
		return
	}
	if h == ConnHealthy {
		j.worst = 0
	}
	j.state, j.since = h, since
	j.changes = append(j.changes, ConnState{State: h, Since: since})
}
