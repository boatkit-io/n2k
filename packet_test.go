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
	p := NewPacket(can.Frame{ID: canIdFromData(130824, 7, 1), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	p.getManCode()
	assert.Equal(t, ManufacturerCodeConst(381), p.Manufacturer)
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
	assert.False(t, p.Fast)
	p.getManCode()
	assert.Equal(t, ManufacturerCodeConst(381), p.Manufacturer)
}

func TestFilterSlow(t *testing.T) {
	p := NewPacket(can.Frame{ID: canIdFromData(130824, 7, 1), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	p.addDecoders()
	assert.Equal(t, 0, len(p.ParseErrors))
	assert.Equal(t, 1, len(p.Decoders))

	p = NewPacket(can.Frame{ID: canIdFromData(130824, 10, 1), Length: 8, Data: [8]uint8{(380 & 0xFF), (380 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	p.addDecoders()
	// assert.Equal(t, 1, len(p.ParseErrors)) p.decode() now handles the no decoders error
	assert.Equal(t, 0, len(p.Decoders))
}

func TestFilterFast(t *testing.T) {
	p := NewPacket(can.Frame{ID: canIdFromData(130820, 10, 1), Length: 8, Data: [8]uint8{160, 5, (419 & 0xFF), (419 >> 8) | (4 << 5), 32, 128, 1, 255}})
	p.Data = p.Data[2:] // normally happens in sequence.complete()
	p.Complete = true   // normally these 2 calls would happen by invoking b.process()
	p.getManCode()
	p.addDecoders()
	assert.Equal(t, 0, len(p.ParseErrors))
	assert.Equal(t, 23, len(p.Decoders))
}

func TestBroadcast(t *testing.T) {
	p := NewPacket(can.Frame{ID: canIdFromData(0x1efff, 7, 1), Length: 8, Data: [8]uint8{160, 9, (137 & 0xFF), (137 >> 8) | (4 << 5), 1, 2, 3, 4}})
	assert.Equal(t, uint32(0x1ef00), p.Info.PGN)
	assert.Equal(t, uint8(255), p.Info.TargetId)
}
