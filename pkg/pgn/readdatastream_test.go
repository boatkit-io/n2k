package pgn

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOffset(t *testing.T) {
	s := NewDataStream([]uint8{0xff, 0xff, 0xff, 0x7f})
	assert.Equal(t, uint32(0), s.getBitOffset())
	err := s.skipBits(7)
	assert.NoError(t, err)
	assert.Equal(t, uint32(7), s.getBitOffset())
	err = s.skipBits(2)
	assert.NoError(t, err)
	assert.Equal(t, uint32(9), s.getBitOffset())
	err = s.skipBits(16)
	assert.NoError(t, err)
	assert.Equal(t, uint32(25), s.getBitOffset())
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
		{0x1D, []uint8{0xFD}, 0, 5},
		{2, []uint8{0xFE}, 0, 2},

		// Off byte boundary
		{2, []uint8{0x14}, 1, 3},
		{0x3D, []uint8{0xF7}, 2, 6},
		{0x21, []uint8{0, 0x1F, 0xF2, 0}, 12, 8},
		{0xC080, []uint8{1, 2, 0x3}, 2, 16},
	}

	for _, tst := range uintTests {
		p := NewDataStream(tst.data)
		if tst.offset > 0 {
			_ = p.skipBits(uint16(tst.offset))
		}
		cnt := 2
		if tst.length < 4 {
			cnt = 1
		}
		v, err := p.readUInt64(tst.length, cnt)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, *v)
	}

	// other uints
	vuint2, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt32(32, 2)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0xFFFFEED4), *vuint2)
	vuint3, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt16(16, 2)
	assert.NoError(t, err)
	assert.Equal(t, uint16(0xEED4), *vuint3)
	vuint4, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readUInt8(8, 2)
	assert.NoError(t, err)
	assert.Equal(t, uint8(0xD4), *vuint4)

	vuintn1, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(32, 2)
	assert.NoError(t, err)
	assert.Nil(t, vuintn1)
	vuintn2, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(16, 2)
	assert.NoError(t, err)
	assert.Nil(t, vuintn2)
	vuintn3, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(8, 2)
	assert.NoError(t, err)
	assert.Nil(t, vuintn3)
	vuintn4, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0xff}).readUInt32(4, 2)
	assert.NoError(t, err)
	assert.Nil(t, vuintn4)

	// signed cases
	vint, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readInt64(32, 2)
	assert.NoError(t, err)
	assert.Equal(t, int64(-4396), *vint)
	vint2, err := NewDataStream([]uint8{0xd4, 0xee, 0xff, 0xff}).readInt32(32, 2)
	assert.NoError(t, err)
	assert.Equal(t, int32(-4396), *vint2)
	vint3, err := NewDataStream([]uint8{0xd4, 0xee}).readInt16(16, 2)
	assert.NoError(t, err)
	assert.Equal(t, int16(-4396), *vint3)
	vint4, err := NewDataStream([]uint8{0xd4}).readInt8(8, 2)
	assert.NoError(t, err)
	assert.Equal(t, int8(-44), *vint4)

	vintn1, err := NewDataStream([]uint8{0xff, 0xff, 0xff, 0x7f}).readInt32(32, 2)
	assert.NoError(t, err)
	assert.Nil(t, vintn1)
	vintn2, err := NewDataStream([]uint8{0xff, 0x7f}).readInt32(16, 2)
	assert.NoError(t, err)
	assert.Nil(t, vintn2)
	vintn3, err := NewDataStream([]uint8{0x7f}).readInt32(8, 2)
	assert.NoError(t, err)
	assert.Nil(t, vintn3)
	vintn4, err := NewDataStream([]uint8{0x7}).readInt32(4, 2)
	assert.NoError(t, err)
	assert.Nil(t, vintn4)

	// binary data
	bdTests := []struct {
		exp         []uint8
		data        []uint8
		offset      uint16
		length      uint16
		errExpected bool
	}{
		{[]uint8{1, 2, 3}, []uint8{1, 2, 3}, 0, 24, false},
		{[]uint8{0x1E}, []uint8{0xFE}, 0, 5, false},
		{[]uint8{0x21}, []uint8{0, 0x1F, 0xF2, 0}, 12, 8, true},
	}

	for _, tst := range bdTests {
		p := NewDataStream(tst.data)
		if tst.offset > 0 {
			_, _ = p.readUInt64(uint16(tst.offset), 2)
		}
		v, err := p.readBinaryData(tst.length)
		if tst.errExpected {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tst.exp, v)
		}
	}
}

// TODO: Tests for strings once we get more confidence
func TestDataStream_readStringWithLengthAndControl(t *testing.T) {
	tests := []struct {
		name          string
		data          []uint8
		expectedStr   string
		expectedError error
	}{
		{
			name: "UTF-16 Basic string",
			// Length 3, control 0 (UTF-16), "ABC" in UTF-16
			data:        []uint8{0x09, 0x00, 0x00, 0x41, 0x00, 0x42, 0x00, 0x43, 0x00},
			expectedStr: "ABC",
		},
		{
			name: "ASCII/UTF-8 Basic string",
			// Length 3, control 1 (ASCII), "ABC"
			data:        []uint8{0x06, 0x01, 0x41, 0x42, 0x43, 0x00},
			expectedStr: "ABC",
		},
		{
			name: "Empty UTF-16 string",
			// Length 0, control 0 (UTF-16)
			data:        []uint8{0x00, 0x00},
			expectedStr: "",
		},
		{
			name: "Empty ASCII string",
			// Length 0, control 1 (ASCII)
			data:        []uint8{0x00, 0x01},
			expectedStr: "",
		},
		{
			name: "UTF-16 string with special characters",
			// Length 2, control 0 (UTF-16), "你好" in UTF-16
			data:        []uint8{0x07, 0x00, 0x4f, 0x60, 0x59, 0x7d, 0x00},
			expectedStr: "你好",
		},
		{
			name:          "Invalid length for UTF-16",
			data:          []uint8{0xFF, 0x00}, // Length too long for available data
			expectedStr:   "",
			expectedError: errors.New("invalid length"),
		},
		{
			name:          "Invalid length for ASCII",
			data:          []uint8{0xFF, 0x01}, // Length too long for available data
			expectedStr:   "",
			expectedError: errors.New("invalid length"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := NewDataStream(tt.data)
			str, err := stream.readStringWithLengthAndControl()

			if tt.expectedError != nil {
				assert.Error(t, err)
				//				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStr, str)
			}
		})
	}
}
