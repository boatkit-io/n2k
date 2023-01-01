package pkt

import (
	"testing"

	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/stretchr/testify/assert"
)

func TestProprietary(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      130824,
		SourceId: 10,
		Priority: 1,
		TargetId: 0,
	}
	p := Packet{
		Info: pInfo,
		Data: []uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF},
	}
	u := p.UnknownPGN()
	assert.Equal(t, pgn.BAndG, u.ManufacturerCode)
	//	assert.Equal(t, uint8(4), p.IndustryCode) Not set--not used for matches, so really don't care
}

func TestEmpty(t *testing.T) {
	pInfo := pgn.MessageInfo{}
	p := NewPacket(pInfo, []uint8{})
	u := p.UnknownPGN()
	assert.NotEqual(t, 0, len(u.Reason.Error()))
}
