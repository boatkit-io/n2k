package n2k

import (
	"testing"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func canIdFromData(pgn uint32, sourceId uint8, priority uint8) uint32 {
	return uint32(sourceId) | (pgn << 8) | (uint32(priority) << 26)
}

func TestInvalid(t *testing.T) {
	var recvPgn interface{}
	b := NewPGNBuilder(logrus.StandardLogger(), func(p interface{}) {
		recvPgn = p
	})
	assert.Nil(t, recvPgn)

	// Nothing for slow or fast packet in data dictionary that'll pass, and we have no way to know
	// which, since the manufacturer ID doesn't match, and 130824 is the only PGN with
	// both fast and slow variants. -- 380 should be 381 to pass
	// SKIPPING this test for now, since a fast pgn might still fit into a single frame. We let
	// subsequent processing decode this, successfully or not
	//	s.ProcessFrame(can.Frame{ID: canIdFromData(130824, 10, 1), Length: 8, Data: [8]uint8{(380 & 0xFF), (380 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	//	assert.Nil(t, recvPgn)

	// Now fix it and it should come out
	b.ProcessFrame(can.Frame{ID: canIdFromData(130824, 10, 1), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	assert.IsType(t, BGWindData{}, recvPgn)
	recvPgn = nil

	// Now do an invalid fast packet
	b.ProcessFrame(can.Frame{ID: canIdFromData(130824, 10, 1), Length: 8, Data: [8]uint8{160, 5, 1, 2, 3, 4, 5, 6}})
	assert.IsType(t, UnknownPGN{}, recvPgn)
	//	assert.Equal(t, 5, len(recvPgn.(UnknownPGN).Data)) We fail to match this before bothering to assemble the fast packet
	recvPgn = nil
}

func TestPossibleSlowFast(t *testing.T) {
	var recvPgn interface{}
	b := NewPGNBuilder(logrus.StandardLogger(), func(p interface{}) {
		recvPgn = p
	})
	assert.Nil(t, recvPgn)

	// 130820 is a PGN with both slow and fast packets.  This is a valid fast packet from it so it should pass.
	b.ProcessFrame(can.Frame{ID: canIdFromData(130820, 10, 1), Length: 8, Data: [8]uint8{160, 5, 163, 153, 32, 128, 1, 255}})
	assert.IsType(t, FusionPowerState{}, recvPgn)
	recvPgn = nil

	// Force a valid fast packet for 130824
	b.ProcessFrame(can.Frame{ID: canIdFromData(130824, 0, 1), Length: 8, Data: [8]uint8{160, 9, (137 & 0xFF), (137 >> 8) | (4 << 5), 1, 2, 3, 4}})
	assert.Nil(t, recvPgn)
	b.ProcessFrame(can.Frame{ID: canIdFromData(130824, 0, 1), Length: 8, Data: [8]uint8{161, 5, 6, 7, 0xFF, 0xFF, 0xFF, 0xFF}})
	assert.IsType(t, MaretronAnnunciator{}, recvPgn)
	recvPgn = nil

	// Now let's force a valid slow packet for 130824
	b.ProcessFrame(can.Frame{ID: canIdFromData(130824, 0, 1), Length: 8, Data: [8]uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF}})
	assert.IsType(t, BGWindData{}, recvPgn)
	recvPgn = nil
}
