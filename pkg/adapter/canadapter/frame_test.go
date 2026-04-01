package canadapter

import (
	"testing"

	"github.com/brutella/can"
	"github.com/stretchr/testify/assert"
)

// TestNewPacketInfo_BroadcastPGN verifies extraction of NMEA 2000 metadata from a CAN ID
// for a broadcast PGN. PGN 127250 (Vessel Heading) has PDU Format 0xF1 (241 >= 240), so
// it's a broadcast message. The PS field is part of the PGN (group extension), not a
// destination address. TargetId should remain 0.
func TestNewPacketInfo_BroadcastPGN(t *testing.T) {
	// PGN 127250 (0x1F112) is a broadcast PGN (PDU format 0xF1 >= 240)
	// Source ID = 42, Priority = 2
	pgn := uint32(127250)
	source := uint8(42)
	priority := uint8(2)
	destination := uint8(0)

	canID := CanIdFromData(pgn, source, priority, destination)
	frame := can.Frame{ID: canID, Length: 8}
	info := NewPacketInfo(&frame)

	assert.Equal(t, pgn, info.PGN, "PGN should match for broadcast")
	assert.Equal(t, source, info.SourceId, "SourceId should match")
	assert.Equal(t, priority, info.Priority, "Priority should match")
	assert.Equal(t, uint8(0), info.TargetId, "TargetId should be 0 for broadcast PGN")
}

// TestNewPacketInfo_AddressedPGN verifies metadata extraction for an addressed (point-to-point)
// PGN. PGN 59904 (ISO Request) has PDU Format 0xEA (234 < 240), making it an addressed
// message. When destination is 0, the lower byte of the PGN field is masked off, resulting
// in PGN 0xEA00. TargetId should be 0 since destination is 0.
func TestNewPacketInfo_AddressedPGN(t *testing.T) {
	// PGN 59904 (0xEA00) is addressed (PDU format 0xEA < 240)
	// CanIdFromData ORs destination into the low byte alongside source,
	// so for addressed PGNs we construct the CAN ID manually to verify TargetId extraction.
	// See TestNewPacketInfo_AddressedPGN_WithTarget for the manual construction.
	// Here we use CanIdFromData with destination=0 to avoid bit collision.
	pgn := uint32(59904)
	source := uint8(100)
	priority := uint8(6)
	destination := uint8(0)

	canID := CanIdFromData(pgn, source, priority, destination)
	frame := can.Frame{ID: canID, Length: 8}
	info := NewPacketInfo(&frame)

	assert.Equal(t, uint32(0xEA00), info.PGN, "Addressed PGN should have lower byte masked to 0")
	assert.Equal(t, source, info.SourceId, "SourceId should match")
	assert.Equal(t, priority, info.Priority, "Priority should match")
	assert.Equal(t, uint8(0), info.TargetId, "TargetId should be 0 when destination is 0")
}

// TestNewPacketInfo_AddressedPGN_WithTarget verifies that TargetId is correctly extracted
// from an addressed PGN with a non-zero destination. The CAN ID is constructed manually
// (bypassing CanIdFromData) to properly place the destination in the PS field of the PGN
// rather than OR'ing it into the source byte.
//
// CAN ID construction for PGN 59904 with destination 5:
//   - Priority (6) in bits 28-26
//   - PF (0xEA) in bits 23-16
//   - PS/destination (5) in bits 15-8
//   - Source (10) in bits 7-0
//
// NewPacketInfo should extract PGN as 0xEA00 (lower byte masked) and TargetId as 5.
func TestNewPacketInfo_AddressedPGN_WithTarget(t *testing.T) {
	// Build an addressed PGN manually to verify TargetId extraction.
	// PGN 59904 = 0xEA00. For addressed PGNs, bits 8-15 of the CAN ID's PGN field
	// contain the destination address. We'll set it to 5.
	// CAN ID format: priority(3) | reserved(1) | DP(1) | PF(8) | PS(8) | SA(8)
	// For PGN 59904, PF=0xEA, PS=destination
	// So the full "PGN field" in the CAN ID = 0xEA00 | destination = 0xEA05
	source := uint8(10)
	priority := uint8(6)
	destination := uint8(5)

	// Build CAN ID manually: priority << 26 | (PF << 16 | PS << 8) | source
	// PF = 0xEA, PS = destination = 5
	canID := uint32(priority)<<26 | uint32(0xEA)<<16 | uint32(destination)<<8 | uint32(source)
	frame := can.Frame{ID: canID, Length: 8}
	info := NewPacketInfo(&frame)

	assert.Equal(t, uint32(0xEA00), info.PGN, "Addressed PGN should have lower byte masked")
	assert.Equal(t, destination, info.TargetId, "TargetId should be the destination")
	assert.Equal(t, source, info.SourceId)
	assert.Equal(t, priority, info.Priority)
}

// TestNewPacketInfo_PriorityExtraction verifies that all 8 possible priority values (0-7)
// are correctly extracted from bits 26-28 of the CAN ID. Priority 0 is the highest
// priority on the NMEA 2000 bus; priority 7 is the lowest.
func TestNewPacketInfo_PriorityExtraction(t *testing.T) {
	// Priority occupies bits 26-28 of the CAN ID (3 bits, values 0-7).
	tests := []struct {
		priority uint8
	}{
		{0}, {1}, {2}, {3}, {4}, {5}, {6}, {7},
	}
	for _, tc := range tests {
		canID := CanIdFromData(127250, 1, tc.priority, 0)
		frame := can.Frame{ID: canID, Length: 8}
		info := NewPacketInfo(&frame)
		assert.Equal(t, tc.priority, info.Priority, "Priority %d should be extracted correctly", tc.priority)
	}
}

// TestNewPacketInfo_SourceIdExtraction verifies that various source address values are
// correctly extracted from bits 0-7 of the CAN ID. NMEA 2000 supports source addresses
// 0-255, with 255 typically reserved for broadcast.
func TestNewPacketInfo_SourceIdExtraction(t *testing.T) {
	// Source ID occupies bits 0-7 of the CAN ID (8 bits, values 0-255).
	tests := []uint8{0, 1, 127, 128, 255}
	for _, src := range tests {
		canID := CanIdFromData(127250, src, 3, 0)
		frame := can.Frame{ID: canID, Length: 8}
		info := NewPacketInfo(&frame)
		assert.Equal(t, src, info.SourceId, "SourceId %d should be extracted correctly", src)
	}
}

// TestCanIdFromData_RoundTrip_Broadcast verifies that encoding a broadcast PGN into a CAN
// ID and then decoding it back yields the original values. This is the fundamental
// round-trip property that CanIdFromData and NewPacketInfo must satisfy for broadcast PGNs.
func TestCanIdFromData_RoundTrip_Broadcast(t *testing.T) {
	// Round-trip: encode then decode should match for broadcast PGN.
	pgn := uint32(127250)
	source := uint8(42)
	priority := uint8(2)

	canID := CanIdFromData(pgn, source, priority, 0)
	frame := can.Frame{ID: canID, Length: 8}
	info := NewPacketInfo(&frame)

	assert.Equal(t, pgn, info.PGN)
	assert.Equal(t, source, info.SourceId)
	assert.Equal(t, priority, info.Priority)
}

// TestCanIdFromData_RoundTrip_MultipleValues verifies the encode/decode round-trip property
// across several different broadcast PGNs with varying source and priority values.
// Each test case encodes the values into a CAN ID and decodes them back, verifying all
// fields match the originals.
func TestCanIdFromData_RoundTrip_MultipleValues(t *testing.T) {
	// Test with several broadcast PGNs.
	testCases := []struct {
		pgn      uint32
		source   uint8
		priority uint8
	}{
		{127250, 0, 0},
		{127501, 224, 3},
		{130820, 10, 1},
		{129029, 22, 6},
		{128259, 200, 5},
	}
	for _, tc := range testCases {
		canID := CanIdFromData(tc.pgn, tc.source, tc.priority, 0)
		frame := can.Frame{ID: canID, Length: 8}
		info := NewPacketInfo(&frame)

		assert.Equal(t, tc.pgn, info.PGN, "PGN round-trip failed for %d", tc.pgn)
		assert.Equal(t, tc.source, info.SourceId, "Source round-trip failed for %d", tc.source)
		assert.Equal(t, tc.priority, info.Priority, "Priority round-trip failed for %d", tc.priority)
	}
}

// TestCanFrameFromRaw_KnownInput verifies that CanFrameFromRaw correctly parses a known
// raw CAN log line into a can.Frame with the expected CAN ID and data bytes. The test uses
// PGN 127501 from source 224 with priority 3 and verifies both the reconstructed CAN ID
// and all 8 data bytes.
func TestCanFrameFromRaw_KnownInput(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f := CanFrameFromRaw(raw)

	// Verify the frame ID encodes PGN=127501, source=224, priority=3, destination=0.
	expectedID := CanIdFromData(127501, 224, 3, 0)
	assert.Equal(t, expectedID, f.ID)
	assert.Equal(t, uint8(8), f.Length)

	// Verify all 8 data bytes were parsed correctly from the hex values.
	assert.Equal(t, uint8(0x00), f.Data[0])
	assert.Equal(t, uint8(0x03), f.Data[1])
	assert.Equal(t, uint8(0xc0), f.Data[2])
	assert.Equal(t, uint8(0xff), f.Data[3])
	assert.Equal(t, uint8(0xff), f.Data[4])
	assert.Equal(t, uint8(0xff), f.Data[5])
	assert.Equal(t, uint8(0xff), f.Data[6])
	assert.Equal(t, uint8(0xff), f.Data[7])
}

// TestCanFrameFromRaw_DecodesCorrectly verifies that a parsed raw line produces a frame
// whose CAN ID decodes to the correct PGN, source, and priority via NewPacketInfo.
// Uses destination=0 to avoid the CanIdFromData bit-OR collision between source and
// destination bytes.
func TestCanFrameFromRaw_DecodesCorrectly(t *testing.T) {
	// Use a raw line with destination=0 to avoid the CanIdFromData bit-OR collision
	// between source and destination in the low byte.
	raw := "2022-12-20T04:14:09Z,6,129540,22,0,8,20,db,3c,ff,12,1a,d1,15"
	f := CanFrameFromRaw(raw)

	info := NewPacketInfo(&f)
	assert.Equal(t, uint32(129540), info.PGN)
	assert.Equal(t, uint8(22), info.SourceId)
	assert.Equal(t, uint8(6), info.Priority)
	assert.Equal(t, uint8(8), f.Length)
	assert.Equal(t, uint8(0x20), f.Data[0])
	assert.Equal(t, uint8(0xdb), f.Data[1])
	assert.Equal(t, uint8(0x15), f.Data[7])
}

// TestCanFrameFromRaw_DestinationBitCollision documents a known limitation of CanIdFromData:
// it ORs the destination byte into the same low byte as the source address. For broadcast
// PGNs (destination=255), this causes the source bits to be masked by the destination.
// In this test, source 22 (0x16) OR'd with destination 255 (0xFF) = 0xFF, so the extracted
// SourceId is 0xFF instead of 22. This is acceptable behavior for the test utility because
// real CAN hardware places the destination in the PS field (bits 15-8), not in the source
// byte.
func TestCanFrameFromRaw_DestinationBitCollision(t *testing.T) {
	// CanIdFromData ORs destination into the same low byte as source.
	// For broadcast PGNs with destination=255, the source bits get masked.
	// This test documents that behavior.
	raw := "2022-12-20T04:14:09Z,6,129540,22,255,8,20,db,3c,ff,12,1a,d1,15"
	f := CanFrameFromRaw(raw)

	info := NewPacketInfo(&f)
	assert.Equal(t, uint32(129540), info.PGN)
	// Source 22 (0x16) OR'd with destination 255 (0xFF) = 0xFF.
	assert.Equal(t, uint8(0xFF), info.SourceId, "Source is OR'd with destination in CanIdFromData")
	assert.Equal(t, uint8(6), info.Priority)
}

// TestCanFrameFromRaw_ShorterLength verifies that frames with fewer than 8 data bytes
// are handled correctly. The declared length is 3, so only 3 data bytes are parsed from
// the CSV. The remaining 5 bytes in the 8-byte array should be zero (Go default).
// Note: CanFrameFromRaw always sets Length=8 regardless of the declared length, because
// NMEA 2000 CAN frames always carry 8 bytes on the wire.
func TestCanFrameFromRaw_ShorterLength(t *testing.T) {
	// A frame with only 3 data bytes.
	raw := "2023-01-01T00:00:00Z,2,127250,1,0,3,aa,bb,cc"
	f := CanFrameFromRaw(raw)

	assert.Equal(t, uint8(8), f.Length) // Length is always set to 8 in CanFrameFromRaw
	assert.Equal(t, uint8(0xaa), f.Data[0])
	assert.Equal(t, uint8(0xbb), f.Data[1])
	assert.Equal(t, uint8(0xcc), f.Data[2])
	// Remaining bytes should be zero (default).
	assert.Equal(t, uint8(0x00), f.Data[3])
}
