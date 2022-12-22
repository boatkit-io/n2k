package n2k

import (
	"testing"

	"github.com/brutella/can"
	"github.com/stretchr/testify/assert"
)

func TestProprietary(t *testing.T) {
	p := newPacket(can.Frame{ID: canIdFromData(130824, 10, 1, 0), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	u := p.unknownPGN()
	assert.Equal(t, BAndG, u.ManufacturerCode)
	//	assert.Equal(t, uint8(4), p.IndustryCode) Not set--not used for matches, so really don't care
}

func TestEmpty(t *testing.T) {
	p := newPacket(can.Frame{ID: 0, Length: 0})
	u := p.unknownPGN()
	assert.NotEqual(t, 0, len(u.Reason.Error()))
}
