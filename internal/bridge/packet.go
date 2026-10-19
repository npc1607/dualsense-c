// Package bridge defines the wire format between the in-prefix Windows helper
// (cmd/ds5bridge) and the native daemon, plus the shared-memory layouts it can
// read.
//
// The format is intentionally name-agnostic: fields carry the id of a signal in
// the standard vocabulary (see internal/signal), so adding a field to a layout
// never changes the decoder on the daemon side.
package bridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const (
	// Magic is "DS5B" little-endian.
	Magic uint32 = 0x42355344
	// Version of the packet layout.
	Version uint16 = 1
	// HeaderSize is the fixed prefix length.
	HeaderSize = 8
	// FieldSize is id (1 byte) + float32 value (4 bytes).
	FieldSize = 5
	// MaxFields is bounded by the 1-byte count.
	MaxFields = 255
)

// ErrShortPacket is returned for truncated datagrams.
var ErrShortPacket = errors.New("bridge: short packet")

// ErrBadMagic is returned for datagrams that are not bridge packets.
var ErrBadMagic = errors.New("bridge: bad magic")

// Field is one named signal value.
type Field struct {
	ID    uint8
	Value float32
}

// Encode serialises fields into a datagram.
func Encode(fields []Field) ([]byte, error) {
	if len(fields) > MaxFields {
		return nil, fmt.Errorf("bridge: too many fields (%d > %d)", len(fields), MaxFields)
	}
	b := make([]byte, HeaderSize+len(fields)*FieldSize)
	binary.LittleEndian.PutUint32(b[0:], Magic)
	binary.LittleEndian.PutUint16(b[4:], Version)
	b[6] = uint8(len(fields))
	b[7] = 0
	for i, f := range fields {
		off := HeaderSize + i*FieldSize
		b[off] = f.ID
		binary.LittleEndian.PutUint32(b[off+1:], math.Float32bits(f.Value))
	}
	return b, nil
}

// Decode parses a datagram. It returns fields in the order they were sent.
func Decode(b []byte) ([]Field, error) {
	if len(b) < HeaderSize {
		return nil, ErrShortPacket
	}
	if binary.LittleEndian.Uint32(b[0:]) != Magic {
		return nil, ErrBadMagic
	}
	if v := binary.LittleEndian.Uint16(b[4:]); v != Version {
		return nil, fmt.Errorf("bridge: unsupported version %d", v)
	}
	n := int(b[6])
	if len(b) < HeaderSize+n*FieldSize {
		return nil, ErrShortPacket
	}
	out := make([]Field, n)
	for i := 0; i < n; i++ {
		off := HeaderSize + i*FieldSize
		out[i] = Field{
			ID:    b[off],
			Value: math.Float32frombits(binary.LittleEndian.Uint32(b[off+1:])),
		}
	}
	return out, nil
}
