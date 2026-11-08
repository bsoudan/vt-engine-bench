// Package wire is the serialization boundary. A connection between two hosts
// carries only bytes, so it must serialize every frame on the way out and
// deserialize it on the way in.
//
// Two problems fall out of that, and this package owns both:
//
//   - Polymorphism. A frame's payload is an nxlink.Event interface (a
//     discovery message, a view frame, a surface's geometry, …). Bytes have no
//     type; to reconstruct the concrete value the decoder needs a tag→type
//     registry. Each payload type registers a stable tag and a codec
//     (Register); EncodeEvent/DecodeEvent round-trip through it. The host
//     never names the payload types — the registry does.
//
// Tags are "iface.leaf" for an interface's payloads (vtappmeta.input,
// surface.role), "journal.frame", "journal.resume" and "journal.proposal"
// for the journal's, whatever leaf carries them (host/announce.go) — a
// picture journal's frames among them, merged in a queue by the leaf's
// declared merging, not by their type (host.Merging) — and a bare word (hello, want, offer, retract, goodbye) for the control messages
// the hosts themselves exchange; every registration is a wire.Register* call
// in the payload's package init.
//
//   - Framing. A byte stream has no message boundaries. The sender length-prefixes
//     each frame; the receiver feeds whatever it reads into a FrameReader that
//     reassembles and pops whole frames back out.
//
// A byte codec is written over Writer and Reader (codec.go): its fields
// put and got in one order, numbers as varints, a string its length
// first. The tag before a payload is one such string.
package wire

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"nx/nxlink"
)

// errUnknownTag means a payload arrived with no registered codec.
func errUnknownTag(tag string) error { return fmt.Errorf("wire: unknown payload tag %q", tag) }

// ErrEncode is a value that cannot be sent: a type with no codec, one its
// codec cannot marshal (a NaN, under JSON), or a frame past MaxFrame. The
// send that made it fails with it, and nothing goes on the wire: the
// sender hears it, rather than its core crashing or the peer ending the
// connection over a frame it can't take.
var ErrEncode = errors.New("wire: cannot be sent")

// Codec marshals/unmarshals one concrete payload type to and from bytes. Marshal
// receives the value as an nxlink.Event (assert to the concrete type inside);
// Unmarshal returns the reconstructed value. A byte codec's Marshal cannot
// fail; a JSON one's can (RegisterJSON), and says so through marshal.
type Codec struct {
	Marshal   func(nxlink.Event) []byte
	Unmarshal func([]byte) (nxlink.Event, error)
	JSON      bool // what Marshal makes is JSON (RegisterJSON), so a reader may show it as it is

	// marshal, set by RegisterJSON, is Marshal for a value it may not take.
	marshal func(nxlink.Event) ([]byte, error)
}

// encode is ev as c makes it, or why it can't be.
func (c Codec) encode(ev nxlink.Event) ([]byte, error) {
	if c.marshal != nil {
		return c.marshal(ev)
	}
	return c.Marshal(ev), nil
}

var (
	byTag  = map[string]Codec{}        // tag -> codec (decode side)
	tagOf  = map[reflect.Type]string{} // concrete type -> tag (encode side)
	typeOf = map[string]reflect.Type{} // tag -> concrete type (FromJSON)
)

// Register wires a payload type's tag and codec. sample is a zero value of the
// type, used only for its dynamic type. Called from each payload's package
// init. Registering the same type under the same tag again just replaces the
// codec; registering two types under one tag, or one type under two tags,
// would make the registry ambiguous in one direction or the other, and is a
// programming error that panics.
func Register(tag string, sample nxlink.Event, c Codec) {
	t := reflect.TypeOf(sample)
	if was, known := tagOf[t]; known && was != tag {
		panic("wire: " + t.String() + " is already registered as " + was)
	}
	if _, taken := byTag[tag]; taken && tagOf[t] != tag {
		panic("wire: tag " + tag + " is already registered for another type")
	}
	byTag[tag] = c
	tagOf[t] = tag
	typeOf[tag] = t
}

// FromJSON is the payload under tag from its JSON as encoding/json makes
// it of the payload's type — how a recording of a trace keeps a byte
// codec's payload, decoded (nxctl trace --json, read back by --read) — and
// whether its codec makes JSON, whose payload is that JSON itself. A tag
// with no codec is an error.
func FromJSON(tag string, b []byte) (ev nxlink.Event, isJSON bool, err error) {
	c, ok := byTag[tag]
	if !ok {
		return nil, false, errUnknownTag(tag)
	}
	if c.JSON {
		ev, err = c.Unmarshal(b)
		return ev, true, err
	}
	v := reflect.New(typeOf[tag])
	if err := json.Unmarshal(b, v.Interface()); err != nil {
		return nil, false, fmt.Errorf("wire: %s: %w", tag, err)
	}
	return v.Elem().Interface().(nxlink.Event), false, nil
}

// RegisterJSON registers a payload type under tag with a codec that marshals
// it as JSON: the one codec every payload uses unless it carries raw bytes in
// bulk (a vtapp's pty output), which JSON would base64. A field tagged `json:"-"`
// stays on its own host.
func RegisterJSON[T nxlink.Event](tag string) {
	var zero T
	Register(tag, zero, Codec{
		marshal: func(ev nxlink.Event) ([]byte, error) {
			b, err := json.Marshal(ev.(T))
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %w", ErrEncode, tag, err)
			}
			return b, nil
		},
		Unmarshal: func(b []byte) (nxlink.Event, error) {
			var v T
			if err := json.Unmarshal(b, &v); err != nil {
				return nil, fmt.Errorf("wire: %s: %w", tag, err)
			}
			return v, nil
		},
		JSON: true,
	})
}

// Encode is a payload's tag and bytes, as EncodeEvent makes them but apart,
// and whether the bytes are JSON. ok is false for a type with no codec (one
// that never crosses a connection) or a value its codec can't marshal,
// which a caller looking on (a trace) shows some other way, where
// EncodeEvent would fail.
func Encode(ev nxlink.Event) (tag string, payload []byte, isJSON, ok bool) {
	tag, ok = tagOf[reflect.TypeOf(ev)]
	if !ok {
		return "", nil, false, false
	}
	c := byTag[tag]
	payload, err := c.encode(ev)
	if err != nil {
		return "", nil, false, false
	}
	return tag, payload, c.JSON, true
}

// Decode is Encode's inverse: the payload under tag, as its type.
func Decode(tag string, payload []byte) (nxlink.Event, error) {
	c, ok := byTag[tag]
	if !ok {
		return nil, errUnknownTag(tag)
	}
	return c.Unmarshal(payload)
}

// EncodeEvent serializes a polymorphic payload as [tag][payload bytes]. A type
// with no codec (a payload crossing a connection that was never meant to),
// or a value its codec can't marshal, is ErrEncode: the send fails, and
// says why, where it once panicked inside the sending core.
func EncodeEvent(ev nxlink.Event) ([]byte, error) {
	tag, ok := tagOf[reflect.TypeOf(ev)]
	if !ok {
		return nil, fmt.Errorf("%w: no codec for %T", ErrEncode, ev)
	}
	payload, err := byTag[tag].encode(ev)
	if err != nil {
		return nil, err
	}
	w := make(Writer, 0, 1+len(tag)+len(payload))
	w.String(tag)
	w.Raw(payload)
	return w, nil
}

// DecodeEvent is the inverse: read the tag, look up the codec, unmarshal the rest.
func DecodeEvent(b []byte) (nxlink.Event, error) {
	tag, rest, err := Tag(b)
	if err != nil {
		return nil, err
	}
	c, ok := byTag[tag]
	if !ok {
		return nil, errUnknownTag(tag)
	}
	return c.Unmarshal(rest)
}

// Tag is the tag EncodeEvent put before a payload, and the payload's
// bytes after it.
func Tag(b []byte) (tag string, payload []byte, err error) {
	r := NewReader(b)
	tag = r.String()
	if r.err != nil {
		return "", nil, r.err
	}
	return tag, r.b, nil
}

// --- stream framing ---

// LengthPrefix wraps a frame blob with a uint32 length, producing the bytes that
// go onto the stream. The receiver's FrameReader keys off this prefix.
func LengthPrefix(blob []byte) []byte {
	out := make([]byte, 4, 4+len(blob))
	binary.BigEndian.PutUint32(out, uint32(len(blob)))
	return append(out, blob...)
}

// MaxFrame bounds a frame's length: far past any frame nx sends, and short
// of what any four bytes of text read as a length come to — a first byte
// that prints is 512MB at least. So bytes that are not nx's at all (a
// .bashrc's greeting on the ssh path, the wrong socket) are known for what
// they are at once, not buffered while waiting for a frame that never ends.
const MaxFrame = 64 << 20

// FrameTooLarge is a length past MaxFrame: the stream is not nx's, or no
// longer in step, and nothing after it can be read as frames.
type FrameTooLarge struct{ Len uint32 }

func (e *FrameTooLarge) Error() string {
	return fmt.Sprintf("wire: what arrived is not nx's (a frame of %d bytes, past the %d there may be)", e.Len, MaxFrame)
}

// FrameReader reassembles a byte stream (delivered as arbitrary reads) back
// into whole length-prefixed frame blobs. It is single-consumer: one per inbound
// stream.
type FrameReader struct {
	buf []byte
	err error // a length past MaxFrame, once read: the stream is over
}

// Push appends incoming bytes and returns every complete frame blob now
// available (zero or more). Returned blobs are copies, safe to keep after the
// next Push. A length past MaxFrame is an error, after the frames before it,
// and so is every Push after.
func (r *FrameReader) Push(data []byte) ([][]byte, error) {
	out, _, err := r.PushUntil(data, nil)
	return out, err
}

// PushUntil is Push, stopping after the first frame last says is the last of
// what is framed this way: what came after it, read already, is rest, and
// the reader holds nothing. rest is nil when no frame was the last, and
// empty when it was and nothing came after it yet. A nil last never stops.
func (r *FrameReader) PushUntil(data []byte, last func(blob []byte) bool) (out [][]byte, rest []byte, err error) {
	if r.err != nil {
		return nil, nil, r.err
	}
	r.buf = append(r.buf, data...)
	for len(r.buf) >= 4 {
		n := binary.BigEndian.Uint32(r.buf[:4])
		if n > MaxFrame {
			r.err, r.buf = &FrameTooLarge{Len: n}, nil
			return out, nil, r.err
		}
		if uint32(len(r.buf)-4) < n {
			break // frame not fully arrived yet
		}
		blob := append([]byte(nil), r.buf[4:4+n]...)
		out = append(out, blob)
		r.buf = r.buf[4+n:]
		if last != nil && last(blob) {
			rest, r.buf = append([]byte{}, r.buf...), nil
			return out, rest, nil
		}
	}
	if len(r.buf) == 0 {
		r.buf = nil // release the backing array once drained
	}
	return out, nil, nil
}
