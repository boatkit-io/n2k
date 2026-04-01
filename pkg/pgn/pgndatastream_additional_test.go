package pgn

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadFixedString_Normal(t *testing.T) {
	data := []uint8{'H', 'e', 'l', 'l', 'o'}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(40)
	assert.NoError(t, err)
	assert.Equal(t, "Hello", str)
}

func TestReadFixedString_PaddedWithNull(t *testing.T) {
	data := []uint8{'H', 'i', 0x00, 0x00, 0x00}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(40)
	assert.NoError(t, err)
	assert.Equal(t, "Hi", str)
}

func TestReadFixedString_PaddedWithFF(t *testing.T) {
	data := []uint8{'H', 'i', 0xFF, 0xFF, 0xFF}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(40)
	assert.NoError(t, err)
	assert.Equal(t, "Hi", str)
}

func TestReadFixedString_PaddedWithAt(t *testing.T) {
	data := []uint8{'H', 'i', '@', '@', '@'}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(40)
	assert.NoError(t, err)
	assert.Equal(t, "Hi", str)
}

func TestReadFixedString_Empty(t *testing.T) {
	data := []uint8{0x00, 0x00, 0x00}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(24)
	assert.NoError(t, err)
	assert.Equal(t, "", str)
}

func TestReadStringWithLength_Normal(t *testing.T) {
	// Length byte = 5, then "Hello"
	data := []uint8{5, 'H', 'e', 'l', 'l', 'o'}
	s := NewPgnDataStream(data)
	str, err := s.readStringWithLength()
	assert.NoError(t, err)
	assert.Equal(t, "Hello", str)
}

func TestReadStringWithLength_NullLength(t *testing.T) {
	// 0xFF is null for uint8
	data := []uint8{0xFF}
	s := NewPgnDataStream(data)
	_, err := s.readStringWithLength()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "null length")
}

func TestReadSignedResolution_Positive(t *testing.T) {
	// Value 100 as int16 (little endian): 0x64, 0x00
	// With scaling factor 0.1, result = 100 * 0.1 = 10.0
	data := []uint8{0x64, 0x00}
	s := NewPgnDataStream(data)
	v, err := s.readSignedResolution(16, 0.1)
	assert.NoError(t, err)
	assert.NotNil(t, v)
	assert.InDelta(t, float32(10.0), *v, 0.01)
}

func TestReadSignedResolution_Negative(t *testing.T) {
	// Value -100 as int16 (little endian): 0x9C, 0xFF
	// -100 in 16 bits: 0xFF9C
	// With scaling factor 0.01, result = -100 * 0.01 = -1.0
	data := []uint8{0x9C, 0xFF}
	s := NewPgnDataStream(data)
	v, err := s.readSignedResolution(16, 0.01)
	assert.NoError(t, err)
	assert.NotNil(t, v)
	assert.InDelta(t, float32(-1.0), *v, 0.01)
}

func TestReadSignedResolution_Null(t *testing.T) {
	// Null for signed 16-bit = 0x7FFF
	data := []uint8{0xFF, 0x7F}
	s := NewPgnDataStream(data)
	v, err := s.readSignedResolution(16, 0.1)
	assert.NoError(t, err)
	assert.Nil(t, v)
}

func TestReadUnsignedResolution_Normal(t *testing.T) {
	// Value 200 as uint16 (little endian): 0xC8, 0x00
	// With scaling factor 0.01, result = 200 * 0.01 = 2.0
	data := []uint8{0xC8, 0x00}
	s := NewPgnDataStream(data)
	v, err := s.readUnsignedResolution(16, 0.01)
	assert.NoError(t, err)
	assert.NotNil(t, v)
	assert.InDelta(t, float32(2.0), *v, 0.01)
}

func TestReadUnsignedResolution_Null(t *testing.T) {
	// Null for unsigned 16-bit = 0xFFFF
	data := []uint8{0xFF, 0xFF}
	s := NewPgnDataStream(data)
	v, err := s.readUnsignedResolution(16, 0.01)
	assert.NoError(t, err)
	assert.Nil(t, v)
}

func TestReadFloat32_Normal(t *testing.T) {
	// Encode float32 1.5 as IEEE 754
	bits := math.Float32bits(1.5)
	data := []uint8{
		uint8(bits),
		uint8(bits >> 8),
		uint8(bits >> 16),
		uint8(bits >> 24),
	}
	s := NewPgnDataStream(data)
	v, err := s.readFloat32()
	assert.NoError(t, err)
	assert.NotNil(t, v)
	assert.Equal(t, float32(1.5), *v)
}

func TestReadFloat32_Null(t *testing.T) {
	// All 0xFF = null for uint32 -> nil
	data := []uint8{0xFF, 0xFF, 0xFF, 0xFF}
	s := NewPgnDataStream(data)
	v, err := s.readFloat32()
	assert.NoError(t, err)
	assert.Nil(t, v)
}

func TestReadLookupField_Normal(t *testing.T) {
	data := []uint8{0x05}
	s := NewPgnDataStream(data)
	v, err := s.readLookupField(8)
	assert.NoError(t, err)
	assert.Equal(t, uint64(5), v)
}

func TestReadLookupField_SubByte(t *testing.T) {
	// 3 bits from 0xFE = 0b110 = 6
	data := []uint8{0xFE}
	s := NewPgnDataStream(data)
	v, err := s.readLookupField(3)
	assert.NoError(t, err)
	assert.Equal(t, uint64(6), v)
}

func TestReadLookupField_TooManyBits(t *testing.T) {
	data := []uint8{0xFF}
	s := NewPgnDataStream(data)
	_, err := s.readLookupField(65)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "65")
}

func TestGetBitOffset_AfterReads(t *testing.T) {
	data := []uint8{0x01, 0x02, 0x03, 0x04}
	s := NewPgnDataStream(data)

	assert.Equal(t, uint32(0), s.getBitOffset())

	_, _ = s.readUInt8(8)
	assert.Equal(t, uint32(8), s.getBitOffset())

	_, _ = s.readUInt16(16)
	assert.Equal(t, uint32(24), s.getBitOffset())
}

func TestGetBitOffset_AfterSubByteRead(t *testing.T) {
	data := []uint8{0xFF, 0xFF}
	s := NewPgnDataStream(data)

	_, _ = s.readLookupField(3)
	assert.Equal(t, uint32(3), s.getBitOffset())

	_, _ = s.readLookupField(5)
	assert.Equal(t, uint32(8), s.getBitOffset())
}

func TestIsEOF_AtStart(t *testing.T) {
	data := []uint8{0x01, 0x02}
	s := NewPgnDataStream(data)
	assert.False(t, s.isEOF())
}

func TestIsEOF_AfterReadingAll(t *testing.T) {
	data := []uint8{0x01, 0x02}
	s := NewPgnDataStream(data)
	_, _ = s.readUInt16(16)
	assert.True(t, s.isEOF())
}

func TestIsEOF_EmptyStream(t *testing.T) {
	data := []uint8{}
	s := NewPgnDataStream(data)
	assert.True(t, s.isEOF())
}

func TestIsEOF_PartialRead(t *testing.T) {
	data := []uint8{0x01, 0x02}
	s := NewPgnDataStream(data)
	_, _ = s.readUInt8(8)
	assert.False(t, s.isEOF())
}

func TestSkipBits_Normal(t *testing.T) {
	data := []uint8{0x01, 0x02, 0x03, 0x04}
	s := NewPgnDataStream(data)
	err := s.skipBits(8)
	assert.NoError(t, err)
	assert.Equal(t, uint32(8), s.getBitOffset())
}

func TestSkipBits_SubByte(t *testing.T) {
	data := []uint8{0xFF, 0xFF}
	s := NewPgnDataStream(data)
	err := s.skipBits(3)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3), s.getBitOffset())
}

func TestSkipBits_PastEnd(t *testing.T) {
	data := []uint8{0x01}
	s := NewPgnDataStream(data)
	err := s.skipBits(16)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "off end")
}

func TestSkipBits_MultipleByteCrossing(t *testing.T) {
	data := []uint8{0x01, 0x02, 0x03, 0x04}
	s := NewPgnDataStream(data)
	err := s.skipBits(5)
	assert.NoError(t, err)
	assert.Equal(t, uint32(5), s.getBitOffset())
	err = s.skipBits(11)
	assert.NoError(t, err)
	assert.Equal(t, uint32(16), s.getBitOffset())
}

func TestReadSequentialFields(t *testing.T) {
	// Simulate reading multiple fields sequentially, like a decoder does
	// SID (8 bits) = 0x01, Heading (16 bits) = 0x1234
	data := []uint8{0x01, 0x34, 0x12, 0xFF}
	s := NewPgnDataStream(data)

	sid, err := s.readUInt8(8)
	assert.NoError(t, err)
	assert.NotNil(t, sid)
	assert.Equal(t, uint8(1), *sid)

	heading, err := s.readUnsignedResolution(16, 0.0001)
	assert.NoError(t, err)
	assert.NotNil(t, heading)
	assert.InDelta(t, float32(0x1234)*0.0001, *heading, 0.0001)
}

func TestReadBinaryData_ByteAligned(t *testing.T) {
	data := []uint8{0xAA, 0xBB, 0xCC}
	s := NewPgnDataStream(data)
	v, err := s.readBinaryData(24)
	assert.NoError(t, err)
	assert.Equal(t, []uint8{0xAA, 0xBB, 0xCC}, v)
}

func TestReadBinaryData_SubByte(t *testing.T) {
	data := []uint8{0xFF}
	s := NewPgnDataStream(data)
	v, err := s.readBinaryData(4)
	assert.NoError(t, err)
	assert.Equal(t, []uint8{0x0F}, v)
}
