package pgn

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReadFixedString_Normal verifies that a fixed-length string with no padding
// is read correctly (all bytes are valid ASCII characters).
func TestReadFixedString_Normal(t *testing.T) {
	data := []uint8{'H', 'e', 'l', 'l', 'o'}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(40)
	assert.NoError(t, err)
	assert.Equal(t, "Hello", str)
}

// TestReadFixedString_PaddedWithNull verifies that NUL (0x00) padding bytes are
// stripped from the end of a fixed-length string, returning only the meaningful content.
func TestReadFixedString_PaddedWithNull(t *testing.T) {
	data := []uint8{'H', 'i', 0x00, 0x00, 0x00}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(40)
	assert.NoError(t, err)
	assert.Equal(t, "Hi", str)
}

// TestReadFixedString_PaddedWithFF verifies that 0xFF padding bytes (the NMEA 2000
// "no data" fill byte) are stripped from fixed-length strings.
func TestReadFixedString_PaddedWithFF(t *testing.T) {
	data := []uint8{'H', 'i', 0xFF, 0xFF, 0xFF}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(40)
	assert.NoError(t, err)
	assert.Equal(t, "Hi", str)
}

// TestReadFixedString_PaddedWithAt verifies that '@' padding characters (used by some
// legacy marine devices) are stripped from fixed-length strings.
func TestReadFixedString_PaddedWithAt(t *testing.T) {
	data := []uint8{'H', 'i', '@', '@', '@'}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(40)
	assert.NoError(t, err)
	assert.Equal(t, "Hi", str)
}

// TestReadFixedString_Empty verifies that a string consisting entirely of NUL padding
// returns an empty string (the padding is the first byte, so everything is stripped).
func TestReadFixedString_Empty(t *testing.T) {
	data := []uint8{0x00, 0x00, 0x00}
	s := NewPgnDataStream(data)
	str, err := s.readFixedString(24)
	assert.NoError(t, err)
	assert.Equal(t, "", str)
}

// TestReadStringWithLength_Normal verifies correct decoding of a STRING_LZ format string
// where the first byte gives the string length, followed by that many character bytes.
func TestReadStringWithLength_Normal(t *testing.T) {
	// Length byte = 5, then "Hello"
	data := []uint8{5, 'H', 'e', 'l', 'l', 'o'}
	s := NewPgnDataStream(data)
	str, err := s.readStringWithLength()
	assert.NoError(t, err)
	assert.Equal(t, "Hello", str)
}

// TestReadStringWithLength_NullLength verifies that a null length byte (0xFF, the
// unsigned 8-bit null sentinel) produces an error rather than attempting to read
// 255 * 8 bits of string data.
func TestReadStringWithLength_NullLength(t *testing.T) {
	data := []uint8{0xFF}
	s := NewPgnDataStream(data)
	_, err := s.readStringWithLength()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "null length")
}

// TestReadSignedResolution_Positive verifies that a positive signed integer is correctly
// read and scaled. Value 100 with resolution 0.1 should produce 10.0.
func TestReadSignedResolution_Positive(t *testing.T) {
	// Value 100 as int16 (little endian): 0x64, 0x00
	data := []uint8{0x64, 0x00}
	s := NewPgnDataStream(data)
	v, err := s.readSignedResolution(16, 0.1)
	assert.NoError(t, err)
	assert.NotNil(t, v)
	assert.InDelta(t, float32(10.0), *v, 0.01)
}

// TestReadSignedResolution_Negative verifies that a negative two's-complement value is
// correctly decoded and scaled. Value -100 with resolution 0.01 should produce -1.0.
func TestReadSignedResolution_Negative(t *testing.T) {
	// Value -100 as int16 (little endian): 0x9C, 0xFF  (0xFF9C = -100 in 16-bit two's complement)
	data := []uint8{0x9C, 0xFF}
	s := NewPgnDataStream(data)
	v, err := s.readSignedResolution(16, 0.01)
	assert.NoError(t, err)
	assert.NotNil(t, v)
	assert.InDelta(t, float32(-1.0), *v, 0.01)
}

// TestReadSignedResolution_Null verifies that the signed null sentinel (0x7FFF for 16-bit)
// returns nil instead of a value, correctly indicating "data not available".
func TestReadSignedResolution_Null(t *testing.T) {
	data := []uint8{0xFF, 0x7F}
	s := NewPgnDataStream(data)
	v, err := s.readSignedResolution(16, 0.1)
	assert.NoError(t, err)
	assert.Nil(t, v)
}

// TestReadUnsignedResolution_Normal verifies that an unsigned integer is correctly
// read and scaled. Value 200 with resolution 0.01 should produce 2.0.
func TestReadUnsignedResolution_Normal(t *testing.T) {
	// Value 200 as uint16 (little endian): 0xC8, 0x00
	data := []uint8{0xC8, 0x00}
	s := NewPgnDataStream(data)
	v, err := s.readUnsignedResolution(16, 0.01)
	assert.NoError(t, err)
	assert.NotNil(t, v)
	assert.InDelta(t, float32(2.0), *v, 0.01)
}

// TestReadUnsignedResolution_Null verifies that the unsigned null sentinel (0xFFFF for 16-bit)
// returns nil, correctly indicating "data not available".
func TestReadUnsignedResolution_Null(t *testing.T) {
	data := []uint8{0xFF, 0xFF}
	s := NewPgnDataStream(data)
	v, err := s.readUnsignedResolution(16, 0.01)
	assert.NoError(t, err)
	assert.Nil(t, v)
}

// TestReadFloat32_Normal verifies that an IEEE 754 float32 is correctly read from a
// little-endian byte sequence. The value 1.5 is encoded and then decoded.
func TestReadFloat32_Normal(t *testing.T) {
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

// TestReadFloat32_Null verifies that all-0xFF bytes (uint32 null sentinel) cause readFloat32
// to return nil, rather than returning the float interpretation of 0xFFFFFFFF (which would be NaN).
func TestReadFloat32_Null(t *testing.T) {
	data := []uint8{0xFF, 0xFF, 0xFF, 0xFF}
	s := NewPgnDataStream(data)
	v, err := s.readFloat32()
	assert.NoError(t, err)
	assert.Nil(t, v)
}

// TestReadLookupField_Normal verifies that a full-byte lookup field value is read correctly.
func TestReadLookupField_Normal(t *testing.T) {
	data := []uint8{0x05}
	s := NewPgnDataStream(data)
	v, err := s.readLookupField(8)
	assert.NoError(t, err)
	assert.Equal(t, uint64(5), v)
}

// TestReadLookupField_SubByte verifies sub-byte lookup field extraction.
// The low 3 bits of 0xFE (binary 11111110) are 110 = 6.
func TestReadLookupField_SubByte(t *testing.T) {
	data := []uint8{0xFE}
	s := NewPgnDataStream(data)
	v, err := s.readLookupField(3)
	assert.NoError(t, err)
	assert.Equal(t, uint64(6), v)
}

// TestReadLookupField_TooManyBits verifies that requesting more than 64 bits
// from readLookupField returns an error (since the result is a uint64).
func TestReadLookupField_TooManyBits(t *testing.T) {
	data := []uint8{0xFF}
	s := NewPgnDataStream(data)
	_, err := s.readLookupField(65)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "65")
}

// TestGetBitOffset_AfterReads verifies that the bit offset is correctly updated
// after sequential byte-aligned reads (8-bit + 16-bit = 24 bits total).
func TestGetBitOffset_AfterReads(t *testing.T) {
	data := []uint8{0x01, 0x02, 0x03, 0x04}
	s := NewPgnDataStream(data)

	assert.Equal(t, uint32(0), s.getBitOffset())

	_, _ = s.readUInt8(8)
	assert.Equal(t, uint32(8), s.getBitOffset())

	_, _ = s.readUInt16(16)
	assert.Equal(t, uint32(24), s.getBitOffset())
}

// TestGetBitOffset_AfterSubByteRead verifies cursor tracking after sub-byte reads.
// Two consecutive reads of 3 and 5 bits should advance to exactly bit 8 (one full byte).
func TestGetBitOffset_AfterSubByteRead(t *testing.T) {
	data := []uint8{0xFF, 0xFF}
	s := NewPgnDataStream(data)

	_, _ = s.readLookupField(3)
	assert.Equal(t, uint32(3), s.getBitOffset())

	_, _ = s.readLookupField(5)
	assert.Equal(t, uint32(8), s.getBitOffset())
}

// TestIsEOF_AtStart verifies that a non-empty stream is not at EOF when freshly created.
func TestIsEOF_AtStart(t *testing.T) {
	data := []uint8{0x01, 0x02}
	s := NewPgnDataStream(data)
	assert.False(t, s.isEOF())
}

// TestIsEOF_AfterReadingAll verifies that EOF is true after consuming all bytes.
func TestIsEOF_AfterReadingAll(t *testing.T) {
	data := []uint8{0x01, 0x02}
	s := NewPgnDataStream(data)
	_, _ = s.readUInt16(16)
	assert.True(t, s.isEOF())
}

// TestIsEOF_EmptyStream verifies that a zero-length stream is immediately at EOF.
func TestIsEOF_EmptyStream(t *testing.T) {
	data := []uint8{}
	s := NewPgnDataStream(data)
	assert.True(t, s.isEOF())
}

// TestIsEOF_PartialRead verifies that EOF is false when only some bytes have been consumed.
func TestIsEOF_PartialRead(t *testing.T) {
	data := []uint8{0x01, 0x02}
	s := NewPgnDataStream(data)
	_, _ = s.readUInt8(8)
	assert.False(t, s.isEOF())
}

// TestSkipBits_Normal verifies a simple byte-aligned skip of 8 bits.
func TestSkipBits_Normal(t *testing.T) {
	data := []uint8{0x01, 0x02, 0x03, 0x04}
	s := NewPgnDataStream(data)
	err := s.skipBits(8)
	assert.NoError(t, err)
	assert.Equal(t, uint32(8), s.getBitOffset())
}

// TestSkipBits_SubByte verifies skipping fewer than 8 bits (sub-byte skip).
func TestSkipBits_SubByte(t *testing.T) {
	data := []uint8{0xFF, 0xFF}
	s := NewPgnDataStream(data)
	err := s.skipBits(3)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3), s.getBitOffset())
}

// TestSkipBits_PastEnd verifies that skipping past the end of the data returns an error.
func TestSkipBits_PastEnd(t *testing.T) {
	data := []uint8{0x01}
	s := NewPgnDataStream(data)
	err := s.skipBits(16)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "off end")
}

// TestSkipBits_MultipleByteCrossing verifies that two consecutive sub-byte skips that
// together cross a byte boundary produce the correct cumulative offset.
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

// TestReadSequentialFields simulates how a real PGN decoder works: reading multiple
// fields in order from a single stream. First reads an 8-bit SID, then a 16-bit heading
// with resolution scaling, verifying that the cursor advances correctly between reads.
func TestReadSequentialFields(t *testing.T) {
	// Layout: SID (8 bits) = 0x01, Heading (16 bits) = 0x1234, padding = 0xFF
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

// TestReadBinaryData_ByteAligned verifies that readBinaryData returns an exact copy
// of the source bytes when reading on byte boundaries.
func TestReadBinaryData_ByteAligned(t *testing.T) {
	data := []uint8{0xAA, 0xBB, 0xCC}
	s := NewPgnDataStream(data)
	v, err := s.readBinaryData(24)
	assert.NoError(t, err)
	assert.Equal(t, []uint8{0xAA, 0xBB, 0xCC}, v)
}

// TestReadBinaryData_SubByte verifies that reading fewer than 8 bits returns a single
// byte with only the low bits populated (upper bits zeroed). Reading 4 bits from 0xFF
// should yield 0x0F.
func TestReadBinaryData_SubByte(t *testing.T) {
	data := []uint8{0xFF}
	s := NewPgnDataStream(data)
	v, err := s.readBinaryData(4)
	assert.NoError(t, err)
	assert.Equal(t, []uint8{0x0F}, v)
}
