package n2k

import (
	"testing"

	"github.com/brutella/can"
	"github.com/stretchr/testify/assert"
)

func TestFastPacket(t *testing.T) {
	m := NewMultiBuilder()

	// test fast packet that's actually complete in single-frame
	p := NewPacket(can.Frame{ID: canIdFromData(130820, 10, 1), Length: 8, Data: [8]uint8{160, 5, 163, 153, 32, 128, 1, 255}})
	m.Add(p)
	assert.True(t, p.Complete)
	assert.True(t, p.valid())

	// we allow out of order frames
	m = NewMultiBuilder()
	p = NewPacket(can.Frame{ID: canIdFromData(130820, 10, 1), Length: 8, Data: [8]uint8{161, 5, 163, 153, 32, 128, 1, 255}})
	m.Add(p)
	assert.False(t, p.Complete)
	assert.NotNil(t, m.sequences[10])
	assert.NotNil(t, m.sequences[10])
	assert.NotNil(t, m.sequences[10][130820][5])

	// test misc multi frame packet
	// Note we only build multi frames for known PGNs.
	// We only know a PGN is multi frame if it's know. We can guess
	// that an unknown pgn with a frame 0 and a valid length byte (0-223, so
	// not much of a test) might be a fast variant, but it's a weak heuristic.
	// Instead we'll return each packet as unknown.
	m = NewMultiBuilder()
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}})
	m.Add(p)
	assert.False(t, p.Complete)
	assert.Equal(t, 1, len(p.Candidates))
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}})
	m.Add(p)
	assert.False(t, p.Complete)
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}})
	m.Add(p)
	assert.False(t, p.Complete)
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}})
	m.Add(p)
	assert.False(t, p.Complete)
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}})
	m.Add(p)
	assert.True(t, p.Complete)
	assert.Equal(t, 32, len(p.Data))
	comp := p.Data

	// test misc multi frame packet out of order
	m = NewMultiBuilder()
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}})
	m.Add(p)
	assert.False(t, p.Complete)
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}})
	m.Add(p)
	assert.False(t, p.Complete)
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}})
	m.Add(p)
	assert.False(t, p.Complete)
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}})
	m.Add(p)
	assert.False(t, p.Complete)
	assert.Equal(t, 1, len(p.Candidates))
	p = NewPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}})
	m.Add(p)
	assert.True(t, p.Complete)
	assert.Equal(t, 32, len(p.Data))
	assert.Equal(t, comp, p.Data)

}
