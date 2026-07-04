package main

import "testing"

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
