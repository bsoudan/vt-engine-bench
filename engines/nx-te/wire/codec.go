package wire

import (
	"encoding/binary"
	"errors"
)

// A byte codec is a list of puts and gets (doc/done/DEVICE_PLAN.md, The
// design 12): a Writer appends a payload's fields, a Reader reads them
// back in the same order. Numbers are varints, a string or a byte slice
// its length (a uvarint) and its bytes. Every byte codec in nx is written
// over these two: a frame's header (package journal), a delta (cell.Delta),
// a picture's tiles, an input, a connection's frame (package transport),
// and the tag before each payload here (EncodeEvent).

// ErrBytes is bytes a Reader could not read as what was asked of it: cut
// short, a number past what the codec takes, a count past the bytes left,
// or bytes left over at its end.
var ErrBytes = errors.New("wire: bytes cut short or malformed")

// Writer is a codec's bytes as it puts them. The zero Writer is empty;
// Writer(b) appends to b.
type Writer []byte

// Byte puts c.
func (w *Writer) Byte(c byte) { *w = append(*w, c) }

// Bool puts v as a byte, 1 or 0.
func (w *Writer) Bool(v bool) {
	c := byte(0)
	if v {
		c = 1
	}
	w.Byte(c)
}

// Uvarint puts v as a uvarint.
func (w *Writer) Uvarint(v uint64) { *w = binary.AppendUvarint(*w, v) }

// Varint puts v as a varint.
func (w *Writer) Varint(v int64) { *w = binary.AppendVarint(*w, v) }

// Int puts n, which is never negative, as a uvarint: a size, a place on a
// grid or a picture. A negative n is put as 0.
func (w *Writer) Int(n int) { w.Uvarint(uint64(max(n, 0))) }

// SignedInt puts n as a varint.
func (w *Writer) SignedInt(n int) { w.Varint(int64(n)) }

// String puts s: its length, then its bytes.
func (w *Writer) String(s string) {
	w.Uvarint(uint64(len(s)))
	*w = append(*w, s...)
}

// Bytes puts b as String puts a string.
func (w *Writer) Bytes(b []byte) {
	w.Uvarint(uint64(len(b)))
	*w = append(*w, b...)
}

// Raw puts b as it is, no length before it: what a codec's last field is
// when it is the rest of the bytes (Reader.Rest).
func (w *Writer) Raw(b []byte) { *w = append(*w, b...) }

// Reader reads what a Writer put, in the order it put it. The first thing
// it can't read is kept (Err), and every get after it is zero, so a codec
// reads every field and asks once at the end (End).
type Reader struct {
	b   []byte
	err error
}

// NewReader reads b.
func NewReader(b []byte) *Reader { return &Reader{b: b} }

// Err is the first thing the reader could not read: nil, none yet.
func (r *Reader) Err() error { return r.err }

// Fail keeps err as the reader's error, if it has none yet: a codec's own
// check of what it read (a kind it does not know).
func (r *Reader) Fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

// Len is how many bytes are left.
func (r *Reader) Len() int { return len(r.b) }

// End is the reader's error, or ErrBytes if bytes are left: what a codec
// whose fields are all of its bytes says at its end.
func (r *Reader) End() error {
	if r.err == nil && len(r.b) > 0 {
		r.err = ErrBytes
	}
	return r.err
}

// Byte gets a byte.
func (r *Reader) Byte() byte {
	if r.err != nil {
		return 0
	}
	if len(r.b) == 0 {
		r.err = ErrBytes
		return 0
	}
	c := r.b[0]
	r.b = r.b[1:]
	return c
}

// Bool gets what Writer.Bool put: a byte that is neither 1 nor 0 is
// ErrBytes.
func (r *Reader) Bool() bool {
	c := r.Byte()
	if c > 1 {
		r.Fail(ErrBytes)
	}
	return c == 1
}

// Uvarint gets a uvarint.
func (r *Reader) Uvarint() uint64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.err = ErrBytes
		return 0
	}
	r.b = r.b[n:]
	return v
}

// Varint gets a varint.
func (r *Reader) Varint() int64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Varint(r.b)
	if n <= 0 {
		r.err = ErrBytes
		return 0
	}
	r.b = r.b[n:]
	return v
}

// Int gets what Writer.Int put, at most limit: past it is ErrBytes, so a
// size read from bytes that are not one never makes a slice of it.
func (r *Reader) Int(limit int) int {
	v := r.Uvarint()
	if v > uint64(limit) {
		r.Fail(ErrBytes)
		return 0
	}
	return int(v)
}

// SignedInt gets what Writer.SignedInt put, between -limit and limit.
func (r *Reader) SignedInt(limit int) int {
	v := r.Varint()
	if v > int64(limit) || v < -int64(limit) {
		r.Fail(ErrBytes)
		return 0
	}
	return int(v)
}

// Count gets a number of things to come, each at least a byte: more than
// the bytes left is ErrBytes, so a count read from bytes that are not one
// never makes a slice of it.
func (r *Reader) Count() int {
	v := r.Uvarint()
	if v > uint64(len(r.b)) {
		r.Fail(ErrBytes)
		return 0
	}
	return int(v)
}

// String gets what Writer.String put.
func (r *Reader) String() string {
	n := r.Count()
	if r.err != nil {
		return ""
	}
	s := string(r.b[:n])
	r.b = r.b[n:]
	return s
}

// Bytes gets what Writer.Bytes put, a copy: nil when it is none.
func (r *Reader) Bytes() []byte {
	n := r.Count()
	if r.err != nil || n == 0 {
		return nil
	}
	b := append([]byte(nil), r.b[:n]...)
	r.b = r.b[n:]
	return b
}

// Rest gets the bytes left: what Writer.Raw put last. They are the
// reader's bytes, not a copy; nil after an error.
func (r *Reader) Rest() []byte {
	if r.err != nil {
		return nil
	}
	b := r.b
	r.b = nil
	return b
}
