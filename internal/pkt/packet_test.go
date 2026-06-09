package pkt

import (
	"testing"

	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/stretchr/testify/assert"
)

func TestValid(t *testing.T) {
	p := &Packet{}
	p.Valid()
	assert.Equal(t, len(p.ParseErrors), 2)
}

func TestGetSeqFrame(t *testing.T) {
	p := &Packet{
		Data: []uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F},
	}
	p.GetSeqFrame()
	assert.Equal(t, uint8(2), p.FrameNum)
	assert.Equal(t, uint8(3), p.SeqId)
}

func TestNewPacket(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      130824,
		SourceId: 7,
		Priority: 1,
		TargetId: 0,
	}
	p := NewPacket(pInfo, []uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF})
	assert.Equal(t, uint32(130824), p.Info.PGN)
	assert.Equal(t, uint8(7), p.Info.SourceId)
	assert.Equal(t, uint8(1), p.Info.Priority)
	assert.Equal(t, 0, len(p.ParseErrors))
	stream := pgn.NewDataStream(p.Data)
	_, err := pgn.FindDecoder(stream, p.Info.PGN)
	assert.Nil(t, err)
}

func TestFilterSlow(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      130824,
		SourceId: 7,
		Priority: 1,
		TargetId: 0,
	}
	p := NewPacket(pInfo, []uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF})
	stream := pgn.NewDataStream(p.Data)
	_, err := pgn.FindDecoder(stream, p.Info.PGN)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(p.ParseErrors))
	pInfo = pgn.MessageInfo{
		PGN:      130824,
		SourceId: 10,
		Priority: 1,
		TargetId: 0,
	}
	p = NewPacket(pInfo, []uint8{(380 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF})
	stream = pgn.NewDataStream(p.Data)
	_, err = pgn.FindDecoder(stream, p.Info.PGN)
	assert.NotNil(t, err)
}

func TestFilterFast(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      126720,
		SourceId: 10,
		Priority: 1,
		TargetId: 0,
	}
	p := NewPacket(pInfo, []uint8{160, 5, (135 & 0xFF), (13 >> 8) | (4 << 5), 32, 128, 1, 255})
	p.Data = p.Data[2:] // normally happens in sequence.complete()
	p.Complete = true   // normally these 2 calls would happen by invoking b.process()
	stream := pgn.NewDataStream(p.Data)
	_, err := pgn.FindDecoder(stream, p.Info.PGN)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(p.ParseErrors))
}

func TestBroadcast(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      0x1ef00,
		SourceId: 7,
		Priority: 1,
		TargetId: 255,
	}
	p := NewPacket(pInfo, []uint8{160, 9, (137 & 0xFF), (137 >> 8) | (4 << 5), 1, 2, 3, 4})
	assert.Equal(t, uint32(0x1ef00), p.Info.PGN)
	assert.Equal(t, uint8(255), p.Info.TargetId)
}
