package pgn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOffset verifies that the bit-level cursor tracking works correctly across
// sub-byte skips, byte-boundary crossings, and multi-byte skips. This is fundamental
// to all stream operations since NMEA 2000 fields are often not byte-aligned.
func TestOffset(t *testing.T) {
	s := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0x7f})
	assert.Equal(t, uint32(0), s.getBitOffset())
	// Skip 7 bits (sub-byte, stays within first byte).
	err := s.skipBits(7)
	assert.NoError(t, err)
	assert.Equal(t, uint32(7), s.getBitOffset())
	// Skip 2 more bits -- crosses a byte boundary (bit 7 -> bit 9).
	err = s.skipBits(2)
	assert.NoError(t, err)
	assert.Equal(t, uint32(9), s.getBitOffset())
	// Skip 16 bits -- multi-byte skip from a non-aligned position.
	err = s.skipBits(16)
	assert.NoError(t, err)
	assert.Equal(t, uint32(25), s.getBitOffset())
}

// TestNumerics is a comprehensive test of the core numeric reading functions.
// It covers:
//   - Unsigned reads at various bit widths (8, 16, 32) on byte boundaries
//   - Sub-byte reads (fewer than 8 bits) on byte boundaries
//   - Reads starting at non-byte-aligned offsets (testing the bit-extraction logic)
//   - Narrower unsigned types (uint32, uint16, uint8) to verify correct truncation
//   - Null detection for unsigned fields (all-ones pattern returns nil)
//   - Signed two's-complement reads for negative values at various widths
//   - Null detection for signed fields (positive max value returns nil)
//   - Binary data reads at byte-aligned and sub-byte offsets
func TestNumerics(t *testing.T) {
	// Table-driven tests for uint64 reads at various offsets and bit widths.
	uintTests := []struct {
		exp    uint64
		data   []uint8
		offset uint16
		length uint16
	}{
		// --- Byte-aligned reads ---
		{0x12, []uint8{0x12}, 0, 8},                           // Single byte
		{0x1234, []uint8{0x34, 0x12}, 0, 16},                  // 16-bit little-endian
		{0x1234, []uint8{0, 0x34, 0x12, 0}, 8, 16},            // 16-bit after skipping one byte
		{0xffffeed4, []uint8{0xd4, 0xee, 0xff, 0xff}, 0, 32},  // Full 32-bit value

		// --- Sub-byte reads on byte boundary ---
		{0x1E, []uint8{0xFE}, 0, 5},  // Low 5 bits of 0xFE = 11110 = 0x1E
		{2, []uint8{0xFE}, 0, 2},     // Low 2 bits of 0xFE = 10 = 2

		// --- Non-byte-aligned reads (testing bit extraction across byte boundaries) ---
		{2, []uint8{0x14}, 1, 3},                             // 3 bits starting at bit 1 of 0x14
		{0x3E, []uint8{0xFB}, 2, 6},                          // 6 bits starting at bit 2 of 0xFB
		{0x21, []uint8{0, 0x1F, 0xF2, 0}, 12, 8},             // 8 bits straddling bytes 1-2
		{0xC080, []uint8{1, 2, 0x3}, 2, 16},                  // 16 bits starting at bit 2
	}

	for _, tst := range uintTests {
		p := NewPgnDataStream(tst.data)
		if tst.offset > 0 {
			_ = p.skipBits(uint16(tst.offset))
		}
		v, err := p.readUInt64(tst.length)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, *v)
	}

	// Verify narrower unsigned types read and truncate correctly.
	vuint2, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt32(32)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0xFFFFEED4), *vuint2)
	vuint3, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt16(16)
	assert.NoError(t, err)
	assert.Equal(t, uint16(0xEED4), *vuint3)
	vuint4, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt8(8)
	assert.NoError(t, err)
	assert.Equal(t, uint8(0xD4), *vuint4)

	// Verify null detection for unsigned fields: all-ones at various bit widths should return nil.
	vuintn1, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(32)
	assert.NoError(t, err)
	assert.Nil(t, vuintn1)
	vuintn2, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(16)
	assert.NoError(t, err)
	assert.Nil(t, vuintn2)
	vuintn3, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(8)
	assert.NoError(t, err)
	assert.Nil(t, vuintn3)
	vuintn4, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(4)
	assert.NoError(t, err)
	assert.Nil(t, vuintn4)

	// Verify signed two's-complement decoding for negative values at various widths.
	// 0xFFFFEED4 as signed 32-bit = -4396.
	vint, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readInt64(32)
	assert.NoError(t, err)
	assert.Equal(t, int64(-4396), *vint)
	vint2, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readInt32(32)
	assert.NoError(t, err)
	assert.Equal(t, int32(-4396), *vint2)
	// 0xEED4 as signed 16-bit = -4396.
	vint3, err := NewPgnDataStream([]uint8{0xd4, 0xee}).readInt16(16)
	assert.NoError(t, err)
	assert.Equal(t, int16(-4396), *vint3)
	// 0xD4 as signed 8-bit = -44.
	vint4, err := NewPgnDataStream([]uint8{0xd4}).readInt8(8)
	assert.NoError(t, err)
	assert.Equal(t, int8(-44), *vint4)

	// Verify null detection for signed fields: positive max value (0x7F...) should return nil.
	vintn1, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0x7f}).readInt32(32)
	assert.NoError(t, err)
	assert.Nil(t, vintn1)
	vintn2, err := NewPgnDataStream([]uint8{0xff, 0x7f}).readInt32(16)
	assert.NoError(t, err)
	assert.Nil(t, vintn2)
	vintn3, err := NewPgnDataStream([]uint8{0x7f}).readInt32(8)
	assert.NoError(t, err)
	assert.Nil(t, vintn3)
	vintn4, err := NewPgnDataStream([]uint8{0x7}).readInt32(4)
	assert.NoError(t, err)
	assert.Nil(t, vintn4)

	// Table-driven tests for readBinaryData at various offsets and lengths.
	bdTests := []struct {
		exp    []uint8
		data   []uint8
		offset uint16
		length uint16
	}{
		{[]uint8{1, 2, 3}, []uint8{1, 2, 3}, 0, 24},                // Byte-aligned, 3 bytes
		{[]uint8{0x1E}, []uint8{0xFE}, 0, 5},                        // Sub-byte: low 5 bits of 0xFE
		{[]uint8{0x21}, []uint8{0, 0x1F, 0xF2, 0}, 12, 8},           // Non-aligned: 8 bits starting at bit 12
	}

	for _, tst := range bdTests {
		p := NewPgnDataStream(tst.data)
		if tst.offset > 0 {
			_, _ = p.readUInt64(uint16(tst.offset))
		}
		v, err := p.readBinaryData(tst.length)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, v)
	}
}

// TODO: Tests for strings once we get more confidence
