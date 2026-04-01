package canadapter

import (
	"strings"
	"testing"

	"github.com/open-ships/n2k/pkg/pkt"
	"github.com/brutella/can"
	"github.com/stretchr/testify/assert"
)

// testData contains a real-world captured multi-frame fast-packet sequence for PGN 129540
// (GNSS Satellites in View). This is a large message spanning 32 CAN frames from source 22
// with sequence ID 1 (0x20 = seqId=1, frameNum=0). The expected payload length is 0xDB = 219
// bytes. This tests the MultiBuilder's ability to handle large, many-frame sequences.
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

/* var testData2 = `
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,00,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,01,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,02,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,03,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,04,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,05,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,06,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,07,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,08,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,09,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,0a,00,00,00,00,00,00
2023-01-21T15:01:40Z,3,127500,144,0,8,ff,0b,02,00,00,64,00,00
`
*/

// TestBigPacket verifies that the MultiBuilder can assemble a large real-world fast-packet
// sequence (PGN 129540, GNSS Satellites in View) spanning 32 CAN frames. The test parses
// the testData capture line by line, feeding each frame to the MultiBuilder. After the last
// frame, the final packet should be marked Complete, indicating successful assembly of
// all 219 bytes declared in frame 0.
func TestBigPacket(t *testing.T) {

	m := NewMultiBuilder()
	var p *pkt.Packet
	lines := strings.Split(testData, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		frame := CanFrameFromRaw(line)
		pInfo := NewPacketInfo(&frame)
		p = pkt.NewPacket(pInfo, frame.Data[:])
		m.Add(p)
	}
	assert.True(t, p.Complete)
}

// TestFastPacket is a comprehensive test of the MultiBuilder that covers several scenarios:
//
// 1. Single-frame fast packet: A fast-packet PGN (130820) whose entire payload (5 bytes)
//    fits in frame 0. The packet should be immediately Complete after a single Add() call.
//
// 2. Out-of-order initial frame: Sending frame 1 before frame 0. The sequence should not
//    be complete, and the sequence entry should exist in the MultiBuilder's map.
//
// 3. Multi-frame in-order assembly: Sending frames 0-4 in order for a 32-byte payload.
//    The packet should be Complete after frame 4 with exactly 32 bytes of data.
//
// 4. Multi-frame out-of-order assembly: Sending frame 0, then frames in non-sequential
//    order (0, 3, 1, 2, 4). Frame 0 must still come first, but subsequent frames can
//    arrive in any order. The assembled data should be identical to the in-order case.
//
// 5. Duplicate frame detection and reset: Sending a duplicate continuation frame (frame 1
//    twice) triggers a sequence reset. After the reset, the sequence is incomplete and
//    cannot be assembled because it's missing frames. The packet should NOT be Complete
//    and its data should differ from the correctly assembled version.
func TestFastPacket(t *testing.T) {
	m := NewMultiBuilder()

	// --- Scenario 1: Single-frame fast packet ---
	// PGN 130820 with payload length 5 fits entirely in frame 0.
	// Byte 0 = 0xa0: seqId=5, frameNum=0. Byte 1 = 5: expected length.
	pInfo := NewPacketInfo(&can.Frame{ID: CanIdFromData(130820, 10, 1, 0), Length: 8})
	data := []uint8{160, 5, 163, 153, 32, 128, 1, 255}
	p := pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.True(t, p.Complete)
	assert.True(t, p.Valid())

	// --- Scenario 2: Out-of-order initial frame ---
	// Sending frame 1 (0xa1 = seqId=5, frameNum=1) before frame 0. The MultiBuilder should
	// accept it but the sequence won't be complete. Verify the sequence exists in the map
	// under source=10, pgn=130820, seqId=5.
	m = NewMultiBuilder()
	p = pkt.NewPacket(NewPacketInfo(&can.Frame{ID: CanIdFromData(130820, 10, 1, 0), Length: 8}), []uint8{161, 5, 163, 153, 32, 128, 1, 255})
	m.Add(p)
	assert.False(t, p.Complete)
	assert.NotNil(t, m.sequences[10])
	assert.NotNil(t, m.sequences[10])
	assert.NotNil(t, m.sequences[10][130820][5])

	// --- Scenario 3: Multi-frame in-order assembly ---
	// Test a 5-frame sequence for PGN 130820 (using CAN ID 0x09F20183).
	// Frame 0 declares 32 bytes expected (0x20). The sequence completes after frame 4.
	// Note: We only build multi frames for known PGNs. Unknown PGNs return as individual
	// UnknownPGN packets since we can't reliably detect fast-packet for unknown PGNs.
	m = NewMultiBuilder()
	pInfo = NewPacketInfo(&can.Frame{ID: 0x09F20183})
	data = []uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	assert.Equal(t, 1, len(p.Candidates))
	data = []uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.True(t, p.Complete)
	assert.Equal(t, 32, len(p.Data))
	comp := p.Data // Save assembled data for comparison in later tests.

	// --- Scenario 4: Multi-frame out-of-order assembly ---
	// Same 5 frames but sent as: 0, 3, 1, 2, 4 (frame 0 must still be first).
	// The assembled data should be identical to the in-order case.
	m = NewMultiBuilder()
	// Frame 0 must be first!
	data = []uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	assert.Equal(t, 1, len(p.Candidates))
	data = []uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.True(t, p.Complete)
	assert.Equal(t, 32, len(p.Data))
	assert.Equal(t, comp, p.Data)

	// --- Another out-of-order test (same order, verifying reproducibility) ---
	m = NewMultiBuilder()
	data = []uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	assert.Equal(t, 1, len(p.Candidates))
	data = []uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.True(t, p.Complete)
	assert.Equal(t, 32, len(p.Data))
	assert.Equal(t, comp, p.Data)

	// --- Scenario 5: Duplicate frame detection and reset ---
	// Send frames 0, 3, 1, then frame 1 again (duplicate). The duplicate triggers a reset.
	// After the reset, send frames 2 and 4. Because frames 0-1 are missing (lost in reset),
	// the packet cannot complete. Verify it's NOT complete and differs from correct assembly.
	m = NewMultiBuilder()
	data = []uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	data = []uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	// Duplicate frame 1 -- triggers sequence reset.
	data = []uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	// After reset, sequence is incomplete. Continue with remaining frames.
	data = []uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	assert.False(t, p.Complete)
	assert.Equal(t, 1, len(p.Candidates))
	data = []uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}
	p = pkt.NewPacket(pInfo, data)
	m.Add(p)
	// Packet should NOT be complete because the reset lost frames 0 and 3.
	assert.False(t, p.Complete)
	assert.NotEqual(t, 32, len(p.Data))
	assert.NotEqual(t, comp, p.Data)
}
