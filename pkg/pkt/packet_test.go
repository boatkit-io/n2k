package pkt

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/open-ships/n2k/pkg/pgn"
)

// TestValid verifies that a completely empty Packet fails validation with exactly two errors:
// one for PGN == 0 and one for empty data. This tests the dual-check logic in Valid().
func TestValid(t *testing.T) {
	p := &Packet{}
	p.Valid()
	assert.Equal(t, len(p.ParseErrors), 2)
}

// TestGetManCode verifies that GetManCode correctly extracts the manufacturer code from
// a proprietary PGN's data payload. The test uses PGN 130824 (a proprietary PGN) with
// manufacturer code 381 (B&G) encoded in the first two bytes using the NMEA 2000
// proprietary encoding scheme:
//   - Byte 0: lower 8 bits of manufacturer code (381 & 0xFF = 0x7D)
//   - Byte 1: upper 3 bits of manufacturer code in bits 0-2, industry code in bits 5-7
//     ((381 >> 8) | (4 << 5) = 0x81)
func TestGetManCode(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      130824,
		SourceId: 7,
		Priority: 1,
		TargetId: 0,
	}
	p := Packet{
		Info: pInfo,
		// Encode manufacturer code 381 and industry code 4 into the first two bytes.
		Data: []uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF},
	}
	p.GetManCode()
	assert.Equal(t, pgn.ManufacturerCodeConst(381), p.Manufacturer)
}

// TestGetSeqFrame verifies extraction of the sequence ID and frame number from the first
// byte of fast-packet data. The test byte 0x62 decodes as:
//   - Bits 7-5 = 0b011 = 3 (sequence ID)
//   - Bits 4-0 = 0b00010 = 2 (frame number)
//
// This confirms the bit masking and shifting logic in GetSeqFrame.
func TestGetSeqFrame(t *testing.T) {
	p := &Packet{
		Data: []uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F},
	}
	p.GetSeqFrame()
	assert.Equal(t, uint8(2), p.FrameNum)
	assert.Equal(t, uint8(3), p.SeqId)
}

// TestNewPacket verifies that NewPacket correctly initializes all fields for a known
// proprietary PGN (130824). It checks:
//   - MessageInfo fields are preserved (PGN, SourceId, Priority)
//   - No parse errors for a valid, known PGN
//   - Candidates are populated (PGN 130824 has 2 known variants)
//   - Fast flag is set (130824 is classified as a fast-packet PGN)
//   - GetManCode correctly extracts manufacturer 381 (B&G) after initialization
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
	assert.Equal(t, 2, len(p.Candidates))
	assert.True(t, p.Fast)
	p.GetManCode()
	assert.Equal(t, pgn.ManufacturerCodeConst(381), p.Manufacturer)
}

// TestFilterSlow verifies that AddDecoders correctly filters proprietary PGN candidates
// by manufacturer code. Two scenarios are tested:
//
//  1. Manufacturer 381 (B&G): matches one of the two PGN 130824 candidates, so exactly
//     one decoder is added with no errors.
//
//  2. Manufacturer 380 (not a known manufacturer for PGN 130824): no candidates match,
//     so the Decoders slice remains empty. The "no decoders" error is deferred to the
//     decode stage rather than being added here.
func TestFilterSlow(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      130824,
		SourceId: 7,
		Priority: 1,
		TargetId: 0,
	}
	p := NewPacket(pInfo, []uint8{(381 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF})
	p.AddDecoders()
	assert.Equal(t, 0, len(p.ParseErrors))
	assert.Equal(t, 1, len(p.Decoders))
	pInfo = pgn.MessageInfo{
		PGN:      130824,
		SourceId: 10,
		Priority: 1,
		TargetId: 0,
	}
	// Use manufacturer code 380 (not matching any known PGN 130824 variant) to verify filtering.
	p = NewPacket(pInfo, []uint8{(380 & 0xFF), (381 >> 8) | (4 << 5), 3, 4, 5, 0xFF, 0xFF, 0xFF})
	p.AddDecoders()
	// assert.Equal(t, 1, len(p.ParseErrors)) p.decode() now handles the no decoders error
	assert.Equal(t, 0, len(p.Decoders))
}

// TestFilterFast verifies AddDecoders for PGN 130820, a fast-packet proprietary PGN with
// many manufacturer-specific variants. The test simulates a complete fast packet by:
//   - Stripping the first 2 bytes (sequence/length header) as sequence.complete() would
//   - Setting Complete = true as the MultiBuilder would
//   - Extracting the manufacturer code (419 = Maretron) and filtering decoders
//
// PGN 130820 has many candidate decoders. After filtering by manufacturer 419 (Maretron),
// 16 decoders remain (Maretron defines many message types under this PGN).
func TestFilterFast(t *testing.T) {
	pInfo := pgn.MessageInfo{
		PGN:      130820,
		SourceId: 10,
		Priority: 1,
		TargetId: 0,
	}
	// Data encodes manufacturer code 419 (Maretron) and industry code 4.
	p := NewPacket(pInfo, []uint8{160, 5, (419 & 0xFF), (419 >> 8) | (4 << 5), 32, 128, 1, 255})
	p.Data = p.Data[2:] // normally happens in sequence.complete()
	p.Complete = true   // normally these 2 calls would happen by invoking b.process()
	p.GetManCode()
	p.AddDecoders()
	assert.Equal(t, 0, len(p.ParseErrors))
	assert.Equal(t, 16, len(p.Decoders)) // was 23, but 1 is furuno, and 6 have no sample data
}

// TestBroadcast verifies that NewPacket handles broadcast PGNs (TargetId = 255) correctly.
// PGN 0x1ef00 is a broadcast PGN. The test confirms that the PGN number and broadcast
// target address (255) are preserved in the packet's MessageInfo.
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
