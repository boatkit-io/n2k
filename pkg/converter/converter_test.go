package converter

import (
	"testing"
)

func TestCanIdFromData(t *testing.T) {
	tests := []struct {
		name        string
		pgn         uint32
		sourceId    uint8
		priority    uint8
		destination uint8
		expected    uint32
	}{
		{
			name:        "Address Claim PGN",
			pgn:         60928,
			sourceId:    110,
			priority:    6,
			destination: 255,
			expected:    2565734510, // 0x98EE006E
		},
		{
			name:        "Heartbeat PGN",
			pgn:         126993,
			sourceId:    238,
			priority:    3,
			destination: 255,
			expected:    2381320686, // 0x8DF011EE
		},
		{
			name:        "Targeted PGN",
			pgn:         59904, // PDU1 format (0xEA00)
			sourceId:    100,
			priority:    2,
			destination: 50,
			expected:    2297049700, // 0x88EA3264
		},
		{
			name:        "Zero values",
			pgn:         0,
			sourceId:    0,
			priority:    0,
			destination: 255,
			expected:    2147483648, // 0x80000000
		},
		{
			name:        "Max priority",
			pgn:         126993,
			sourceId:    255,
			priority:    7,
			destination: 255,
			expected:    2649756159, // 0x9DF011FF
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanIdFromData(tt.pgn, tt.sourceId, tt.priority, tt.destination)
			if result != tt.expected {
				t.Errorf("CanIdFromData(%d, %d, %d, %d) = 0x%X, want 0x%X",
					tt.pgn, tt.sourceId, tt.priority, tt.destination, result, tt.expected)
			}
		})
	}
}

func TestDecodeCanId(t *testing.T) {
	tests := []struct {
		name     string
		id       uint32
		expected FrameHeader
	}{
		{
			name: "Address Claim PGN",
			id:   418250862, // 0x18EE006E
			expected: FrameHeader{
				SourceId: 110,
				PGN:      60928,
				Priority: 6,
				TargetId: 0, // PDU2 format - TargetId not set
			},
		},
		{
			name: "Heartbeat PGN",
			id:   233837038, // 0xDF011EE
			expected: FrameHeader{
				SourceId: 238,
				PGN:      126993,
				Priority: 3,
				TargetId: 0, // PDU2 format - TargetId not set
			},
		},
		{
			name: "Targeted PGN",
			id:   149566052, // 0x8EA3264
			expected: FrameHeader{
				SourceId: 100,
				PGN:      59904,
				Priority: 2,
				TargetId: 50, // PDU1 format - TargetId extracted from PGN
			},
		},
		{
			name: "Zero ID",
			id:   0,
			expected: FrameHeader{
				SourceId: 0,
				PGN:      0,
				Priority: 0,
				TargetId: 0, // PDU2 format - TargetId not set
			},
		},
		{
			name: "Max priority",
			id:   502272511, // 0x1DF011FF
			expected: FrameHeader{
				SourceId: 255,
				PGN:      126993,
				Priority: 7,
				TargetId: 0, // PDU2 format - TargetId not set
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecodeCanId(tt.id)
			// Don't compare TimeStamp as it's set to time.Now()
			if result.SourceId != tt.expected.SourceId {
				t.Errorf("DecodeCanId(0x%X).SourceId = %d, want %d",
					tt.id, result.SourceId, tt.expected.SourceId)
			}
			if result.PGN != tt.expected.PGN {
				t.Errorf("DecodeCanId(0x%X).PGN = %d, want %d",
					tt.id, result.PGN, tt.expected.PGN)
			}
			if result.Priority != tt.expected.Priority {
				t.Errorf("DecodeCanId(0x%X).Priority = %d, want %d",
					tt.id, result.Priority, tt.expected.Priority)
			}
			if result.TargetId != tt.expected.TargetId {
				t.Errorf("DecodeCanId(0x%X).TargetId = %d, want %d",
					tt.id, result.TargetId, tt.expected.TargetId)
			}
		})
	}
}

func TestCanIdFromDataAndDecodeCanIdRoundTrip(t *testing.T) {
	tests := []struct {
		name             string
		pgn              uint32
		sourceId         uint8
		priority         uint8
		destination      uint8
		expectedPgn      uint32 // Expected PGN after decode (may change due to PDU format masking)
		expectedTargetId uint8  // Expected TargetId after decode (depends on PDU format)
	}{
		{"Address Claim", 60928, 110, 6, 255, 60928, 0}, // PDU2 - broadcast, TargetId becomes 0
		{"Heartbeat", 126993, 238, 3, 255, 126993, 0},   // PDU2 - broadcast, TargetId becomes 0
		{"Targeted PGN", 59904, 100, 2, 50, 59904, 50},  // PDU1 - targeted, TargetId preserved
		{"Zero values", 0, 0, 0, 255, 0, 0},             // PDU2 - TargetId becomes 0
		{"Min values", 1, 1, 1, 255, 0, 1},              // PDU1 - PGN=1 becomes PGN=0, TargetId=1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			canId := CanIdFromData(tt.pgn, tt.sourceId, tt.priority, tt.destination)

			// Decode
			header := DecodeCanId(canId)

			// Verify round trip
			if header.SourceId != tt.sourceId {
				t.Errorf("SourceId mismatch: got %d, want %d", header.SourceId, tt.sourceId)
			}
			if header.Priority != tt.priority {
				t.Errorf("Priority mismatch: got %d, want %d", header.Priority, tt.priority)
			}
			if header.PGN != tt.expectedPgn {
				t.Errorf("PGN mismatch: got %d, want %d", header.PGN, tt.expectedPgn)
			}
			if header.TargetId != tt.expectedTargetId {
				t.Errorf("TargetId mismatch: got %d, want %d", header.TargetId, tt.expectedTargetId)
			}
		})
	}
}
