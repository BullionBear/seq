package codec

import (
	"encoding/binary"
	"unsafe"
)

// Cursor is a bounds-checked sequential writer over a byte slice with a
// sticky error, replacing hand-maintained `pos += 8` offset arithmetic in
// variable-size encoders. All integer writes are little-endian (the wire
// byte order). After the last write, check Err() once.
type Cursor struct {
	buf []byte
	pos int
	err error
}

// NewCursor returns a Cursor writing into buf starting at offset 0.
func NewCursor(buf []byte) Cursor {
	return Cursor{buf: buf}
}

// ensure reports whether n more bytes fit; on failure it sets the sticky error.
func (c *Cursor) ensure(n int) bool {
	if c.err != nil {
		return false
	}
	if len(c.buf)-c.pos < n {
		c.err = ErrBufferTooSmall
		return false
	}
	return true
}

// PutUint64 writes v little-endian and advances the cursor.
func (c *Cursor) PutUint64(v uint64) {
	if !c.ensure(8) {
		return
	}
	binary.LittleEndian.PutUint64(c.buf[c.pos:], v)
	c.pos += 8
}

// PutUint32 writes v little-endian and advances the cursor.
func (c *Cursor) PutUint32(v uint32) {
	if !c.ensure(4) {
		return
	}
	binary.LittleEndian.PutUint32(c.buf[c.pos:], v)
	c.pos += 4
}

// PutBytes copies b and advances the cursor.
func (c *Cursor) PutBytes(b []byte) {
	if !c.ensure(len(b)) {
		return
	}
	copy(c.buf[c.pos:], b)
	c.pos += len(b)
}

// PutString copies s and advances the cursor.
func (c *Cursor) PutString(s string) {
	if !c.ensure(len(s)) {
		return
	}
	copy(c.buf[c.pos:], s)
	c.pos += len(s)
}

// Pos returns the number of bytes written so far.
func (c *Cursor) Pos() int { return c.pos }

// Err returns the sticky error (nil, or ErrBufferTooSmall once any write
// did not fit). Writes after a failure are no-ops.
func (c *Cursor) Err() error { return c.err }

// Put appends the raw memory of the POD wire type *v at the cursor.
// Same layout contract as Encode.
func Put[T any](c *Cursor, v *T) {
	size := int(unsafe.Sizeof(*v))
	if !c.ensure(size) {
		return
	}
	copy(c.buf[c.pos:], unsafe.Slice((*byte)(unsafe.Pointer(v)), size))
	c.pos += size
}
