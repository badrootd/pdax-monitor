package binary

import (
	"encoding/binary"
	"math"
)

// ReadCursor represents pointer to byte array with ability to read arbitrary data types.
type ReadCursor struct {
	CurPos uint // byte position
	Data   []byte
}

// ReadUint8 used to read uint8.
func (rc *ReadCursor) ReadUint8() uint8 {
	defer func() {
		rc.CurPos++
	}()

	return rc.Data[rc.CurPos]
}

// ReadUint32 used to read uint32.
func (rc *ReadCursor) ReadUint32() uint32 {
	defer func() {
		rc.CurPos += 4
	}()

	return binary.BigEndian.Uint32(rc.Data[rc.CurPos:])
}

// ReadUint16 used to read uint16.
func (rc *ReadCursor) ReadUint16() uint16 {
	defer func() {
		rc.CurPos += 2
	}()

	return binary.BigEndian.Uint16(rc.Data[rc.CurPos:])
}

// ReadUint16Float used to read uint16 with float64 conversion.
func (rc *ReadCursor) ReadUint16Float() float64 {
	defer func() {
		rc.CurPos += 2
	}()

	return math.Float64frombits(uint64(binary.BigEndian.Uint16(rc.Data[rc.CurPos:])))
}

// ReadFloat64 used to read float64.
func (rc *ReadCursor) ReadFloat64() float64 {
	defer func() {
		rc.CurPos += 8
	}()

	return math.Float64frombits(binary.BigEndian.Uint64(rc.Data[rc.CurPos:]))
}

// Advance used to shift cursor.
func (rc *ReadCursor) Advance(shift uint) {
	rc.CurPos += shift
}

// WriteCursor represents pointer to byte array with ability to write arbitrary data types.
type WriteCursor struct {
	CurPos uint // byte position
	Data   []byte
}

// WriteFloat64 used to write float64.
func (wc *WriteCursor) WriteFloat64(f float64) {
	defer func() {
		wc.CurPos += 8
	}()

	binary.BigEndian.PutUint64(wc.Data[wc.CurPos:], math.Float64bits(f))
}

// WriteUint8 used to write uint8.
func (wc *WriteCursor) WriteUint8(v uint8) {
	defer func() {
		wc.CurPos++
	}()

	wc.Data[wc.CurPos] = v
}

// WriteUint16 used to write uint16.
func (wc *WriteCursor) WriteUint16(v uint16) {
	defer func() {
		wc.CurPos += 2
	}()

	binary.BigEndian.PutUint16(wc.Data[wc.CurPos:], v)
}
