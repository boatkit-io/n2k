package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveBaselineDomainRangesCoversAllPQAndCurated(t *testing.T) {
	res025 := float32(0.25)
	conv := &canboatConverter{
		PhysicalUnits: []PhysicalUnit{
			{Name: "TIME", Unit: "s"},
			{Name: "DISTANCE", Unit: "m"},
			{Name: "ANGLE", Unit: "rad"},
			{Name: "CONCENTRATION", Unit: "ppm"},
		},
		PGNs: []*PGN{
			{
				Fields: []PGNField{
					{PhysicalQuantity: "TIME", Unit: "s", RangeMin: 0, RangeMax: 10, Signed: false},
					{PhysicalQuantity: "ANGLE", Unit: "rad", RangeMin: -3.14, RangeMax: 3.14, Signed: true},
					// max expressible (uint8) should become nil Max
					{PhysicalQuantity: "PRESSURE", Unit: "Pa", RangeMin: 0, RangeMax: 255, BitLength: 8, FieldType: "NUMBER", Signed: false},
					// canboat bug: maxValid-1 for len>=8 should also become nil Max
					{PhysicalQuantity: "ELECTRICAL_POWER", Unit: "W", RangeMin: 0, RangeMax: 65532, BitLength: 16, FieldType: "NUMBER", Signed: false},
					// scaled max expressible (speed, 0.25*65535) should become nil Max
					{PhysicalQuantity: "SPEED", Unit: "m/s", RangeMin: 0, RangeMax: 16383, BitLength: 16, FieldType: "NUMBER", Signed: false, Resolution: &res025},
					// unsigned zero min should become nil
					{PhysicalQuantity: "CONCENTRATION", Unit: "ppm", RangeMin: 0, RangeMax: 100, BitLength: 16, FieldType: "NUMBER", Signed: false},
				},
			},
		},
	}

	baseline := conv.deriveBaselineDomainRanges()

	// Curated TIME override should apply (0..86401)
	timeRange := baseline[domainKey{PhysicalQuantity: "TIME", Unit: "s", Signed: false}]
	if timeRange.Min == nil || *timeRange.Min != 0 || timeRange.Max == nil || *timeRange.Max != 86401 {
		t.Fatalf("expected TIME/s curated range 0..86401, got %+v", timeRange)
	}

	// Distance should exist even without field data, with curated nil max
	distRange, ok := baseline[domainKey{PhysicalQuantity: "DISTANCE", Unit: "m", Signed: false}]
	if !ok || distRange.Min == nil || *distRange.Min != 0 || distRange.Max != nil {
		t.Fatalf("expected DISTANCE/m 0..nil, got %+v", distRange)
	}

	// Angle should reflect canboat values
	angleRange, ok := baseline[domainKey{PhysicalQuantity: "ANGLE", Unit: "rad", Signed: true}]
	if !ok || angleRange.Min == nil || *angleRange.Min != -3.14 || angleRange.Max == nil || *angleRange.Max != 3.14 {
		t.Fatalf("expected ANGLE/rad -3.14..3.14, got %+v", angleRange)
	}

	pressureRange, ok := baseline[domainKey{PhysicalQuantity: "PRESSURE", Unit: "Pa", Signed: false}]
	if !ok || pressureRange.Max != nil {
		t.Fatalf("expected PRESSURE/Pa unsigned max nil due to expressible sentinel, got %+v", pressureRange)
	}

	powerRange, ok := baseline[domainKey{PhysicalQuantity: "ELECTRICAL_POWER", Unit: "W", Signed: false}]
	if !ok || powerRange.Max != nil {
		t.Fatalf("expected ELECTRICAL_POWER/W unsigned max nil due to canboat bug sentinel, got %+v", powerRange)
	}

	speedRange, ok := baseline[domainKey{PhysicalQuantity: "SPEED", Unit: "m/s", Signed: false}]
	if !ok || speedRange.Max != nil {
		t.Fatalf("expected SPEED/m/s unsigned max nil due to scaled expressible sentinel, got %+v", speedRange)
	}

	concRange, ok := baseline[domainKey{PhysicalQuantity: "CONCENTRATION", Unit: "ppm", Signed: false}]
	if !ok || concRange.Min != nil {
		t.Fatalf("expected CONCENTRATION/ppm unsigned min nil when canboat min is 0, got %+v", concRange)
	}
}

func TestDiffDomainTables(t *testing.T) {
	baseline := map[domainKey]domainRange{
		{"TIME", "s", false}:      {Min: float64Ptr(0), Max: float64Ptr(86401)},
		{"DISTANCE", "m", false}:  {Min: float64Ptr(0), Max: nil},
		{"ANGLE", "rad", true}:    {Min: float64Ptr(-3.14), Max: float64Ptr(3.14)},
		{"PRESSURE", "Pa", false}: {},
	}
	editable := map[domainKey]domainRange{
		{"TIME", "s", false}:     {Min: float64Ptr(0), Max: float64Ptr(86400)}, // changed max
		{"DISTANCE", "m", false}: {Min: float64Ptr(0), Max: nil},               // same
		{"TEMP", "K", false}:     {Min: float64Ptr(100), Max: float64Ptr(200)}, // added
	}

	diff := diffDomainTables(baseline, editable)

	assertContainsKey(t, diff.missing, domainKey{"ANGLE", "rad", true})
	assertContainsKey(t, diff.missing, domainKey{"PRESSURE", "Pa", false})
	assertContainsKey(t, diff.added, domainKey{"TEMP", "K", false})
	assertContainsKey(t, diff.changed, domainKey{"TIME", "s", false})
}

func TestWriteAndReadDomainRangesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domain_ranges.generated.yaml")
	table := map[domainKey]domainRange{
		{"TIME", "s", false}:     {Min: float64Ptr(0), Max: float64Ptr(86401)},
		{"DISTANCE", "m", false}: {Min: float64Ptr(0), Max: nil},
	}

	if err := writeDomainRangesYAML(path, table); err != nil {
		t.Fatalf("writeDomainRangesYAML failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	readTable, err := readDomainRangesYAML(path)
	if err != nil {
		t.Fatalf("readDomainRangesYAML failed: %v", err)
	}

	if !domainRangesEqual(table[domainKey{"TIME", "s", false}], readTable[domainKey{"TIME", "s", false}]) {
		t.Fatalf("TIME range mismatch after round-trip")
	}
	if !domainRangesEqual(table[domainKey{"DISTANCE", "m", false}], readTable[domainKey{"DISTANCE", "m", false}]) {
		t.Fatalf("DISTANCE range mismatch after round-trip")
	}
}

func assertContainsKey(t *testing.T, keys []domainKey, want domainKey) {
	t.Helper()
	for _, k := range keys {
		if k == want {
			return
		}
	}
	t.Fatalf("expected key %+v in list %+v", want, keys)
}
