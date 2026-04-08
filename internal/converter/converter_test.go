package converter

import (
	"testing"

	"github.com/boatkit-io/n2k/internal/pgn"
)

func TestCanIDFromData(t *testing.T) {
	tests := []struct {
		name        string
		pgn         uint32
		sourceID    uint8
		priority    uint8
		destination uint8
		expected    uint32
	}{
		{
			name:        "Address Claim PGN",
			pgn:         60928,
			sourceID:    110,
			priority:    6,
			destination: 255,
			expected:    2565799790, // 0x98EEFF6E (PDU1 format, destination 255 encoded)
		},
		{
			name:        "Heartbeat PGN",
			pgn:         126993,
			sourceID:    238,
			priority:    3,
			destination: 255,
			expected:    2381320686, // 0x8DF011EE
		},
		{
			name:        "Targeted PGN",
			pgn:         59904, // PDU1 format (0xEA00)
			sourceID:    100,
			priority:    2,
			destination: 50,
			expected:    2297049700, // 0x88EA3264
		},
		{
			name:        "Zero values",
			pgn:         0,
			sourceID:    0,
			priority:    0,
			destination: 255,
			expected:    2147548928, // 0x8000FF00 (PDU1 format, destination 255 encoded)
		},
		{
			name:        "Max priority",
			pgn:         126993,
			sourceID:    255,
			priority:    7,
			destination: 255,
			expected:    2649756159, // 0x9DF011FF
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanIDFromData(tt.pgn, tt.sourceID, tt.priority, tt.destination)
			if result != tt.expected {
				t.Errorf("CanIDFromData(%d, %d, %d, %d) = 0x%X, want 0x%X",
					tt.pgn, tt.sourceID, tt.priority, tt.destination, result, tt.expected)
			}
		})
	}
}

func TestDecodeCanID(t *testing.T) {
	tests := []struct {
		name     string
		id       uint32
		expected FrameHeader
	}{
		{
			name: "Address Claim PGN",
			id:   2565799790, // 0x98EEFF6E - actual CAN ID for PGN 60928, source 110, priority 6, dest 255
			expected: FrameHeader{
				SourceID: 110,
				PGN:      pgn.IsoAddressClaimPgn,
				Priority: 6,
				TargetID: 255, // PDU1 format - TargetID extracted from PGN
			},
		},
		{
			name: "Heartbeat PGN",
			id:   233837038, // 0xDF011EE
			expected: FrameHeader{
				SourceID: 238,
				PGN:      126993,
				Priority: 3,
				TargetID: 255, // PDU2 format - TargetID always 255
			},
		},
		{
			name: "Targeted PGN",
			id:   149566052, // 0x8EA3264
			expected: FrameHeader{
				SourceID: 100,
				PGN:      59904,
				Priority: 2,
				TargetID: 50, // PDU1 format - TargetID extracted from PGN
			},
		},
		{
			name: "Zero ID",
			id:   0,
			expected: FrameHeader{
				SourceID: 0,
				PGN:      0,
				Priority: 0,
				TargetID: 0, // PDU1 format with PGN=0, TargetID=0
			},
		},
		{
			name: "Max priority",
			id:   502272511, // 0x1DF011FF
			expected: FrameHeader{
				SourceID: 255,
				PGN:      126993,
				Priority: 7,
				TargetID: 255, // PDU2 format - TargetID always 255
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecodeCanID(tt.id)
			// Don't compare TimeStamp as it's set to time.Now()
			if result.SourceID != tt.expected.SourceID {
				t.Errorf("DecodeCanID(0x%X).SourceID = %d, want %d",
					tt.id, result.SourceID, tt.expected.SourceID)
			}
			if result.PGN != tt.expected.PGN {
				t.Errorf("DecodeCanID(0x%X).PGN = %d, want %d",
					tt.id, result.PGN, tt.expected.PGN)
			}
			if result.Priority != tt.expected.Priority {
				t.Errorf("DecodeCanID(0x%X).Priority = %d, want %d",
					tt.id, result.Priority, tt.expected.Priority)
			}
			if result.TargetID != tt.expected.TargetID {
				t.Errorf("DecodeCanID(0x%X).TargetID = %d, want %d",
					tt.id, result.TargetID, tt.expected.TargetID)
			}
		})
	}
}

func TestCanIDFromDataAndDecodeCanIDRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		pgn         uint32
		sourceID    uint8
		priority    uint8
		destination uint8
		expectError bool
	}{
		// PDU1 format tests (PF < 240)
		{"IsoAcknowledgement PDU1 with specific destination", pgn.IsoAcknowledgementPgn, 110, 6, 50, false},
		{"IsoAcknowledgement PDU1 with broadcast destination", pgn.IsoAcknowledgementPgn, 110, 6, 255, false},
		{"IsoRequest PDU1 with specific destination", pgn.IsoRequestPgn, 100, 2, 25, false},
		{"IsoRequest PDU1 with broadcast destination", pgn.IsoRequestPgn, 100, 2, 255, false},

		// PDU2 format tests (PF >= 240)
		{"Heartbeat PDU2 with global destination", pgn.HeartbeatPgn, 238, 3, 255, false},
		{"Heartbeat PDU2 with zero destination", pgn.HeartbeatPgn, 238, 3, 0, false},
		{"VesselHeading PDU2 with global destination", pgn.VesselHeadingPgn, 200, 2, 255, false},
		{"VesselHeading PDU2 with zero destination", pgn.VesselHeadingPgn, 200, 2, 0, false},

		// Invalid PDU2 cases
		{"Heartbeat PDU2 with invalid destination", pgn.HeartbeatPgn, 238, 3, 50, true},
		{"VesselHeading PDU2 with invalid destination", pgn.VesselHeadingPgn, 200, 2, 25, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with validation
			canID, err := CanIDFromDataWithValidation(tt.pgn, tt.sourceID, tt.priority, tt.destination)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for PDU2 PGN with invalid destination, got none")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Decode
			header := DecodeCanID(canID)

			// Verify round trip - all values should match exactly
			if header.SourceID != tt.sourceID {
				t.Errorf("SourceID mismatch: got %d, want %d", header.SourceID, tt.sourceID)
			}
			if header.Priority != tt.priority {
				t.Errorf("Priority mismatch: got %d, want %d", header.Priority, tt.priority)
			}
			if header.PGN != tt.pgn {
				t.Errorf("PGN mismatch: got %d, want %d", header.PGN, tt.pgn)
			}

			// For PDU2 format, TargetID should always be 255 regardless of input destination
			pduFormat := uint8((tt.pgn & 0xFF00) >> 8)
			expectedTargetID := tt.destination
			if pduFormat >= 240 {
				expectedTargetID = 255
			}
			if header.TargetID != expectedTargetID {
				t.Errorf("TargetID mismatch: got %d, want %d", header.TargetID, expectedTargetID)
			}
		})
	}
}

func TestCanIDFromDataWithoutValidation(t *testing.T) {
	tests := []struct {
		name        string
		pgn         uint32
		sourceID    uint8
		priority    uint8
		destination uint8
	}{
		// Test that CanIDFromData still works without validation
		{"Heartbeat PDU2 with invalid destination (no validation)", pgn.HeartbeatPgn, 238, 3, 50},
		{"VesselHeading PDU2 with invalid destination (no validation)", pgn.VesselHeadingPgn, 200, 2, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should not error, but the round-trip behavior may be unexpected
			canID := CanIDFromData(tt.pgn, tt.sourceID, tt.priority, tt.destination)
			header := DecodeCanID(canID)

			// For PDU2, TargetID should always be 255 regardless of input destination
			if header.TargetID != 255 {
				t.Errorf("PDU2 TargetID should be 255, got %d", header.TargetID)
			}
		})
	}
}

func TestPriorityHandling(t *testing.T) {
	tests := []struct {
		name        string
		pgn         uint32
		sourceID    uint8
		priority    uint8
		destination uint8
	}{
		// Test all valid priority values (0-7) for PDU1 format
		{"PDU1 Priority 0", pgn.IsoRequestPgn, 100, 0, 50},
		{"PDU1 Priority 1", pgn.IsoRequestPgn, 100, 1, 50},
		{"PDU1 Priority 2", pgn.IsoRequestPgn, 100, 2, 50},
		{"PDU1 Priority 3", pgn.IsoRequestPgn, 100, 3, 50},
		{"PDU1 Priority 4", pgn.IsoRequestPgn, 100, 4, 50},
		{"PDU1 Priority 5", pgn.IsoRequestPgn, 100, 5, 50},
		{"PDU1 Priority 6", pgn.IsoRequestPgn, 100, 6, 50},
		{"PDU1 Priority 7", pgn.IsoRequestPgn, 100, 7, 50},

		// Test all valid priority values (0-7) for PDU2 format
		{"PDU2 Priority 0", pgn.HeartbeatPgn, 200, 0, 255},
		{"PDU2 Priority 1", pgn.HeartbeatPgn, 200, 1, 255},
		{"PDU2 Priority 2", pgn.HeartbeatPgn, 200, 2, 255},
		{"PDU2 Priority 3", pgn.HeartbeatPgn, 200, 3, 255},
		{"PDU2 Priority 4", pgn.HeartbeatPgn, 200, 4, 255},
		{"PDU2 Priority 5", pgn.HeartbeatPgn, 200, 5, 255},
		{"PDU2 Priority 6", pgn.HeartbeatPgn, 200, 6, 255},
		{"PDU2 Priority 7", pgn.HeartbeatPgn, 200, 7, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			canID := CanIDFromData(tt.pgn, tt.sourceID, tt.priority, tt.destination)

			// Decode
			header := DecodeCanID(canID)

			// Check priority round-trip
			if header.Priority != tt.priority {
				t.Errorf("Priority mismatch: got %d, want %d", header.Priority, tt.priority)
			}

			// Verify priority is in correct bit position in CAN ID
			expectedPriorityBits := (canID & 0x1C000000) >> 26
			if expectedPriorityBits != uint32(tt.priority) {
				t.Errorf("Priority bits in CAN ID incorrect: got %d, want %d", expectedPriorityBits, tt.priority)
			}
		})
	}
}

func TestPriorityMasking(t *testing.T) {
	tests := []struct {
		name     string
		priority uint8
		expected uint8
	}{
		{"Priority 8 should be masked to 0", 8, 0},
		{"Priority 9 should be masked to 1", 9, 1},
		{"Priority 10 should be masked to 2", 10, 2},
		{"Priority 11 should be masked to 3", 11, 3},
		{"Priority 12 should be masked to 4", 12, 4},
		{"Priority 13 should be masked to 5", 13, 5},
		{"Priority 14 should be masked to 6", 14, 6},
		{"Priority 15 should be masked to 7", 15, 7},
		{"Priority 255 should be masked to 7", 255, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canID := CanIDFromData(pgn.HeartbeatPgn, 200, tt.priority, 255)
			header := DecodeCanID(canID)

			if header.Priority != tt.expected {
				t.Errorf("Priority masking failed: got %d, want %d", header.Priority, tt.expected)
			}
		})
	}
}

func TestCanIDFromStruct(t *testing.T) {
	tests := []struct {
		name     string
		data     CanIDData
		expected uint32
	}{
		{
			"Address Claim PGN",
			CanIDData{PGN: pgn.IsoAddressClaimPgn, SourceID: 110, Priority: 6, Destination: 255},
			0x98EEFF6E, // Same as TestCanIDFromData
		},
		{
			"Speed PGN (PDU2)",
			CanIDData{PGN: pgn.SpeedPgn, SourceID: 238, Priority: 3, Destination: 255},
			0x8DF503EE, // Speed PGN with source 238, priority 3, destination 255
		},
		{
			"Zero values",
			CanIDData{PGN: 0, SourceID: 0, Priority: 0, Destination: 0},
			0x80000000, // Same as TestCanIDFromData
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanIDFromStruct(tt.data)
			if result != tt.expected {
				t.Errorf("expected 0x%08X, got 0x%08X", tt.expected, result)
			}
		})
	}
}

func TestCanIDFromStructWithValidation(t *testing.T) {
	tests := []struct {
		name        string
		data        CanIDData
		expectError bool
	}{
		{
			"Valid PDU1 with specific destination",
			CanIDData{PGN: pgn.IsoAcknowledgementPgn, SourceID: 100, Priority: 3, Destination: 200},
			false,
		},
		{
			"Valid PDU2 with global destination",
			CanIDData{PGN: pgn.HeartbeatPgn, SourceID: 100, Priority: 3, Destination: 255},
			false,
		},
		{
			"Valid PDU2 with zero destination",
			CanIDData{PGN: pgn.HeartbeatPgn, SourceID: 100, Priority: 3, Destination: 0},
			false,
		},
		{
			"Invalid PDU2 with specific destination",
			CanIDData{PGN: pgn.HeartbeatPgn, SourceID: 100, Priority: 3, Destination: 200},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CanIDFromStructWithValidation(tt.data)
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
