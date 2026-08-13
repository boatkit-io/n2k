// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestMergePGNOverridesReplacesAndAddsDefinitions(t *testing.T) {
	manufacturer172 := 172
	converter := &canboatConverter{
		PGNs: []*PGN{
			testPGNDefinition("yanmarEngineDataA", 65280, 172, "upstream"),
			testPGNDefinition("hondaEngineAlerts", 65284, 175, "honda"),
		},
	}
	replacement := testPGNDefinition("yanmarEngineDataA", 65280, manufacturer172, "local")
	addition := testPGNDefinition("yanmarThrottleControl", 65284, manufacturer172, "local")

	replaced, added, err := converter.mergePGNOverrides([]*PGN{replacement, addition})
	if err != nil {
		t.Fatalf("mergePGNOverrides() error = %v", err)
	}
	if replaced != 1 || added != 1 {
		t.Fatalf("mergePGNOverrides() counts = (%d, %d), want (1, 1)", replaced, added)
	}
	if converter.PGNs[0] != replacement {
		t.Fatal("replacement did not retain the upstream definition's position")
	}
	if converter.PGNs[len(converter.PGNs)-1] != addition {
		t.Fatal("addition was not appended")
	}
}

func TestMergePGNOverridesRejectsDuplicateOverrideIDs(t *testing.T) {
	converter := &canboatConverter{}
	definition := testPGNDefinition("yanmarThrottleControl", 65284, 172, "local")

	_, _, err := converter.mergePGNOverrides([]*PGN{definition, definition})
	if err == nil || !strings.Contains(err.Error(), "duplicate PGN override ID") {
		t.Fatalf("mergePGNOverrides() error = %v, want duplicate ID error", err)
	}
}

func TestMergePGNOverridesRejectsUpstreamMatchCollision(t *testing.T) {
	converter := &canboatConverter{
		PGNs: []*PGN{testPGNDefinition("upstreamYanmarThrottle", 65284, 172, "upstream")},
	}
	override := testPGNDefinition("yanmarThrottleControl", 65284, 172, "local")

	_, _, err := converter.mergePGNOverrides([]*PGN{override})
	if err == nil || !strings.Contains(err.Error(), "conflicts with upstream definition") {
		t.Fatalf("mergePGNOverrides() error = %v, want upstream collision error", err)
	}
}

func TestMergePGNOverridesRejectsRedundantOverride(t *testing.T) {
	definition := testPGNDefinition("yanmarEngineDataA", 65280, 172, "same")
	converter := &canboatConverter{PGNs: []*PGN{definition}}
	override := testPGNDefinition("yanmarEngineDataA", 65280, 172, "same")

	_, _, err := converter.mergePGNOverrides([]*PGN{override})
	if err == nil || !strings.Contains(err.Error(), "identical to the upstream definition") {
		t.Fatalf("mergePGNOverrides() error = %v, want redundant override error", err)
	}
}

func TestApplyPGNMinLengthOverrides(t *testing.T) {
	converter := &canboatConverter{
		PGNs: []*PGN{
			{
				PGN:       129809,
				Id:        "aisClassBStaticDataMsg24PartA",
				Length:    27,
				MinLength: 0,
				Fields: []PGNField{
					{Id: "name", BitOffset: 40},
					{Id: "aisTransceiverInformation", BitOffset: 200},
				},
			},
		},
	}

	applied, err := converter.applyPGNMinLengthOverrides(map[string]uint32{
		"aisClassBStaticDataMsg24PartA": 25,
	})
	if err != nil {
		t.Fatalf("applyPGNMinLengthOverrides() error = %v", err)
	}
	if applied != 1 {
		t.Fatalf("applyPGNMinLengthOverrides() applied = %d, want 1", applied)
	}
	if got := converter.PGNs[0].MinLength; got != 25 {
		t.Fatalf("MinLength = %d, want 25", got)
	}
}

func TestApplyPGNMinLengthOverridesRejectsNonBoundary(t *testing.T) {
	converter := &canboatConverter{
		PGNs: []*PGN{{
			PGN:    129809,
			Id:     "aisClassBStaticDataMsg24PartA",
			Length: 27,
			Fields: []PGNField{{Id: "aisTransceiverInformation", BitOffset: 200}},
		}},
	}

	_, err := converter.applyPGNMinLengthOverrides(map[string]uint32{
		"aisClassBStaticDataMsg24PartA": 24,
	})
	if err == nil || !strings.Contains(err.Error(), "does not end on a field boundary") {
		t.Fatalf("applyPGNMinLengthOverrides() error = %v, want field-boundary error", err)
	}
}

func testPGNDefinition(id string, number uint32, manufacturer int, description string) *PGN {
	resolution := float32(1)
	return &PGN{
		PGN:         number,
		Id:          id,
		Description: description,
		Type:        "Single",
		Length:      8,
		FieldCount:  1,
		Fields: []PGNField{
			{
				Order:      1,
				Id:         "manufacturerCode",
				Name:       "Manufacturer Code",
				BitLength:  11,
				BitOffset:  0,
				FieldType:  "LOOKUP",
				LookupName: "MANUFACTURER_CODE",
				Resolution: &resolution,
				Match:      &manufacturer,
			},
		},
	}
}
