// Package nxlink is the leaf primitive. An interface *leaf* — one stream of
// one nx interface — is directional: a Sink on the side that sends and a
// Source on the side that receives. A leaf is wired by whoever holds its
// Wiring: the host, for a same-host pair or a pair with a proxy that stands
// for a leaf on a peer.
//
// Every core has one Inbox, and every Source of the core delivers into it, so
// what a core hears on all its leaves is one ordered stream, lifecycle and
// payload alike, read by the core's own goroutine: no goroutine per leaf.
// Its timers (Timer) and what its own goroutines tell it (Local, LocalLeaf)
// are leaves on it too, so a core's loop is a range over the inbox (All) and
// one switch, each case asking a leaf whether the item is its own (Got,
// Connected, Disconnected, State, Fired). All is a sequence, not a handler:
// a return inside the range ends the core, as in any loop. A
// Sink's Send is synchronous: it hands the value to the leaf it is wired to
// — a Source's inbox, or a connection — under the Sink's overflow policy,
// which is applied at the door of the target's bounded buffer; so a Send
// blocks, fails or replaces exactly when the policy says, and nothing is
// queued without limit. A Sink that is not wired yet buffers, and hands the
// buffer on when it is. Lifecycle (Connected / Disconnected) is queued
// without waiting, so a host can wire and unwire from its loop, which never
// waits, whatever state the consuming core is in. A Sink learns of a disconnect the way the
// spec says it does: Send fails with ErrDisconnected.
package nxlink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// ErrFull is returned by Send under the Error policy when the buffer is full.
var ErrFull = errors.New("nxlink: buffer full")

// ErrStale is what a proxy says of a frame it drops for being of a pairing
// of its link that is over: sent before the peer heard the pairing end.
var ErrStale = errors.New("nxlink: frame of a pairing that is over")

// ErrDisconnected is returned by a Sink's Send once its link has dropped: the
// value is not buffered (drop-both, doc/spec.md). A Sink that has never been
// linked is not disconnected — it is pending, and buffers until it is wired.
var ErrDisconnected = errors.New("nxlink: disconnected")

// Policy selects Send's behavior when the buffer it delivers into is full.
type Policy int

const (
	// Block makes Send wait for space (honoring ctx). Lossless backpressure.
	Block Policy = iota
	// Error makes Send return ErrFull immediately when full. Never blocks.
	Error
	// Latest keeps only the newest value: a Send that finds an earlier value
	// of the same leaf still waiting replaces it rather than failing or
	// blocking. For whole-value state (a title, a size, a list) where anything
	// but the current value is stale the moment the next one exists, so a slow
	// or absent peer sees the present, never a history. It takes no space from
	// the buffer: at most one value per leaf is ever waiting. A value that is
	// not whole — a delta, which says only what changed — is on a leaf
	// declared with a Merge, and the value it replaces is merged into it,
	// so what it leaves unsaid is not lost with the one before.
	Latest
	// State is Latest for whole-value state, sent "on change and on bind":
	// a value equal to the last one the Sink took in its current pairing is
	// not sent again (Send says nil), since the far end has it, or will
	// before anything newer. Equal is by JSON, what crosses a connection,
	// so it is for values whose JSON is the whole of them. The Sink forgets
	// when it is unwired: a new pairing's first value always goes. What the
	// Sink hands on, it hands on under Latest.
	//
	// Three leaves are State: file.offer (an offer, immutable per version
	// and resynced by its hash), ends.state (an answer to a numbered ask)
	// and automation.state (the test rig's reading of a frontend). Every
	// other whole value is a journal with no backlog (host.StateSender),
	// whose bind is a snapshot (doc/done/journal/STATE_PLAN.md).
	State
)

// lastWords is how a Sink flushed as its core ends (Flush) hands on what it
// holds: past the target's bound, never waiting for room nor failing for
// the want of it — a few values over the bound, once, as a core goes — so
// a target that is slow, or not reading, neither holds the teardown nor
// loses the words. A Sink's own, never a core's to send under.
const lastWords Policy = -1

// Event is one element of a receive stream: a payload value (which must
// implement Event) or a lifecycle event. Payload types opt in with EventBase.
type Event interface{ isNxEvent() }

// Merge is a leaf's merging, declared with the leaf (host.Merging), for a
// leaf sent under Latest whose values do not stand alone: what replaces
// prev where it waits when next is sent after it, the one value that says
// what prev, then next, said. A delta merged into the delta before it
// carries the rows of both; into a whole value, it is the whole value it
// makes. prev is the leaf's own value, the one sent just before. Each end
// of the leaf is made with it — its Sink, its Source, a proxy of it, a
// connection's stream of it — so a value waiting in any of their buffers
// is merged into; a leaf with none has the newer replace the older.
type Merge func(prev, next Event) Event

// MergeOf is merge as a leaf of T's Merge: a prev that is not a T (none
// is, on such a leaf) is replaced.
func MergeOf[T Event](merge func(prev, next T) T) Merge {
	return func(prev, next Event) Event {
		p, ok := prev.(T)
		n, ok2 := next.(T)
		if !ok || !ok2 {
			return next
		}
		return merge(p, n)
	}
}

// merged is what replaces prev, waiting, when next is sent under Latest:
// next, with prev merged into it by m, if the leaf has one.
func (m Merge) merged(prev, next Event) Event {
	if m == nil {
		return next
	}
	return m(prev, next)
}

// oneMerge is the Merge a constructor was given, if it was (at most one).
func oneMerge(merge []Merge) Merge {
	if len(merge) > 1 {
		panic("nxlink: a leaf has one Merge")
	}
	if len(merge) == 1 {
		return merge[0]
	}
	return nil
}

// EventBase, embedded in a type, makes it satisfy Event.
type EventBase struct{}

func (EventBase) isNxEvent() {}

// Connected and Disconnected are the link's lifecycle events, delivered inline
// on a receive stream: each Source of a link hears them, and so does a
// provider on its handle (the item's Leaf is the handle the host gave it),
// once per consumer paired with it, after its Sources' — how a provider
// that only sends hears its consumer come and go.
//
// LetGo, on Disconnected, says why: the link crossed a connection, and this
// end let it go because its core stopped reading — what it was sent held
// the connection's reader, and every other link on it, past a bound
// (doc/ARCHITECTURE.md, "A core that stops reading"). The link is paired
// again once the core has read what it had.
type (
	Connected    struct{ EventBase }
	Disconnected struct {
		EventBase
		LetGo bool
	}
)

// ConnState is lifecycle too, for a link that crosses a connection: the
// connection's state, told when it changes. Since is when that state began —
// for stalled, when the last of what came did. A link on one host is always
// healthy, and never hears it.
type ConnState struct {
	EventBase
	State Health
	Since time.Time
}

// Health is what a connection's state is: healthy; degraded, getting
// through but unreliably, so gaps are to be expected; stalled, nothing
// come for longer than its gaps have been; or lost, the connection gone
// and being redialed. Worse as it goes up. The judge says the first three
// (ConnState); lost is never its word, but what holds the link's state
// says it (iface.Lost, the band's "reconnecting…").
type Health uint8

const (
	ConnHealthy Health = iota
	ConnDegraded
	ConnStalled
	ConnLost
)

func (h Health) String() string {
	switch h {
	case ConnDegraded:
		return "degraded"
	case ConnStalled:
		return "stalled"
	case ConnLost:
		return "lost"
	}
	return "healthy"
}

// isLifecycle says whether ev is a lifecycle event, not a payload: a mark in
// the stream that a Latest value is never moved across.
func isLifecycle(ev Event) bool {
	switch ev.(type) {
	case Connected, Disconnected, ConnState:
		return true
	}
	return false
}

// --- the inbox ---

// Item is one thing a core took from its Inbox: the event, and the leaf it
// arrived on — the core's own Source (compare it to the handle's field), a
// provider's handle (its Connected and Disconnected), or a Watch. Two items of one leaf are in the order they were delivered,
// and lifecycle is ordered against payload.
type Item struct {
	Leaf any
	Ev   Event
	slot chan struct{} // the leaf's buffer slot this item holds, released on Pop
	seq  uint64        // how many items the inbox had pushed before it
}

// Inbox is a core's one receive stream: an ordered queue fed by every Source
// the core holds, and by its Watches, and read by the core alone. Delivery
// never blocks on the reader for lifecycle; payload waits for room in the
// delivering leaf's own buffer, which is the backpressure a Sink policy sees.
type Inbox struct {
	mu      sync.Mutex
	pending []Item
	wake    chan struct{} // cap 1: a receive means Pop has something
	done    chan struct{} // closed with the inbox: releases whoever waits for room
	closed  bool
	// pushed counts the items put, and popped is how many of them are
	// behind the core: the seq of the oldest still waiting, or pushed with
	// none — so an item taken out unread (a timer's firing taken back,
	// drop) is behind it too. Written under mu: whether the core has read
	// up to a point (read), for a Sink re-wired (WireAfter).
	pushed, popped atomic.Uint64
	// caught is what waits for the core to have read up to a point
	// (CaughtUp), each closed once it has. Under mu.
	caught []caughtUp
}

// caughtUp is a wait for the core to have taken every item put before its
// inbox had pushed n.
type caughtUp struct {
	n    uint64
	done chan struct{}
}

// CaughtUp is closed once the core has taken every item its inbox holds
// now, or the inbox has closed: for whoever waits on a core that stopped
// reading to have read again (a host that let its link go).
func (b *Inbox) CaughtUp() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	done := make(chan struct{})
	if n := b.pushed.Load(); b.closed || b.read(n) {
		close(done)
	} else {
		b.caught = append(b.caught, caughtUp{n: n, done: done})
	}
	return done
}

// caughtUp closes the waits the core has now read up to. Caller holds mu.
func (b *Inbox) caughtUp() {
	kept := b.caught[:0]
	for _, c := range b.caught {
		if b.read(c.n) {
			close(c.done)
			continue
		}
		kept = append(kept, c)
	}
	clear(b.caught[len(kept):])
	b.caught = kept
}

// read says whether the core has taken every item put before its inbox
// had pushed n.
func (b *Inbox) read(n uint64) bool { return b.popped.Load() >= n }

// NewInbox makes an empty inbox.
func NewInbox() *Inbox {
	return &Inbox{wake: make(chan struct{}, 1), done: make(chan struct{})}
}

// C fires when there is something to Pop, and is closed once the inbox is.
func (b *Inbox) C() <-chan struct{} { return b.wake }

// Pop takes the next item. ok is false once the inbox is closed; until then
// a Pop after C fired always has something.
func (b *Inbox) Pop() (it Item, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.pending) == 0 {
		return Item{}, false
	}
	it = b.pending[0]
	b.pending[0] = Item{}
	b.pending = b.pending[1:]
	b.behind()
	if len(b.caught) > 0 {
		b.caughtUp()
	}
	if len(b.pending) == 0 {
		b.pending = nil // release the backing array once drained
	} else {
		b.signal()
	}
	if it.slot != nil {
		<-it.slot
		it.slot = nil
	}
	return it, true
}

// Len is how many items are waiting.
func (b *Inbox) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

// Close ends the inbox: C closes, Pop says so, and what was waiting is
// dropped. For the core's teardown.
func (b *Inbox) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	b.pending = nil
	close(b.done)
	close(b.wake)
	for _, c := range b.caught {
		close(c.done)
	}
	b.caught = nil
}

// signal wakes the reader once. Caller holds mu.
func (b *Inbox) signal() {
	if b.closed {
		return
	}
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// push appends an item. Caller holds mu, and has checked the inbox is open.
func (b *Inbox) push(it Item) {
	it.seq = b.pushed.Load()
	b.pending = append(b.pending, it)
	b.pushed.Add(1)
	b.signal()
}

// behind sets popped after items were taken out: everything before the
// oldest still waiting is behind the core. Caller holds mu.
func (b *Inbox) behind() {
	if len(b.pending) > 0 {
		b.popped.Store(b.pending[0].seq)
		return
	}
	b.popped.Store(b.pushed.Load())
}

// replaceOrPush puts a Latest value: the newest waiting item of the same
// leaf is replaced, in its place, merged into it by merge (the leaf's, if
// it has one), else it is appended. Caller holds mu.
func (b *Inbox) replaceOrPush(it Item, merge Merge) {
	if i := b.replaceable(it.Leaf); i >= 0 {
		b.pending[i].Ev = merge.merged(b.pending[i].Ev, it.Ev)
		return
	}
	b.push(it)
}

// replaceable is where leaf's newest waiting item is, if a Latest value may
// replace it, or -1. Only a Latest value is replaced: not lifecycle, which
// it is never moved across, nor one that holds a slot (sent under Block or
// Error), which would be heard after it. Caller holds mu.
func (b *Inbox) replaceable(leaf any) int {
	for i := len(b.pending) - 1; i >= 0; i-- {
		if b.pending[i].Leaf != leaf {
			continue
		}
		if b.pending[i].slot == nil && !isLifecycle(b.pending[i].Ev) {
			return i
		}
		return -1
	}
	return -1
}

// PutLast queues ev from leaf under Latest, as Put does, but last: the
// leaf's value still waiting is taken out and ev goes behind everything
// put since, rather than in its place. For a whole state whose newest must
// never overtake what was said after the one it replaces: a connection's
// control state (transport's SendControlLatest), which a Retract queued
// after it must reach the peer ahead of. It never waits; ErrDisconnected
// once the inbox is closed.
func (b *Inbox) PutLast(leaf any, ev Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrDisconnected
	}
	if i := b.replaceable(leaf); i >= 0 {
		b.pending = slices.Delete(b.pending, i, i+1)
		b.behind() // taken out, replaced by the one put last
	}
	b.push(Item{Leaf: leaf, Ev: ev})
	return nil
}

// Put queues ev from leaf on behalf of a Source-like deliverer: under Block
// or Error it takes a slot from the leaf's buffer (waiting, or failing with
// ErrFull), under Latest it replaces the leaf's waiting value, merged
// into it by merge (the leaf's Merge, nil none); a Sink's last words
// (lastWords) take no slot, and never wait. open, called
// with the lock held once a slot is in hand, says whether the leaf still
// takes deliveries; false drops the event, as a closed inbox does, and
// either is ErrDisconnected: the value is gone, and its sender is told so.
// Under Block, a ctx ended by then drops it too, with ctx's error: a wait
// given up is given up whether the slot or the end was seen first. It is
// the one door into an inbox, but for PutLast's.
func (b *Inbox) Put(ctx context.Context, leaf any, ev Event, slots chan struct{}, policy Policy, merge Merge, open func() bool) error {
	it := Item{Leaf: leaf, Ev: ev}
	switch policy {
	case Block:
		select {
		case slots <- struct{}{}:
			it.slot = slots
		case <-ctx.Done():
			return ctx.Err()
		case <-b.done:
			return ErrDisconnected
		}
	case Error:
		select {
		case slots <- struct{}{}:
			it.slot = slots
		default:
			return ErrFull
		}
	case lastWords:
		// No slot: past the bound, once, as a core ends.
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || (open != nil && !open()) {
		if it.slot != nil {
			<-it.slot
		}
		return ErrDisconnected
	}
	if policy == Block && ctx.Err() != nil {
		// It ended as the slot came: what was waited for under it is not
		// wanted, whichever the select saw first.
		<-it.slot
		return ctx.Err()
	}
	if policy == Latest {
		b.replaceOrPush(it, merge)
	} else {
		b.push(it)
	}
	return nil
}

// Lifecycle queues a lifecycle event from leaf: never waits, never fails.
// A Source's own (its Wire, unwire and Tell) are written beside it rather
// than through it: each does one more thing under the same lock — marks
// itself linked, or not, or replaces a state still waiting — which a
// shared helper would hide behind a flag.
func (b *Inbox) Lifecycle(leaf any, ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.push(Item{Leaf: leaf, Ev: ev})
	}
}

// All is every item the inbox yields, in order, until ctx ends or the inbox
// closes: a core's loop ranges over it, and a switch says what each item is.
// It is a sequence, not a handler: the loop is still the core's, and a
// return inside it ends the core as it would any loop. C and Pop are for
// what drains an inbox that is not a core's.
func (b *Inbox) All(ctx context.Context) iter.Seq[Item] {
	return func(yield func(Item) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-b.wake:
			}
			it, ok := b.Pop()
			if !ok {
				if b.isClosed() {
					return
				}
				continue
			}
			if t, isTimer := it.Leaf.(*Timer); isTimer && t.once() {
				t.armed = false // heard: a timer set once is set no more
			}
			if !yield(it) {
				return
			}
		}
	}
}

// isClosed says whether the inbox has been closed.
func (b *Inbox) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// drop takes every waiting item of leaf out of the inbox: a timer's firing
// it no longer wants. Caller holds mu.
func (b *Inbox) drop(leaf any) {
	kept := b.pending[:0]
	for _, it := range b.pending {
		if it.Leaf != leaf {
			kept = append(kept, it)
		}
	}
	clear(b.pending[len(kept):])
	b.pending = kept
	// Taken out unread, but taken: a wait for the core to have read past
	// them must not wait on them (WireAfter, CaughtUp).
	b.behind()
	if len(b.caught) > 0 {
		b.caughtUp()
	}
}

// --- timers ---

// Fired is a timer's event: the time it was set for has come.
type Fired struct{ EventBase }

// Timer is a leaf a core sets for itself: it fires as an item on the core's
// inbox, in order with what its leaves deliver, whose Leaf is the timer and
// whose Ev is Fired. It is made stopped (Inbox.Timer); Reset sets it, Stop
// takes it back. A firing the core has not read when it resets or stops the
// timer is taken out of the inbox, so the core never hears a timer it has
// stopped, nor one it has set again for later.
//
// It fires with time.AfterFunc, which keeps no goroutine waiting. Reset,
// Stop and Every are the core's, called from its own goroutine.
type Timer struct {
	inbox *Inbox
	t     *time.Timer
	gen   uint64        // moved on by every Reset and Stop; guarded by the inbox's mu
	every time.Duration // set by Every: it fires again, this long after each firing
	armed bool          // Set; the core's alone
}

// Timer makes a stopped timer that fires on this inbox.
func (b *Inbox) Timer() *Timer { return &Timer{inbox: b} }

// Reset sets the timer to fire once, d from now, taking back any firing it
// had set or had queued and not yet been read.
func (t *Timer) Reset(d time.Duration) { t.set(d, 0) }

// Every sets the timer to fire every d from now on, until it is stopped or
// reset. A firing not yet read when the next comes is not doubled: the core
// hears one, as it would a time.Ticker.
func (t *Timer) Every(d time.Duration) { t.set(d, d) }

// Stop takes the timer back: what it had set does not fire, and a firing
// queued and not yet read is taken out of the inbox.
func (t *Timer) Stop() {
	b := t.inbox
	b.mu.Lock()
	defer b.mu.Unlock()
	t.stopLocked()
	t.armed = false
}

// once says the timer is set to fire once, not Every. The core's: every is
// written only by its own Reset, Every and Stop.
func (t *Timer) once() bool { return t.every == 0 }

// Set says whether the timer is set: reset, or set with Every, and neither
// stopped since nor, set once, heard to fire (All has yielded its firing).
func (t *Timer) Set() bool { return t.armed }

// stopLocked is Stop. Caller holds the inbox's mu.
func (t *Timer) stopLocked() {
	t.gen++
	t.every = 0
	if t.t != nil {
		t.t.Stop()
		t.t = nil
	}
	t.inbox.drop(t)
}

func (t *Timer) set(d, every time.Duration) {
	b := t.inbox
	b.mu.Lock()
	defer b.mu.Unlock()
	t.stopLocked()
	t.every, t.armed = every, true
	gen := t.gen
	t.t = time.AfterFunc(d, func() { t.fire(gen) })
}

// fire queues the firing set in generation gen, unless the timer has been
// stopped or reset since; for Every, it sets the next.
func (t *Timer) fire(gen uint64) {
	b := t.inbox
	b.mu.Lock()
	defer b.mu.Unlock()
	if gen != t.gen || b.closed {
		return
	}
	if t.every > 0 {
		b.replaceOrPush(Item{Leaf: t, Ev: Fired{}}, nil)
		t.t.Reset(t.every)
		return
	}
	b.push(Item{Leaf: t, Ev: Fired{}})
	t.t = nil
}

// Fired says whether it is this timer's firing.
func (t *Timer) Fired(it Item) bool { return it.Leaf == any(t) }

// --- leaves ---

// Deliverer is where a Sink hands what it sends: a Source (into its core's
// inbox), a proxy leaf, or a connection's stream. policy is the sender's,
// applied at the deliverer's buffer.
type Deliverer interface {
	Deliver(ctx context.Context, ev Event, policy Policy) error
}

// Receiver is what a connection delivers a stream's frames to: a proxy,
// told which pairing of its link each frame was sent for, which it holds
// or hands on only if that is the pairing it is in, and says ErrStale of
// otherwise. A connection delivers under the Block policy.
type Receiver interface {
	Receive(ctx context.Context, pairing uint64, ev Event) error
}

// Wiring is the type-erased control surface of a leaf, used by whoever links
// it. Wire wires the leaf: a Sink starts delivering to target, its pending
// buffer first; a Source (which ignores target) queues Connected on its
// inbox. Unwire undoes it: the Sink's buffer is discarded and its Send fails
// from then on, and the Source queues Disconnected. Close ends the leaf for
// good when its core is torn down.
//
// A linker must wire an instance's Sinks before its Sources and unwire its
// Sinks before its Sources, so a core answering Connected finds its Sinks
// wired, and no payload follows Disconnected. IsSource is what lets it order
// the passes.
type Wiring interface {
	IsSource() bool
	Wire(target Deliverer)
	Unwire()
	Close()
}

// Sink is the sending end of a leaf: the core calls Send. What a core
// says again on bind it says itself, from its own loop, with Send.
type Sink[T Event] interface {
	Send(ctx context.Context, v T) error
}

// Source is the receiving end of a leaf. It has no stream of its own: what
// arrives on it comes out of the core's Inbox, as Items whose Leaf is this
// Source. A Source is compared (it.Leaf == src), or asked whether an item is
// its own: each test checks the leaf, then the event, so a core's loop is
// one switch with a case per leaf and kind.
type Source[T Event] interface {
	isSource()
	// Got says whether it is a T from this leaf, and if so sets *v to it.
	// The payload's type is the compiler's to check.
	Got(it Item, v *T) bool
	// Connected says whether it is this leaf's Connected.
	Connected(it Item) bool
	// Disconnected says whether it is this leaf's Disconnected.
	Disconnected(it Item) bool
	// State says whether it is this leaf's ConnState, and if so sets *s.
	State(it Item, s *ConnState) bool
}

// NewSink creates a Sink with a buffer of the given capacity and overflow
// policy, initially pending (never wired): it buffers until wired, and the
// policy fires when the buffer is full. Once wired, Send delivers to the
// target under the policy; once unwired, Send returns ErrDisconnected until
// it is wired again. merge, if given (one), is the leaf's (Merge): a value
// waiting in its buffer is merged into by the next, under Latest.
func NewSink[T Event](capacity int, policy Policy, merge ...Merge) (Sink[T], Wiring) {
	s := newSink(capacity, policy, oneMerge(merge))
	return typedSink[T]{s}, s
}

// typedSink is the core's typed handle onto a sink.
type typedSink[T Event] struct{ s *sink }

func (t typedSink[T]) Send(ctx context.Context, v T) error {
	if !t.s.whole {
		return t.s.Deliver(ctx, v, t.s.policy)
	}
	return t.s.sendState(ctx, v)
}

// NewSource creates a Source delivering into inbox, whose buffer admits
// capacity payload values ahead of the reader. merge, if given (one), is
// the leaf's (Merge): a value waiting unread in the inbox is merged into by
// the next its Sink sends under Latest.
func NewSource[T Event](inbox *Inbox, capacity int, merge ...Merge) (Source[T], Wiring) {
	if capacity < 1 {
		capacity = 1
	}
	s := &source[T]{inbox: inbox, slots: make(chan struct{}, capacity), merge: oneMerge(merge)}
	return s, s
}

// Local makes a leaf with both ends in one core: the Source delivers into
// inbox, and the Sink, wired to it already, is for a goroutine of the
// core's own — a reader of a call that blocks (a pty, stdin, a signal), or
// whatever in its process has something to tell it — so that what it says
// arrives on the inbox in order with everything else the core hears. The
// Sink sends under policy into a buffer of capacity. There is no Connected:
// the leaf is wired before the core reads.
func Local[T Event](inbox *Inbox, capacity int, policy Policy) (Sink[T], Source[T]) {
	l := NewLocalLeaf[T](capacity, policy)
	return l.Sink, l.Bind(inbox)
}

// LocalLeaf is a local leaf made before the core that hears it has its
// inbox: Sink is the teller's from the start — a field set as the core is
// made, a test's hook — and what it sends waits in it, under its policy,
// until the core binds the leaf to its inbox (Bind, once, in its Run).
type LocalLeaf[T Event] struct {
	Sink     Sink[T]
	w        Wiring
	capacity int
}

// NewLocalLeaf makes a local leaf, not yet bound.
func NewLocalLeaf[T Event](capacity int, policy Policy) *LocalLeaf[T] {
	sink, w := NewSink[T](capacity, policy)
	return &LocalLeaf[T]{Sink: sink, w: w, capacity: capacity}
}

// Bind makes the leaf's Source on inbox and wires the Sink to it, handing
// on what it held. There is no Connected: the leaf is wired before the
// core reads.
func (l *LocalLeaf[T]) Bind(inbox *Inbox) Source[T] {
	src, _ := NewSource[T](inbox, l.capacity)
	s := src.(*source[T])
	s.linked = true
	l.w.Wire(s)
	return src
}

type sinkState uint8

const (
	sinkPending      sinkState = iota // never wired: buffer, the peer will come
	sinkLinked                        // delivering to the target
	sinkDisconnected                  // the link dropped: fail fast, buffer nothing
	sinkLetGo                         // a proxy's leaf let go of: what comes is dropped until it is wired again
)

// sink is the Sink implementation: a bounded pending buffer under a policy
// while unwired, and a synchronous hand-off to the target while wired. The
// pending buffer is handed on by a one-shot drainer when the sink is wired;
// values sent meanwhile queue behind it, so order holds.
//
// The pending buffer is an Inbox of its own (pend), its room the sink's
// slots: the policies mean at a Sink's buffer what they mean at a Source's,
// written once (Inbox.Put) — a Latest value waiting is replaced, however
// big the buffer, so a proxy's holds the newest, not a history.
type sink struct {
	mu       sync.Mutex
	cond     *sync.Cond
	policy   Policy
	merge    Merge // the leaf's, if it has one: what a Latest value waiting is merged into by
	state    sinkState
	target   Deliverer
	pend     *Inbox
	slots    chan struct{}
	draining bool // the drainer is handing the pending buffer on
	inflight int  // Sends admitting into the buffer right now
	// cuts counts the times the pending buffer was thrown away (discard):
	// a value that waited for room in it is taken only if none came while
	// it waited, so one sent for a pairing gone is neither handed to the
	// next nor said sent (ErrDisconnected). Changed under mu; read, too,
	// under pend's lock (Inbox.Put's open).
	cuts atomic.Uint64

	// flushed says the Sink is flushed (Flush): what it holds goes as last
	// words, past the target's bound. giveUp, while the drainer waits
	// for room in the target, ends that wait: for the value to go so, or,
	// the pairing ended (endPairing), for it to be dropped, the pairing's
	// and no other's, and the drainer not held past it by a core that
	// never reads.
	flushed bool
	giveUp  context.CancelFunc

	// pairing, on a proxy that receives from a peer, is the pairing of its
	// link it is in: a frame it is given (framed) is handed on only if it
	// was sent for that one.
	pairing uint64
	// paired, on a proxy that receives from a peer, ends with the pairing
	// it is wired for (unpair): what is on its way to the target then,
	// waiting for room, is dropped rather than handed to the next one.
	paired context.Context
	unpair context.CancelFunc

	// unread, on a Sink its core holds that was wired again after a drop,
	// is the core's inbox until the core has read what it held then
	// (WireAfter): up to the Disconnected that ended the pairing before.
	// Until then what the core sends is that pairing's, and fails.
	unread *Inbox
	armAt  uint64

	// whole, a Sink under State: last is the JSON of the last value it
	// took in this pairing (nil, none), and pairings counts the pairings
	// it has forgotten, so a send that raced an unwire is not remembered
	// into the next (pairings counts them under every policy: the drainer
	// tells by it that the pairing it waited in is over).
	whole    bool
	last     []byte
	pairings uint64
}

// sendState is Send under State: ev, unless it is the last value taken
// in this pairing.
func (s *sink) sendState(ctx context.Context, ev Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return s.Deliver(ctx, ev, Latest)
	}
	s.mu.Lock()
	same, at := s.last != nil && bytes.Equal(s.last, b), s.pairings
	s.mu.Unlock()
	if same {
		return nil
	}
	if err := s.Deliver(ctx, ev, Latest); err != nil {
		return err
	}
	s.mu.Lock()
	if s.pairings == at {
		s.last = b
	}
	s.mu.Unlock()
	return nil
}

// forget ends what a State Sink knows the far end has: its pairing is
// over. Caller holds mu.
func (s *sink) forget() {
	s.last = nil
	s.pairings++
}

// endPairing ends what the sink is handing on for the pairing it was wired
// for, if it is a proxy's, and, under State, what it knows the far end
// has. Caller holds mu.
func (s *sink) endPairing() {
	s.forget()
	s.flushed = false
	if s.giveUp != nil {
		s.giveUp() // the drainer's wait for room was this pairing's
	}
	if s.unpair != nil {
		s.unpair()
		s.paired, s.unpair = nil, nil
	}
}

// handOn delivers ev to target, under the pairing it is wired for, if the
// sink is a proxy's (paired, as it stood when the frame was taken): one
// still waiting for room in the target when that pairing ends is dropped,
// with ErrStale.
func handOn(ctx, paired context.Context, target Deliverer, ev Event, policy Policy) error {
	if paired == nil {
		return target.Deliver(ctx, ev, policy)
	}
	dctx, cancel := context.WithCancel(paired)
	defer cancel()
	defer context.AfterFunc(ctx, cancel)()
	err := target.Deliver(dctx, ev, policy)
	if err != nil && paired.Err() != nil && ctx.Err() == nil {
		return ErrStale
	}
	return err
}

// framed is a frame a proxy received from a peer, with the pairing it was
// sent for, until it is handed on or dropped.
type framed struct {
	EventBase
	ev      Event
	pairing uint64
}

// current is what to hand on of ev: a frame of the sink's pairing as its
// payload, anything else as it is. ok is false for a frame of another
// pairing. Caller holds mu.
func (s *sink) current(ev Event) (out Event, ok bool) {
	if f, isFrame := ev.(framed); isFrame {
		return f.ev, f.pairing == s.pairing
	}
	return ev, true
}

func newSink(capacity int, policy Policy, merge Merge) *sink {
	whole := policy == State
	if whole {
		policy = Latest
	}
	if policy == Latest || capacity < 1 {
		capacity = 1
	}
	s := &sink{pend: NewInbox(), slots: make(chan struct{}, capacity), policy: policy, merge: merge, whole: whole}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *sink) IsSource() bool { return false }

// Deliver is Send with an explicit policy: the sink's own for its core, the
// sender's when the sink is a proxy standing between a local leaf and a
// connection.
func (s *sink) Deliver(ctx context.Context, ev Event, policy Policy) error {
	s.mu.Lock()
	payload, current := s.current(ev)
	if s.unread != nil && s.unread.read(s.armAt) {
		s.unread = nil // the core knows the pairing before is over
	}
	switch {
	case s.unread != nil:
		// Sent before the core read that its pairing ended: that
		// pairing's, and it is gone.
		s.mu.Unlock()
		return ErrDisconnected
	case !current:
		// Sent for a pairing that is over, before the peer heard it was:
		// handed on, it would be the next pairing's twice.
		s.mu.Unlock()
		return ErrStale
	case s.state == sinkDisconnected:
		s.mu.Unlock()
		return ErrDisconnected
	case s.state == sinkLetGo:
		// For the pairing that is gone, in flight when it went: held, it
		// would wait for a pairing that may never come, and hold the
		// connection's reader with it.
		s.mu.Unlock()
		return ErrStale
	case s.state == sinkLinked && !s.draining:
		target, paired := s.target, s.paired
		if s.flushed && policy == Block {
			policy = lastWords // flushed: a core's last words, on their way through
		}
		s.mu.Unlock()
		return handOn(ctx, paired, target, payload, policy)
	}
	// Pending, or wired with the pending buffer still being handed on: queue
	// behind what is there. Latest never waits, so it is put under the lock,
	// where no discard can come between.
	if policy == Latest {
		err := s.pend.Put(ctx, s, ev, s.slots, policy, s.merge, nil)
		s.cond.Broadcast()
		s.mu.Unlock()
		return err
	}
	s.inflight++
	cut := s.cuts.Load()
	s.mu.Unlock()
	err := s.pend.Put(ctx, s, ev, s.slots, policy, s.merge, func() bool { return s.cuts.Load() == cut })
	s.mu.Lock()
	s.inflight--
	if errors.Is(err, ErrDisconnected) && s.state == sinkLetGo {
		err = ErrStale // a proxy's frame, its link let go as it waited: counted so
	}
	s.cond.Broadcast()
	s.mu.Unlock()
	return err
}

// Wire starts delivering to target. Re-wiring after a disconnect first
// discards whatever slipped into the buffer in between (drop-both: nothing
// from before the drop is replayed); a pending Sink's buffer is delivered
// whole, by a drainer that runs until it is empty.
func (s *sink) Wire(target Deliverer) { s.wire(target, nil) }

// WireAfter is Wire for a Sink a core holds, core its inbox. Wired again
// after a drop, it fails as a dropped Sink does until the core has read
// what its inbox held as it was wired, the drop's Disconnected among it:
// the core goes on sending for the pairing it is in, as it knows it, until
// it hears that pairing end, and what it sends meanwhile is that pairing's,
// not the next one's. Its first wiring hands on what it held, as Wire's.
func (s *sink) WireAfter(target Deliverer, core *Inbox) { s.wire(target, core) }

func (s *sink) wire(target Deliverer, core *Inbox) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unread = nil
	if s.state == sinkDisconnected {
		s.discard()
		if core != nil {
			s.unread, s.armAt = core, core.pushed.Load()
		}
	}
	s.state, s.target = sinkLinked, target
	if !s.draining && (s.pend.Len() > 0 || s.inflight > 0) {
		s.draining = true
		go s.drain()
	}
}

// drain hands the pending buffer on, in order, then stops; a Send that finds
// the drainer running queues behind it. Sends admitting into the buffer as
// it empties are waited for, so nothing is left behind. A frame held for a
// pairing over by now is dropped.
func (s *sink) drain() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.state == sinkLinked {
		if it, ok := s.pend.Pop(); ok {
			ev, current := s.current(it.Ev)
			if !current {
				continue
			}
			target, paired, policy := s.target, s.paired, s.policy
			if policy == Error {
				// Taken into the buffer, it was said sent: it waits for
				// room in the target, as a Block value does, rather than
				// be lost to ErrFull where no one hears of it. A Send
				// meanwhile queues behind it, and is told when there is
				// no room.
				policy = Block
			}
			if s.flushed && policy == Block {
				policy = lastWords
			}
			wait, giveUp := context.WithCancel(context.Background())
			s.giveUp = giveUp
			at := s.pairings
			s.mu.Unlock()
			err := handOn(wait, paired, target, ev, policy)
			gaveUp := wait.Err() != nil
			giveUp()
			s.mu.Lock()
			s.giveUp = nil
			if err != nil && gaveUp && s.state == sinkLinked && s.pairings == at {
				// Flushed as it waited for room: it goes as a last word.
				target, paired = s.target, s.paired
				s.mu.Unlock()
				_ = handOn(context.Background(), paired, target, ev, lastWords)
				s.mu.Lock()
			}
			continue
		}
		if s.inflight == 0 {
			break
		}
		s.cond.Wait()
	}
	s.draining = false
	s.cond.Broadcast()
}

// Unwire stops delivery; the Sink is disconnected until wired again.
func (s *sink) Unwire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == sinkLinked {
		s.state, s.target = sinkDisconnected, nil
		s.endPairing()
		s.discard()
		s.cond.Broadcast()
	}
}

// Close is Unwire for good: the leaf's core is gone.
func (s *sink) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state, s.target = sinkDisconnected, nil
	s.endPairing()
	s.discard()
	s.cond.Broadcast()
}

// reset puts a sink back to pending with nothing in it: a proxy's leaf that
// was let go of and may be taken again. Caller holds mu.
func (s *sink) reset() {
	s.state, s.target = sinkPending, nil
	s.endPairing()
	s.discard()
	s.cond.Broadcast()
}

// letGo is reset for a proxy's leaf that was paired and is let go of: what
// arrives for it from now on is dropped, not held, until it is wired again.
// Caller holds mu.
func (s *sink) letGo() {
	s.reset()
	s.state = sinkLetGo
}

// discard empties the buffer, and refuses what is on its way into it: a
// Send waiting for room as it is emptied is ErrDisconnected, not queued
// (cuts). Caller holds mu.
func (s *sink) discard() {
	s.cuts.Add(1)
	for {
		if _, ok := s.pend.Pop(); !ok {
			return
		}
	}
}

// Flusher is a leaf that can hand on, at once, what it holds.
type Flusher interface {
	Flush()
}

// Flush hands on everything the Sink holds, now, and returns once it has:
// the pending buffer is empty and nothing is being admitted, or the Sink is
// not wired. It is for after the last Send: a core that publishes its last
// word and returns is torn down right after, and without this the word
// could still be in the buffer when the leaf is unwired. What it holds goes
// as last words (lastWords): past the target's bound, so it waits for no
// room — a target slow, or not reading, has the words all the same, and
// nothing is waited for that may never come. One under Block hands on
// past the bound from then on, until its pairing ends.
func (s *sink) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != sinkLinked {
		return
	}
	s.flushed = true
	if s.giveUp != nil {
		s.giveUp()
	}
	for s.state == sinkLinked && (s.pend.Len() > 0 || s.draining || s.inflight > 0) {
		s.cond.Wait()
	}
}

// source is the Source implementation: an identity, a buffer of slots, and
// the inbox it delivers into. linked is guarded by the inbox's lock, so a
// delivery is admitted or dropped atomically with the lifecycle around it.
type source[T Event] struct {
	inbox  *Inbox
	slots  chan struct{}
	merge  Merge // the leaf's, if it has one (NewSource)
	linked bool
}

func (s *source[T]) IsSource() bool { return true }
func (s *source[T]) isSource()      {}

func (s *source[T]) Got(it Item, v *T) bool { return Got(it, s, v) }

// Got says whether it is a T from leaf, and if so sets *v to it: Source's
// Got, for what arrives on an inbox from a leaf that is not a Source (a
// watch, a spawned core's handle).
func Got[T Event](it Item, leaf any, v *T) bool {
	if it.Leaf != leaf {
		return false
	}
	x, ok := it.Ev.(T)
	if ok {
		*v = x
	}
	return ok
}

func (s *source[T]) Connected(it Item) bool {
	_, ok := it.Ev.(Connected)
	return ok && it.Leaf == any(s)
}

func (s *source[T]) Disconnected(it Item) bool {
	_, ok := it.Ev.(Disconnected)
	return ok && it.Leaf == any(s)
}

func (s *source[T]) State(it Item, st *ConnState) bool { return Got(it, s, st) }

// Inbox is the inbox this Source delivers into: the core's. For whoever
// reads a core's leaves on its behalf (a test harness).
func (s *source[T]) Inbox() *Inbox { return s.inbox }

func (s *source[T]) Wire(Deliverer) {
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	if s.inbox.closed {
		return
	}
	s.linked = true
	s.inbox.push(Item{Leaf: s, Ev: Connected{}})
}

func (s *source[T]) Unwire() { s.unwire(false) }

// LetGo is Unwire for a link this end let go because its core stopped
// reading: its Disconnected says so (Disconnected.LetGo).
func (s *source[T]) LetGo() { s.unwire(true) }

func (s *source[T]) unwire(letGo bool) {
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	if !s.linked {
		return
	}
	s.linked = false
	if !s.inbox.closed {
		s.inbox.push(Item{Leaf: s, Ev: Disconnected{LetGo: letGo}})
	}
}

// Tell queues a lifecycle event of the link's connection (ConnState)
// on the inbox, in order with what the leaf delivers, while it is linked.
// A state still waiting, with nothing of the leaf's after it, is replaced
// rather than followed: a state is whole, and Tell takes no slot, so a core
// that has stopped reading would otherwise gather one per change of its
// connection, without bound.
func (s *source[T]) Tell(ev Event) {
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	if !s.linked || s.inbox.closed {
		return
	}
	if _, state := ev.(ConnState); state {
		for i := len(s.inbox.pending) - 1; i >= 0; i-- {
			it := &s.inbox.pending[i]
			if it.Leaf != any(s) {
				continue
			}
			if _, was := it.Ev.(ConnState); was {
				it.Ev = ev
				return
			}
			break
		}
	}
	s.inbox.push(Item{Leaf: s, Ev: ev})
}

func (s *source[T]) Close() {
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	s.linked = false
}

// Deliver queues one payload on the core's inbox under the sender's policy,
// or drops it once the leaf is unwired.
func (s *source[T]) Deliver(ctx context.Context, ev Event, policy Policy) error {
	return s.inbox.Put(ctx, s, ev, s.slots, policy, s.merge, func() bool { return s.linked })
}

// --- proxies ---

// A proxy is a leaf standing in for one on a peer: a bounded buffer that
// hands on what it is given once it knows where to. There are two, one for
// each way a leaf runs across the connection, and each faces the local
// pairing as what the leaf it stands for is not. An InProxy stands for the
// peer's Sink: the connection delivers the peer's frames to it (Receive),
// and it, a Sink here, delivers them into a local Source once paired. An
// OutProxy stands for the peer's Source: a local Sink delivers into it, a
// Source here, and it delivers to the connection's stream once mounted.
// The pairing side wires a proxy with Wire and Unwire, the connection side
// with Mount and Unmount (an OutProxy) or Receive (an InProxy); either
// order works, since it buffers until it has both ends.
//
// A frame that arrives before the pairing, or a value sent before the
// mount, waits under the Block policy in a buffer of the given capacity,
// which is where backpressure reaches a connection's reader, or a local
// sender.

// InProxy is a proxy for a leaf the peer sends on.
//
// A link may be paired across a connection many times, and a peer goes on
// sending for one pairing until it hears that it ended, so each frame says
// which pairing of its link it was sent for (Receive). An InProxy is in one
// pairing at a time (Expect), and drops a frame of any other: one of the
// pairing before, handed on, would be applied to the next twice — once as
// it was in flight, and again as the peer sends it for the next.
type InProxy struct{ *sink }

// NewInProxy makes a proxy for a leaf the peer sends on.
func NewInProxy(capacity int) *InProxy { return &InProxy{newSink(capacity, Block, nil)} }

// Wire is the pairing side: the proxy delivers into the local Source from
// now on, for this pairing alone — Unwire drops what is on its way still,
// waiting for room.
func (p *InProxy) Wire(target Deliverer) {
	p.sink.mu.Lock()
	p.sink.endPairing()
	p.sink.paired, p.sink.unpair = context.WithCancel(context.Background())
	p.sink.mu.Unlock()
	p.sink.Wire(target)
}

// Receive is the connection side: a frame from the peer, sent for the
// pairing given, held or handed on as Deliver would if that is the pairing
// the proxy is in, else dropped with ErrStale.
func (p *InProxy) Receive(ctx context.Context, pairing uint64, ev Event) error {
	return p.sink.Deliver(ctx, framed{ev: ev, pairing: pairing}, Block)
}

// Unwire is the pairing side letting go: what the proxy holds is dropped,
// and so is what arrives for it until the next pairing — frames the peer
// sent before it heard, which, held, would hold the connection's reader
// until then.
func (p *InProxy) Unwire() {
	p.sink.mu.Lock()
	p.sink.letGo()
	p.sink.mu.Unlock()
}

// Expect is the pairing side naming the pairing the proxy is in from now
// on: a frame of any other is dropped, one held already among them. A
// proxy let go of holds what arrives for it again, as before the first
// pairing: the pairing is asked for on the wire before it is made here,
// and the peer's answer may come first. One not let go of keeps its state.
func (p *InProxy) Expect(pairing uint64) {
	p.sink.mu.Lock()
	p.sink.pairing = pairing
	if p.sink.state == sinkLetGo {
		p.sink.state = sinkPending
	}
	p.sink.mu.Unlock()
}

// OutProxy is a proxy for a leaf the peer receives on: a Source to the
// local pairing, whose Sink delivers into it.
type OutProxy struct{ *sink }

// NewOutProxy makes a proxy for a leaf the peer receives on; merge is the
// leaf's (Merge), nil if it has none: a Latest value held unmounted is
// merged into by the next.
func NewOutProxy(capacity int, merge Merge) *OutProxy {
	return &OutProxy{newSink(capacity, Block, merge)}
}

func (p *OutProxy) IsSource() bool { return true }

// Wire and Unwire are the pairing side, which an OutProxy has nothing to
// learn from: the local Sink delivers into it, or doesn't.
func (p *OutProxy) Wire(Deliverer) {}
func (p *OutProxy) Unwire()        {}

// Mount is the connection side: the proxy delivers to the stream from now
// on, what it held first.
func (p *OutProxy) Mount(stream Deliverer) { p.sink.Wire(stream) }

// Unmount is the connection side letting go: what the proxy holds is
// dropped, and it waits, pending, for the next mount.
func (p *OutProxy) Unmount() {
	p.sink.mu.Lock()
	p.sink.reset()
	p.sink.mu.Unlock()
}
