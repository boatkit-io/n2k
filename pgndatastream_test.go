package n2k

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOffset(t *testing.T) {
	s := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0x7f})
	assert.Equal(t, uint32(0), s.GetBitOffset())
	err := s.SkipBits(7)
	assert.NoError(t, err)
	assert.Equal(t, uint32(7), s.GetBitOffset())
	err = s.SkipBits(2)
	assert.NoError(t, err)
	assert.Equal(t, uint32(9), s.GetBitOffset())
	err = s.SkipBits(16)
	assert.NoError(t, err)
	assert.Equal(t, uint32(25), s.GetBitOffset())
}

func TestNumerics(t *testing.T) {
	// test a variety of uint64 basics
	uintTests := []struct {
		exp    uint64
		data   []uint8
		offset uint16
		length uint16
	}{
		// On byte boundary
		{0x12, []uint8{0x12}, 0, 8},
		{0x1234, []uint8{0x34, 0x12}, 0, 16},
		{0x1234, []uint8{0, 0x34, 0x12, 0}, 8, 16},
		{0xffffeed4, []uint8{0xd4, 0xee, 0xff, 0xff}, 0, 32},

		// On byte boundary, sub-byte
		{0x1E, []uint8{0xFE}, 0, 5},
		{2, []uint8{0xFE}, 0, 2},

		// Off byte boundary
		{2, []uint8{0x14}, 1, 3},
		{0x3E, []uint8{0xFB}, 2, 6},
		{0x21, []uint8{0, 0x1F, 0xF2, 0}, 12, 8},
		{0xC080, []uint8{1, 2, 0x3}, 2, 16},
	}

	for _, tst := range uintTests {
		p := NewPgnDataStream(tst.data)
		if tst.offset > 0 {
			_ = p.SkipBits(uint16(tst.offset))
		}
		v, err := p.ReadUInt64(tst.length)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, *v)
	}

	// other uints
	vuint2, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).ReadUInt32(32)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0xFFFFEED4), *vuint2)
	vuint3, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).ReadUInt16(16)
	assert.NoError(t, err)
	assert.Equal(t, uint16(0xEED4), *vuint3)
	vuint4, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).ReadUInt8(8)
	assert.NoError(t, err)
	assert.Equal(t, uint8(0xD4), *vuint4)

	vuintn1, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).ReadUInt32(32)
	assert.NoError(t, err)
	assert.Nil(t, vuintn1)
	vuintn2, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).ReadUInt32(16)
	assert.NoError(t, err)
	assert.Nil(t, vuintn2)
	vuintn3, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).ReadUInt32(8)
	assert.NoError(t, err)
	assert.Nil(t, vuintn3)
	vuintn4, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).ReadUInt32(4)
	assert.NoError(t, err)
	assert.Nil(t, vuintn4)

	// signed cases
	vint, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).ReadInt64(32)
	assert.NoError(t, err)
	assert.Equal(t, int64(-4396), *vint)
	vint2, err := NewPgnDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).ReadInt32(32)
	assert.NoError(t, err)
	assert.Equal(t, int32(-4396), *vint2)
	vint3, err := NewPgnDataStream([]uint8{0xd4, 0xee}).ReadInt16(16)
	assert.NoError(t, err)
	assert.Equal(t, int16(-4396), *vint3)
	vint4, err := NewPgnDataStream([]uint8{0xd4}).ReadInt8(8)
	assert.NoError(t, err)
	assert.Equal(t, int8(-44), *vint4)

	vintn1, err := NewPgnDataStream([]uint8{0xff, 0xff, 0xff, 0x7f}).ReadInt32(32)
	assert.NoError(t, err)
	assert.Nil(t, vintn1)
	vintn2, err := NewPgnDataStream([]uint8{0xff, 0x7f}).ReadInt32(16)
	assert.NoError(t, err)
	assert.Nil(t, vintn2)
	vintn3, err := NewPgnDataStream([]uint8{0x7f}).ReadInt32(8)
	assert.NoError(t, err)
	assert.Nil(t, vintn3)
	vintn4, err := NewPgnDataStream([]uint8{0x7}).ReadInt32(4)
	assert.NoError(t, err)
	assert.Nil(t, vintn4)

	// binary data
	bdTests := []struct {
		exp    []uint8
		data   []uint8
		offset uint16
		length uint16
	}{
		{[]uint8{1, 2, 3}, []uint8{1, 2, 3}, 0, 24},
		{[]uint8{0x1E}, []uint8{0xFE}, 0, 5},
		{[]uint8{0x21}, []uint8{0, 0x1F, 0xF2, 0}, 12, 8},
	}

	for _, tst := range bdTests {
		p := NewPgnDataStream(tst.data)
		if tst.offset > 0 {
			_, _ = p.ReadUInt64(uint16(tst.offset))
		}
		v, err := p.ReadBinaryData(tst.length)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, v)
	}
}

// TODO: Tests for strings once we get more confidence
