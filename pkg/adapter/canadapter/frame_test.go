package canadapter

import (
	"testing"

	"github.com/brutella/can"
	"github.com/stretchr/testify/assert"
)

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

func TestNewPacketInfo_AddressedPGN_WithTarget(t *testing.T) {
	// Build an addressed PGN manually to verify TargetId extraction
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

func TestNewPacketInfo_PriorityExtraction(t *testing.T) {
	// Priority occupies bits 26-28 of the CAN ID
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

func TestNewPacketInfo_SourceIdExtraction(t *testing.T) {
	// Source ID occupies bits 0-7 of the CAN ID
	tests := []uint8{0, 1, 127, 128, 255}
	for _, src := range tests {
		canID := CanIdFromData(127250, src, 3, 0)
		frame := can.Frame{ID: canID, Length: 8}
		info := NewPacketInfo(&frame)
		assert.Equal(t, src, info.SourceId, "SourceId %d should be extracted correctly", src)
	}
}

func TestCanIdFromData_RoundTrip_Broadcast(t *testing.T) {
	// Round-trip: encode then decode should match for broadcast PGN
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

func TestCanIdFromData_RoundTrip_MultipleValues(t *testing.T) {
	// Test with several broadcast PGNs
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

func TestCanFrameFromRaw_KnownInput(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f := CanFrameFromRaw(raw)

	// Verify the frame ID encodes PGN=127501, source=224, priority=3, destination=0
	expectedID := CanIdFromData(127501, 224, 3, 0)
	assert.Equal(t, expectedID, f.ID)
	assert.Equal(t, uint8(8), f.Length)

	// Verify data bytes
	assert.Equal(t, uint8(0x00), f.Data[0])
	assert.Equal(t, uint8(0x03), f.Data[1])
	assert.Equal(t, uint8(0xc0), f.Data[2])
	assert.Equal(t, uint8(0xff), f.Data[3])
	assert.Equal(t, uint8(0xff), f.Data[4])
	assert.Equal(t, uint8(0xff), f.Data[5])
	assert.Equal(t, uint8(0xff), f.Data[6])
	assert.Equal(t, uint8(0xff), f.Data[7])
}

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

func TestCanFrameFromRaw_DestinationBitCollision(t *testing.T) {
	// CanIdFromData ORs destination into the same low byte as source.
	// For broadcast PGNs with destination=255, the source bits get masked.
	// This test documents that behavior.
	raw := "2022-12-20T04:14:09Z,6,129540,22,255,8,20,db,3c,ff,12,1a,d1,15"
	f := CanFrameFromRaw(raw)

	info := NewPacketInfo(&f)
	assert.Equal(t, uint32(129540), info.PGN)
	// Source 22 (0x16) OR'd with destination 255 (0xFF) = 0xFF
	assert.Equal(t, uint8(0xFF), info.SourceId, "Source is OR'd with destination in CanIdFromData")
	assert.Equal(t, uint8(6), info.Priority)
}

func TestCanFrameFromRaw_ShorterLength(t *testing.T) {
	// A frame with only 3 data bytes
	raw := "2023-01-01T00:00:00Z,2,127250,1,0,3,aa,bb,cc"
	f := CanFrameFromRaw(raw)

	assert.Equal(t, uint8(8), f.Length) // Length is always set to 8 in CanFrameFromRaw
	assert.Equal(t, uint8(0xaa), f.Data[0])
	assert.Equal(t, uint8(0xbb), f.Data[1])
	assert.Equal(t, uint8(0xcc), f.Data[2])
	// Remaining bytes should be zero (default)
	assert.Equal(t, uint8(0x00), f.Data[3])
}
