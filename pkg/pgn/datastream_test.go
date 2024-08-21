package pgn

import (
	"testing"

	"github.com/boatkit-io/tugboat/pkg/units"
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
		v, err := p.readUInt64(tst.length)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, *v)
	}

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

func TestWritePgn(t *testing.T) {
	p := ManOverboardNotification{
		Info: MessageInfo{
			SourceId: 12,
			PGN:      129702,
		},
		Sid:                nil,
		MobEmitterId:       nil,
		ManOverboardStatus: MobStatusConst(1),
		ActivationTime:     nil,
		PositionSource:     MobPositionSourceConst(3),
		PositionDate:       nil,
		PositionTime:       nil,
		Latitude:           nil,
		Longitude:          nil,
		CogReference:       DirectionReferenceConst(2),
		Cog:                nil,
		Sog: &units.Velocity{
			Unit:  1,
			Value: 8,
		},
		MmsiOfVesselOfOrigin:       nil,
		MobEmitterBatteryLowStatus: LowBatteryConst(1),
	}
	stream := NewDataStream(make([]uint8, 223, 223))
	info, err := p.Encode(stream)
	var ok bool
	if _, ok = interface{}(&p).(PgnStruct); ok {
	}
	assert.True(t, ok)
	assert.Equal(t, info.PGN, uint32(129702))
	assert.Nil(t, err)
}
