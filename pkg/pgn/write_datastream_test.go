package pgn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteNumerics(t *testing.T) {
	// test a variety of uint64 basics
	uintTests := []struct {
		exp    []uint8
		value  uint64
		length uint16
	}{
		// On byte boundary
		{[]uint8{0x12}, 0x12, 8},
		{[]uint8{0x12, 0x34, 0x12}, 0x1234, 16},
		{[]uint8{0x12, 0x34, 0x12, 0x24}, 0x24, 8},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12}, 0x1234, 16},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff}, 0xffffeed4, 32},

		// On byte boundary, sub-byte
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0x1E}, 0x1E, 5},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0xFE}, 7, 3},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0xFE, 0x02}, 2, 2},

		// Off byte boundary
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0xFE, 0x16}, 5, 3},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0xFE, 0xD6, 0x7}, 0x3E, 6},
		/*				{[]uint8{0, 0x10, 0x02, 0}, 0x21, 0, 12, 8},
						{[]uint8{1, 2, 0x3}, 0xC080, 0, 2, 16},
		*/
	}

	data := make([]uint8, 223, 223) // 0-filled, len=223
	p := NewDataStream(data)
	for _, tst := range uintTests {
		err := p.putNumberRaw(tst.value, tst.length)
		assert.NoError(t, err)
		for i := range tst.exp {
			assert.Equal(t, tst.exp[i], data[i])
		}
	}
	readTests := []struct {
		exp    uint64
		length uint16
	}{
		{0x12, 8},
		{0x1234, 16},
		{0x24, 8},
		{0x1234, 16},
		{0xffffeed4, 32},
	}
	p.resetToStart()
	for _, tst := range readTests {
		v, err := p.getNumberRaw(tst.length)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, v)
	}
}

/*
	// other uints
	vuint2, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt32(32)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0xFFFFEED4), *vuint2)
	vuint3, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt16(16)
	assert.NoError(t, err)
	assert.Equal(t, uint16(0xEED4), *vuint3)
	vuint4, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt8(8)
	assert.NoError(t, err)
	assert.Equal(t, uint8(0xD4), *vuint4)

	vuintn1, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(32)
	assert.NoError(t, err)
	assert.Nil(t, vuintn1)
	vuintn2, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(16)
	assert.NoError(t, err)
	assert.Nil(t, vuintn2)
	vuintn3, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(8)
	assert.NoError(t, err)
	assert.Nil(t, vuintn3)
	vuintn4, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(4)
	assert.NoError(t, err)
	assert.Nil(t, vuintn4)

	// signed cases
	vint, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readInt64(32)
	assert.NoError(t, err)
	assert.Equal(t, int64(-4396), *vint)
	vint2, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readInt32(32)
	assert.NoError(t, err)
	assert.Equal(t, int32(-4396), *vint2)
	vint3, err := NewDataStream([]uint8{0xd4, 0xee}).readInt16(16)
	assert.NoError(t, err)
	assert.Equal(t, int16(-4396), *vint3)
	vint4, err := NewDataStream([]uint8{0xd4}).readInt8(8)
	assert.NoError(t, err)
	assert.Equal(t, int8(-44), *vint4)

	vintn1, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0x7f}).readInt32(32)
	assert.NoError(t, err)
	assert.Nil(t, vintn1)
	vintn2, err := NewDataStream([]uint8{0xff, 0x7f}).readInt32(16)
	assert.NoError(t, err)
	assert.Nil(t, vintn2)
	vintn3, err := NewDataStream([]uint8{0x7f}).readInt32(8)
	assert.NoError(t, err)
	assert.Nil(t, vintn3)
	vintn4, err := NewDataStream([]uint8{0x7}).readInt32(4)
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
		p := NewDataStream(tst.data)
		if tst.offset > 0 {
			_, _ = p.readUInt64(uint16(tst.offset))
		}
		v, err := p.readBinaryData(tst.length)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, v)
	}
}

// TODO: Tests for strings once we get more confidence

*/
