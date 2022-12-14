package n2k

import (
	"strings"
	"testing"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var log = logrus.StandardLogger()

var testData = `
2022-12-20T04:14:09Z,6,129540,22,255,8,20,db,3c,ff,12,1a,d1,15
2022-12-20T04:14:09Z,6,129540,22,255,8,21,fd,86,24,13,00,00,00
2022-12-20T04:14:09Z,6,129540,22,255,8,22,00,f2,1f,c4,31,13,c7
2022-12-20T04:14:09Z,6,129540,22,255,8,23,f8,11,00,00,00,00,f2
2022-12-20T04:14:09Z,6,129540,22,255,8,24,2e,dc,08,14,a9,f8,11
2022-12-20T04:14:09Z,6,129540,22,255,8,25,00,00,00,00,f2,43,96
2022-12-20T04:14:09Z,6,129540,22,255,8,26,38,a1,67,f8,11,00,00
2022-12-20T04:14:09Z,6,129540,22,255,8,27,00,00,f2,0a,22,06,43
2022-12-20T04:14:09Z,6,129540,22,255,8,28,75,30,11,00,00,00,00
2022-12-20T04:14:09Z,6,129540,22,255,8,29,f2,0c,c5,04,39,19,30
2022-12-20T04:14:09Z,6,129540,22,255,8,2a,11,00,00,00,00,f2,19
2022-12-20T04:14:09Z,6,129540,22,255,8,2b,a2,1c,dc,26,cc,10,00
2022-12-20T04:14:09Z,6,129540,22,255,8,2c,00,00,00,f2,30,e8,0a
2022-12-20T04:14:09Z,6,129540,22,255,8,2d,5a,a6,cc,10,00,00,00
2022-12-20T04:14:09Z,6,129540,22,255,8,2e,00,f2,20,dc,26,b8,4d
2022-12-20T04:14:09Z,6,129540,22,255,8,2f,68,10,00,00,00,00,f2
2022-12-20T04:14:09Z,6,129540,22,255,8,30,44,f3,1b,e4,dc,68,10
2022-12-20T04:14:09Z,6,129540,22,255,8,31,00,00,00,00,f2,1d,dc
2022-12-20T04:14:09Z,6,129540,22,255,8,32,08,95,47,04,10,00,00
2022-12-20T04:14:09Z,6,129540,22,255,8,33,00,00,f2,42,16,13,db
2022-12-20T04:14:09Z,6,129540,22,255,8,34,62,04,10,00,00,00,00
2022-12-20T04:14:09Z,6,129540,22,255,8,35,f2,01,68,03,ce,ba,a0
2022-12-20T04:14:09Z,6,129540,22,255,8,36,0f,00,00,00,00,f2,4e
2022-12-20T04:14:09Z,6,129540,22,255,8,37,a2,1c,c2,9a,a0,0f,00
2022-12-20T04:14:09Z,6,129540,22,255,8,38,00,00,00,f2,16,39,37
2022-12-20T04:14:09Z,6,129540,22,255,8,39,f3,2a,3c,0f,00,00,00
2022-12-20T04:14:09Z,6,129540,22,255,8,3a,00,f2,33,68,12,71,9b
2022-12-20T04:14:09Z,6,129540,22,255,8,3b,3c,0f,00,00,00,00,f2
2022-12-20T04:14:09Z,6,129540,22,255,8,3c,55,68,03,a2,0d,3c,0f
2022-12-20T04:14:09Z,6,129540,22,255,8,3d,00,00,00,00,f2,4c,5c
2022-12-20T04:14:09Z,6,129540,22,255,8,3e,10,96,1a,d8,0e,00,00
2022-12-20T04:14:09Z,6,129540,22,255,8,3f,00,00,f2,ff,ff,ff,ff
`

func TestBigPacket(t *testing.T) {

	m := newMultiBuilder(log)
	var p *packet
	lines := strings.Split(testData, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		frame := canFrameFromRaw(line)
		p = newPacket(frame)
		m.add(p)
	}
	assert.True(t, p.complete)
}

func TestFastPacket(t *testing.T) {
	m := newMultiBuilder(log)

	// test fast packet that's actually complete in single-frame
	p := newPacket(can.Frame{ID: canIdFromData(130820, 10, 1, 0), Length: 8, Data: [8]uint8{160, 5, 163, 153, 32, 128, 1, 255}})
	m.add(p)
	assert.True(t, p.complete)
	assert.True(t, p.valid())

	// we allow out of order frames
	m = newMultiBuilder(log)
	p = newPacket(can.Frame{ID: canIdFromData(130820, 10, 1, 0), Length: 8, Data: [8]uint8{161, 5, 163, 153, 32, 128, 1, 255}})
	m.add(p)
	assert.False(t, p.complete)
	assert.NotNil(t, m.sequences[10])
	assert.NotNil(t, m.sequences[10])
	assert.NotNil(t, m.sequences[10][130820][5])

	// test misc multi frame packet
	// Note we only build multi frames for known PGNs.
	// We only know a PGN is multi frame if it's known. We can guess
	// that an unknown pgn with a frame 0 and a valid length byte (0-223, so
	// not much of a test) might be a fast variant, but it's a weak heuristic.
	// Instead we'll return each packet as unknown.
	m = newMultiBuilder(log)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}})
	m.add(p)
	assert.False(t, p.complete)
	assert.Equal(t, 1, len(p.candidates))
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}})
	m.add(p)
	assert.True(t, p.complete)
	assert.Equal(t, 32, len(p.data))
	comp := p.data // used in next test for out of order

	// test misc multi frame packet out of order
	m = newMultiBuilder(log)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}})
	m.add(p)
	assert.False(t, p.complete)
	assert.Equal(t, 1, len(p.candidates))
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}})
	m.add(p)
	assert.True(t, p.complete)
	assert.Equal(t, 32, len(p.data))
	assert.Equal(t, comp, p.data)

	// test misc multi frame packet out of order
	m = newMultiBuilder(log)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}})
	m.add(p)
	assert.False(t, p.complete)
	assert.Equal(t, 1, len(p.candidates))
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}})
	m.add(p)
	assert.True(t, p.complete)
	assert.Equal(t, 32, len(p.data))
	assert.Equal(t, comp, p.data)

	// test that receiving a duplicate frame resets the sequence
	m = newMultiBuilder(log)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}})
	m.add(p)
	// duplicate, so sequence should reset, and after "completing" the packet will remain incomplete
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}})
	m.add(p)
	assert.False(t, p.complete)
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}})
	m.add(p)
	assert.False(t, p.complete)
	assert.Equal(t, 1, len(p.candidates))
	p = newPacket(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}})
	m.add(p)
	assert.False(t, p.complete)
	assert.NotEqual(t, 32, len(p.data))
	assert.NotEqual(t, comp, p.data)
}
