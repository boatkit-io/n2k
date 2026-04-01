package pkt

import (
	"testing"

	"github.com/open-ships/n2k/pkg/pgn"
	"github.com/stretchr/testify/assert"
)

// TestProprietary verifies that UnknownPGN correctly extracts the manufacturer code from
// a proprietary PGN's data payload. PGN 130824 is in the proprietary range, so when
// building an UnknownPGN, the manufacturer code should be extracted from the first two
// data bytes. The test uses manufacturer code 381 (B&G / Bandg) and confirms it appears
// in the resulting UnknownPGN's ManufacturerCode field.
func TestProprietary(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      130824,
		SourceId: 10,
		Priority: 1,
		TargetId: 0,
	}
	p := Packet{
		Info: pInfo,
		// Encode manufacturer 381 (B&G) and industry code 4 into bytes 0-1.
		Data: []uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF},
	}
	u := p.UnknownPGN()
	assert.Equal(t, pgn.BAndG, u.ManufacturerCode)
	//	assert.Equal(t, uint8(4), p.IndustryCode) Not set--not used for matches, so really don't care
}

// TestEmpty verifies that creating an UnknownPGN from an empty/invalid packet still
// produces a meaningful error reason. When NewPacket is called with zero-value MessageInfo
// and empty data, it accumulates parse errors (PGN=0, empty data). Those errors should
// be merged into the UnknownPGN's Reason field as a non-empty error string.
func TestEmpty(t *testing.T) {
	pInfo := pgn.MessageInfo{}
	p := NewPacket(pInfo, []uint8{})
	u := p.UnknownPGN()
	assert.NotEqual(t, 0, len(u.Reason.Error()))
}
