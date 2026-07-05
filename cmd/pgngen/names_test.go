package main

import (
	"encoding/json"
	"testing"
)

func TestGenerateConstNamePreservesInitialisms(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{name: "gnss", text: "GNSS fix", expected: "GNSSFix"},
		{name: "dgnss", text: "DGNSS fix", expected: "DGNSSFix"},
		{name: "precise gnss", text: "Precise GNSS", expected: "PreciseGNSS"},
		{name: "rtk fixed", text: "RTK Fixed Integer", expected: "RTKFixedInteger"},
		{name: "rtk float", text: "RTK float", expected: "RTKFloat"},
		{name: "no gnss", text: "No GNSS", expected: "NoGNSS"},
		{name: "estimated dr", text: "Estimated DR mode", expected: "EstimatedDRMode"},
		{name: "normal words", text: "not available", expected: "NotAvailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := generateConstName("Fallback", tt.text, 0)
			if actual != tt.expected {
				t.Fatalf("generateConstName(%q) = %q, want %q", tt.text, actual, tt.expected)
			}
		})
	}
}

func TestGoIdentifierFromCanboatNamePreservesInitialismsFromName(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		source   string
		expected string
	}{
		{name: "gnss struct", id: "gnssPositionData", source: "GNSS Position Data", expected: "GNSSPositionData"},
		{name: "ais struct", id: "aisClassAPositionReport", source: "AIS Class A Position Report", expected: "AISClassAPositionReport"},
		{name: "pgn struct", id: "pgnListTransmitAndReceive", source: "PGN List (Transmit and Receive)", expected: "PGNListTransmitAndReceive"},
		{name: "nmea struct", id: "nmeaRequestGroupFunction", source: "NMEA - Request group function", expected: "NMEARequestGroupFunction"},
		{name: "ac struct", id: "acInputStatus", source: "AC Input Status", expected: "ACInputStatus"},
		{name: "dc struct", id: "dcDetailedStatus", source: "DC Detailed Status", expected: "DCDetailedStatus"},
		{name: "field id", id: "sourceId", source: "Source ID", expected: "SourceID"},
		{name: "plural acronym", id: "gnssDops", source: "GNSS DOPs", expected: "GNSSDOPs"},
		{name: "plural id", id: "numberOfIds", source: "Number of IDs", expected: "NumberOfIDs"},
		{name: "normal word", id: "mode", source: "Mode", expected: "Mode"},
		{name: "first prefix", id: "1stDeviceStatus", source: "1st Device Status", expected: "FirstDeviceStatus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := goIdentifierFromCanboatName(tt.id, tt.source)
			if actual != tt.expected {
				t.Fatalf("goIdentifierFromCanboatName(%q, %q) = %q, want %q", tt.id, tt.source, actual, tt.expected)
			}
		})
	}
}

func TestGoIdentifierDoesNotInventInitialisms(t *testing.T) {
	actual := goIdentifier("sourceId")
	if actual != "SourceId" {
		t.Fatalf("goIdentifier(sourceId) = %q, want SourceId", actual)
	}
}

func TestConvertToConstUsesRawIdentifierCasing(t *testing.T) {
	name := "GNS_METHOD"
	convertToConst(&name)
	if name != "GnsMethodConst" {
		t.Fatalf("convertToConst(GNS_METHOD) = %q, want GnsMethodConst", name)
	}
}

func TestReserveConstNameAvoidsStructNameCollisions(t *testing.T) {
	reservedNames := map[string]bool{"DistanceLog": true}

	actual := reserveConstName("SimnetDataSourceConst", "DistanceLog", reservedNames)
	if actual != "SimnetDataSourceConstDistanceLog" {
		t.Fatalf("reserveConstName collision = %q, want SimnetDataSourceConstDistanceLog", actual)
	}

	actual = reserveConstName("SimnetDataSourceConst", "Depth", reservedNames)
	if actual != "Depth" {
		t.Fatalf("reserveConstName non-collision = %q, want Depth", actual)
	}
}

func TestCanboatFieldDescriptionAcceptsStringNumberAndNull(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected canboatFieldDescription
	}{
		{name: "string", raw: `{"Description":"field description"}`, expected: "field description"},
		{name: "number", raw: `{"Description":21}`, expected: "21"},
		{name: "null", raw: `{"Description":null}`, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var field PGNField
			if err := json.Unmarshal([]byte(tt.raw), &field); err != nil {
				t.Fatalf("json.Unmarshal(%s) returned error: %v", tt.raw, err)
			}
			if field.Description != tt.expected {
				t.Fatalf("Description = %q, want %q", field.Description, tt.expected)
			}
		})
	}
}
