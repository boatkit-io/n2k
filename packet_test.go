package n2k

import (
	"testing"

	"github.com/brutella/can"
	"github.com/stretchr/testify/assert"
)

func TestNewInfo(t *testing.T) {
	p := &Packet{}
	p.Info = newPacketInfo(can.Frame{ID: canIdFromData(130824, 7, 1), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	assert.Equal(t, uint32(130824), p.Info.PGN)
	assert.Equal(t, uint8(7), p.Info.SourceId)
	assert.Equal(t, uint8(1), p.Info.Priority)
	assert.Equal(t, 0, len(p.ParseErrors))
}

func TestValid(t *testing.T) {
	p := &Packet{}
	p.valid()
	assert.Equal(t, len(p.ParseErrors), 2)
}

func TestGetManCode(t *testing.T) {
	s := NewPgnDataStream([]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF})
	m, err := getManCode(s)
	assert.Equal(t, ManufacturerCodeConst(381), m)
	assert.Equal(t, nil, err)
}

func TestGetSeqFrame(t *testing.T) {
	p := &Packet{
		Data: []uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F},
	}
	p.getSeqFrame()
	assert.Equal(t, uint8(2), p.FrameNum)
	assert.Equal(t, uint8(3), p.SeqId)
}

func TestNewPacket(t *testing.T) {
	p := NewPacket(can.Frame{ID: canIdFromData(130824, 7, 1), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	assert.Equal(t, uint32(130824), p.Info.PGN)
	assert.Equal(t, uint8(7), p.Info.SourceId)
	assert.Equal(t, uint8(1), p.Info.Priority)
	assert.Equal(t, 0, len(p.ParseErrors))
	assert.Equal(t, 2, len(p.Candidates))
}

func TestFilterSlow(t *testing.T) {
	p := NewPacket(can.Frame{ID: canIdFromData(130824, 7, 1), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	p.FilterOnManufacturer()
	assert.Equal(t, 0, len(p.ParseErrors))
	assert.Equal(t, 1, len(p.Decoders))

	p = NewPacket(can.Frame{ID: canIdFromData(130824, 10, 1), Length: 8, Data: [8]uint8{(380 & 0xFF), (380 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	p.FilterOnManufacturer()
	assert.Equal(t, 1, len(p.ParseErrors))
	assert.Equal(t, 0, len(p.Decoders))
}

func TestFilterFast(t *testing.T) {
	p := NewPacket(can.Frame{ID: canIdFromData(130820, 10, 1), Length: 8, Data: [8]uint8{160, 5, 163, 153, 32, 128, 1, 255}})
	p.FilterOnManufacturer()
	assert.Equal(t, 0, len(p.ParseErrors))
	assert.Equal(t, 23, len(p.Decoders))
}

/*func TestSlowFramePass(t *testing.T) {
	f := newFrame(can.Frame{ID: canIdFromData(130824, 7, 1), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	assert.Equal(t, uint32(130824), f.Info.PGN)
	assert.Equal(t, uint8(7), f.Info.SourceId)
	assert.Equal(t, uint8(1), f.Info.Priority)
	assert.Equal(t, 0, len(f.ParseErrors))
	r := decodeFrame(f)
	assert.IsType(t, BGWindData{}, r)
	assert.Equal(t, false, f.Fast)
}

func TestFastFrame(t *testing.T) {
	f := newFrame(can.Frame{ID: canIdFromData(130824, 7, 1), Length: 8, Data: [8]uint8{160, 9, (137 & 0xFF), (137 >> 8) | (4 << 5), 1, 2, 3, 4}})
	assert.Equal(t, uint32(130824), f.Info.PGN)
	assert.Equal(t, uint8(7), f.Info.SourceId)
	assert.Equal(t, uint8(1), f.Info.Priority)
	//	assert.Equal(t, 1, len(f.ParseErrors)) change in parsing matches this fast packet without error
	assert.Equal(t, true, f.Fast)
}

func TestBroadcast(t *testing.T) {
	f := newFrame(can.Frame{ID: canIdFromData(0x1efff, 7, 1), Length: 8, Data: [8]uint8{160, 9, (137 & 0xFF), (137 >> 8) | (4 << 5), 1, 2, 3, 4}})
	assert.Equal(t, uint32(0x1ef00), f.Info.PGN)
	assert.Equal(t, uint8(255), f.Info.TargetId)
}
*/
