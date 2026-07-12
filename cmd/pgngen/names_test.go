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
		{name: "aton report", text: "ATON report", expected: "ATONReport"},
		{name: "ais utc", text: "AIS UTC and date report", expected: "AISUTCAndDateReport"},
		{name: "cog sog", text: "COG/SOG, rapid update", expected: "COGSOGRapidUpdate"},
		{name: "gps vhf", text: "GPS/VHF MOB alert", expected: "GPSVHFMOBAlert"},
		{name: "sid", text: "SID", expected: "SID"},
		{name: "rms", text: "AC RMS voltage", expected: "ACRMSVoltage"},
		{name: "glonass", text: "GLONASS almanac data", expected: "GLONASSAlmanacData"},
		{name: "usb rds eq", text: "USB RDS EQ", expected: "USBRDSEQ"},
		{name: "sar", text: "SAR aircraft position", expected: "SARAircraftPosition"},
		{name: "sart", text: "AIS-SART", expected: "AISSART"},
		{name: "dgps", text: "GPS and DGPS Info", expected: "GPSAndDGPSInfo"},
		{name: "waas dgps", text: "WAAS/DGPS", expected: "WAASDGPS"},
		{name: "no gnss", text: "No GNSS", expected: "NoGNSS"},
		{name: "estimated dr", text: "Estimated DR mode", expected: "EstimatedDRMode"},
		{name: "concatenated initialisms", text: "AISUTC report", expected: "AISUTCReport"},
		{name: "unknown capital run", text: "ARKS Enterprises Inc", expected: "ArksEnterprisesInc"},
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
		{
			name:     "aton struct",
			id:       "aisAidsToNavigationAtonReport",
			source:   "AIS Aids to Navigation (AtoN) Report",
			expected: "AISAidsToNavigationATONReport",
		},
		{name: "aton field", id: "virtualAtonFlag", source: "Virtual AtoN Flag", expected: "VirtualATONFlag"},
		{name: "gps field", id: "gpsQuality", source: "GPS Quality", expected: "GPSQuality"},
		{name: "utc field", id: "utcDate", source: "UTC Date", expected: "UTCDate"},
		{name: "cog sog struct", id: "cogSogRapidUpdate", source: "COG/SOG, Rapid Update", expected: "COGSOGRapidUpdate"},
		{name: "ac struct", id: "acInputStatus", source: "AC Input Status", expected: "ACInputStatus"},
		{name: "dc struct", id: "dcDetailedStatus", source: "DC Detailed Status", expected: "DCDetailedStatus"},
		{name: "field id", id: "sourceId", source: "Source ID", expected: "SourceID"},
		{name: "plural acronym", id: "gnssDops", source: "GNSS DOPs", expected: "GNSSDOPs"},
		{name: "plural id", id: "numberOfIds", source: "Number of IDs", expected: "NumberOfIDs"},
		{name: "normal word", id: "mode", source: "Mode", expected: "Mode"},
		{name: "first prefix", id: "1stDeviceStatus", source: "1st Device Status", expected: "FirstDeviceStatus"},
		{
			name:     "hex range placeholder",
			id:       "0xe800-0xee00StandardizedSingleFrameAddressed",
			source:   "0xE800-0xEE00: Standardized single-frame addressed",
			expected: "ZeroXe8000Xee00StandardizedSingleFrameAddressed",
		},
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
	if name != "GNSMethodConst" {
		t.Fatalf("convertToConst(GNS_METHOD) = %q, want GNSMethodConst", name)
	}
}

func TestConvertToConstPreservesInitialisms(t *testing.T) {
	tests := []struct {
		raw      string
		expected string
	}{
		{raw: "AIS_MESSAGE_ID", expected: "AISMessageIDConst"},
		{raw: "ATON_TYPE", expected: "ATONTypeConst"},
		{raw: "PGN_ERROR_CODE", expected: "PGNErrorCodeConst"},
		{raw: "MOB_POSITION_SOURCE", expected: "MOBPositionSourceConst"},
		{raw: "NMEA_FUNCTION_CODE", expected: "NMEAFunctionCodeConst"},
		{raw: "GNS_INTEGRITY", expected: "GNSIntegrityConst"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			name := tt.raw
			convertToConst(&name)
			if name != tt.expected {
				t.Fatalf("convertToConst(%q) = %q, want %q", tt.raw, name, tt.expected)
			}
		})
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
