package main

import (
	"fmt"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/internal/pgn"
)

func main() {
	fmt.Println("=== Round-trip Test ===")

	testCases := []struct {
		name        string
		pgn         uint32
		sourceId    uint8
		priority    uint8
		destination uint8
	}{
		{"PDU1 - IsoAcknowledgement with specific dest", pgn.IsoAcknowledgementPgn, 110, 6, 50},
		{"PDU1 - IsoAcknowledgement with broadcast dest", pgn.IsoAcknowledgementPgn, 110, 6, 255},
		{"PDU1 - IsoRequest with specific dest", pgn.IsoRequestPgn, 100, 2, 25},
		{"PDU1 - IsoRequest with broadcast dest", pgn.IsoRequestPgn, 100, 2, 255},
		{"PDU2 - Heartbeat with global dest", pgn.HeartbeatPgn, 238, 3, 255},
		{"PDU2 - Heartbeat with zero dest", pgn.HeartbeatPgn, 238, 3, 0},
		{"PDU2 - VesselHeading with global dest", pgn.VesselHeadingPgn, 200, 2, 255},
		{"PDU2 - VesselHeading with zero dest", pgn.VesselHeadingPgn, 200, 2, 0},
	}

	for _, tc := range testCases {
		// Encode
		canId := converter.CanIdFromData(tc.pgn, tc.sourceId, tc.priority, tc.destination)

		// Decode
		header := converter.DecodeCanId(canId)

		// Check round-trip
		success := true
		if header.SourceId != tc.sourceId {
			fmt.Printf("❌ %s: SourceId mismatch (got %d, want %d)\n", tc.name, header.SourceId, tc.sourceId)
			success = false
		}
		if header.Priority != tc.priority {
			fmt.Printf("❌ %s: Priority mismatch (got %d, want %d)\n", tc.name, header.Priority, tc.priority)
			success = false
		}
		if header.PGN != tc.pgn {
			fmt.Printf("❌ %s: PGN mismatch (got %d, want %d)\n", tc.name, header.PGN, tc.pgn)
			success = false
		}

		// For PDU2 format, TargetId should always be 255 regardless of input destination
		pduFormat := uint8((tc.pgn & 0xFF00) >> 8)
		expectedTargetId := tc.destination
		if pduFormat >= 240 {
			expectedTargetId = 255
		}
		if header.TargetId != expectedTargetId {
			fmt.Printf("❌ %s: TargetId mismatch (got %d, want %d)\n", tc.name, header.TargetId, expectedTargetId)
			success = false
		}

		if success {
			fmt.Printf("✅ %s: Perfect round-trip\n", tc.name)
		}
	}

	fmt.Println("\n=== Validation Test ===")

	// Test validation
	_, err := converter.CanIdFromDataWithValidation(pgn.HeartbeatPgn, 238, 3, 50)
	if err != nil {
		fmt.Printf("✅ Validation correctly rejected PDU2 with invalid destination: %v\n", err)
	} else {
		fmt.Printf("❌ Validation should have rejected PDU2 with invalid destination\n")
	}

	_, err = converter.CanIdFromDataWithValidation(pgn.HeartbeatPgn, 238, 3, 255)
	if err == nil {
		fmt.Printf("✅ Validation correctly allowed PDU2 with valid destination\n")
	} else {
		fmt.Printf("❌ Validation incorrectly rejected PDU2 with valid destination: %v\n", err)
	}
}
