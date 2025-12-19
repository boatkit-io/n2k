package main

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Domain ranges YAML file paths used by pgngen.
const (
	// editableDomainRangesPath is the user-maintained domain ranges table.
	editableDomainRangesPath = "cmd/pgngen/domain_ranges.yaml"
	// generatedDomainRangesPath is the derived baseline domain ranges table.
	generatedDomainRangesPath = "cmd/pgngen/domain_ranges.generated.yaml"
)

// domainKey identifies a PhysicalQuantity + Unit pair.
type domainKey struct {
	PhysicalQuantity string
	Unit             string
	Signed           bool
}

// lessDomainKey orders domainKey values deterministically for stable YAML output.
func lessDomainKey(a, b domainKey) bool {
	if a.PhysicalQuantity == b.PhysicalQuantity {
		if a.Unit == b.Unit {
			if a.Signed == b.Signed {
				return false
			}
			return !a.Signed && b.Signed
		}
		return a.Unit < b.Unit
	}
	return a.PhysicalQuantity < b.PhysicalQuantity
}

// domainRange captures optional domain constraints for a PQ+Unit combo.
type domainRange struct {
	Min *float64 `yaml:"min"`
	Max *float64 `yaml:"max"`
}

// domainRangeEntry is the YAML row format.
type domainRangeEntry struct {
	PhysicalQuantity string      `yaml:"physicalQuantity"`
	Unit             string      `yaml:"unit"`
	Signed           bool        `yaml:"signed"`
	Min              interface{} `yaml:"min"` // allow null or number for YAML output
	Max              interface{} `yaml:"max"` // allow null or number for YAML output
	Notes            string      `yaml:"notes,omitempty"`
}

// domainDiagnostics collects visibility into gaps or inconsistencies.
type domainDiagnostics struct {
	missing []domainKey // present in baseline, absent in editable
	added   []domainKey // present in editable, absent in baseline
	changed []domainKey // same key, differing min/max
}

// curatedPQUnitDefaults are deterministic baseline values applied when canboat is missing or incorrect.
var curatedPQUnitDefaults = map[domainKey]domainRange{
	{PhysicalQuantity: "TIME", Unit: "s", Signed: false}: {
		Min: float64Ptr(0),
		Max: float64Ptr(86401),
	},
	{PhysicalQuantity: "GEOGRAPHICAL_LATITUDE", Unit: "deg", Signed: true}: {
		Min: float64Ptr(-90),
		Max: float64Ptr(90),
	},
	{PhysicalQuantity: "GEOGRAPHICAL_LONGITUDE", Unit: "deg", Signed: true}: {
		Min: float64Ptr(-180),
		Max: float64Ptr(180),
	},
	{PhysicalQuantity: "DISTANCE", Unit: "m", Signed: false}: {
		Min: float64Ptr(0),
		Max: nil, // use max valid raw value
	},
}

// curatedFieldOverrides are field-ID specific domain corrections (applied before PQ+Unit mapping).
var curatedFieldOverrides = map[string]domainRange{}

// float64Ptr allocates a float64 pointer.
func float64Ptr(v float64) *float64 {
	return &v
}

// buildDomainRanges loads the editable YAML table, writing/refreshing a generated baseline as needed.
// It applies the editable values to the converter and reports drift from canboat-derived baseline.
func (conv *canboatConverter) buildDomainRanges() {
	baseline := conv.deriveBaselineDomainRanges()
	ensureDir(filepath.Dir(generatedDomainRangesPath))
	if err := writeDomainRangesYAML(generatedDomainRangesPath, baseline); err != nil {
		log.Fatalf("failed to write generated domain ranges: %v", err)
	}

	editable, err := readDomainRangesYAML(editableDomainRangesPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Fatalf("domain ranges file %s not found; copy %s and edit it", editableDomainRangesPath, generatedDomainRangesPath)
		}
		log.Fatalf("failed to read %s: %v", editableDomainRangesPath, err)
	}

	diff := diffDomainTables(baseline, editable)
	conv.domainDiagnostics = domainDiagnostics(diff)
	conv.logDomainRangeDiagnostics()

	conv.domainRanges = editable
}

// deriveBaselineDomainRanges collects PQ/Unit ranges from canboat plus curated defaults, and ensures coverage for all PhysicalQuantities.
func (conv *canboatConverter) deriveBaselineDomainRanges() map[domainKey]domainRange {
	table := make(map[domainKey]domainRange)

	// Seed from canboat fields
	for _, pgn := range conv.PGNs {
		for i := range pgn.Fields {
			field := pgn.Fields[i]
			if field.PhysicalQuantity == "" || field.Unit == "" {
				continue
			}
			key := domainKey{PhysicalQuantity: field.PhysicalQuantity, Unit: field.Unit, Signed: field.Signed}

			candidate := domainRange{
				Min: func() *float64 {
					if !field.Signed && field.RangeMin == 0 {
						return nil
					}
					return float64Ptr(field.RangeMin)
				}(),
			}
			if !isCanboatMaxExpressible(field) && field.RangeMax != 0 {
				candidate.Max = float64Ptr(field.RangeMax)
			}

			if _, ok := table[key]; ok {
				// Keep the first; conflicts are handled downstream by editable diffs.
				continue
			}
			table[key] = candidate
		}
	}

	// Apply curated defaults
	for key, rng := range curatedPQUnitDefaults {
		table[key] = rng
	}

	// Ensure every defined PhysicalQuantity/Unit pair exists.
	for _, pq := range conv.PhysicalUnits {
		key := domainKey{PhysicalQuantity: pq.Name, Unit: pq.Unit, Signed: false}
		if _, ok := table[key]; !ok {
			table[key] = domainRange{}
		}
	}

	return table
}

// fixDomainLimits applies field-level and PQ+Unit domain mappings, then falls back to special cases.
func (conv *canboatConverter) fixDomainLimits(field *PGNField) {
	if override, ok := curatedFieldOverrides[field.Id]; ok {
		applyDomainRange(field, override)
	}

	if rng, ok := conv.domainRanges[domainKey{PhysicalQuantity: field.PhysicalQuantity, Unit: field.Unit}]; ok {
		applyDomainRange(field, rng)
	}

	// Fallback hard-coded domain constraints for known fields.
	switch field.Id {
	case "Latitude":
		if field.DomainMin == nil {
			field.DomainMin = float64Ptr(-90.0)
		}
		if field.DomainMax == nil {
			field.DomainMax = float64Ptr(90.0)
		}
	case "Longitude":
		if field.DomainMin == nil {
			field.DomainMin = float64Ptr(-180.0)
		}
		if field.DomainMax == nil {
			field.DomainMax = float64Ptr(180.0)
		}
	}
}

// applyDomainRange applies non-nil min/max overrides onto field domain constraints.
func applyDomainRange(field *PGNField, rng domainRange) {
	if rng.Min != nil {
		field.DomainMin = rng.Min
	}
	if rng.Max != nil {
		field.DomainMax = rng.Max
	}
}

// domainRangesEqual compares two domainRange values including nil-vs-non-nil.
func domainRangesEqual(a, b domainRange) bool {
	return float64PtrsEqual(a.Min, b.Min) && float64PtrsEqual(a.Max, b.Max)
}

// float64PtrsEqual reports whether two float64 pointers are both nil or both equal values.
func float64PtrsEqual(a, b *float64) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

// isCanboatMaxExpressible detects cases where canboat's RangeMax is just the max-expressible (or the known canboat bug variant) scaled by resolution.
// These should be treated as unknown (nil) so a curated/domain value can replace them.
func isCanboatMaxExpressible(field PGNField) bool {
	if !reservedNumericType(field.FieldType) {
		return false
	}
	if field.RangeMax == 0 {
		return false
	}

	resolution := 1.0
	if field.Resolution != nil {
		resolution = float64(*field.Resolution)
	}
	offset := float64(field.Offset)

	approx := func(a, b float64) bool {
		tol := resolution / 2
		if tol < 1e-6 {
			tol = 1e-6
		}
		return math.Abs(a-b) <= tol
	}

	maxRaw := float64(calcMaxRawValue(field))*resolution + offset
	if approx(field.RangeMax, maxRaw) {
		return true
	}

	maxValid := calcMaxValidRawValue(field)
	if field.BitLength >= 8 && maxValid > 0 {
		// canboat bug: uses maxValid-1 for len>=8 (effectively assuming 3 reserved)
		bug := float64(maxValid-1)*resolution + offset
		if approx(field.RangeMax, bug) {
			return true
		}
	}

	return false
}

// formatDomainKeyList formats a list of domain keys for logging.
func formatDomainKeyList(keys []domainKey) string {
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = fmt.Sprintf("%s/%s/signed=%t", key.PhysicalQuantity, key.Unit, key.Signed)
	}
	return strings.Join(parts, ", ")
}

// logDomainRangeDiagnostics logs differences between baseline and editable domain tables.
func (conv *canboatConverter) logDomainRangeDiagnostics() {
	if len(conv.domainDiagnostics.missing) > 0 {
		log.Warnf("Domain ranges missing from editable file for %d PQ/Unit combos: %s", len(conv.domainDiagnostics.missing), formatDomainKeyList(conv.domainDiagnostics.missing))
	}
	if len(conv.domainDiagnostics.added) > 0 {
		log.Warnf("Domain ranges in editable file not present in baseline for %d PQ/Unit combos: %s", len(conv.domainDiagnostics.added), formatDomainKeyList(conv.domainDiagnostics.added))
	}
	if len(conv.domainDiagnostics.changed) > 0 {
		log.Infof("Domain ranges differ from baseline for %d PQ/Unit combos: %s", len(conv.domainDiagnostics.changed), formatDomainKeyList(conv.domainDiagnostics.changed))
	}
}

// writeDomainRangesYAML serializes a domain table to YAML at path.
func writeDomainRangesYAML(path string, table map[domainKey]domainRange) error {
	entries := mapToEntries(table)
	data, err := yaml.Marshal(entries)
	if err != nil {
		return err
	}

	// Avoid rewriting identical content.
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, data) {
			return nil
		}
	}
	return os.WriteFile(path, data, 0644)
}

// readDomainRangesYAML parses a YAML domain table from path.
func readDomainRangesYAML(path string) (map[domainKey]domainRange, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []domainRangeEntry
	if err := yaml.Unmarshal(content, &entries); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("domain ranges file is empty")
	}

	result := make(map[domainKey]domainRange)
	for _, e := range entries {
		if e.PhysicalQuantity == "" || e.Unit == "" {
			return nil, fmt.Errorf("invalid entry: missing PhysicalQuantity or Unit")
		}
		key := domainKey{PhysicalQuantity: e.PhysicalQuantity, Unit: e.Unit, Signed: e.Signed}
		rng := domainRange{
			Min: interfaceToFloat64Ptr(e.Min),
			Max: interfaceToFloat64Ptr(e.Max),
		}
		result[key] = rng
	}
	return result, nil
}

// mapToEntries converts an internal domain table into a sorted YAML entry list.
func mapToEntries(table map[domainKey]domainRange) []domainRangeEntry {
	keys := make([]domainKey, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return lessDomainKey(keys[i], keys[j]) })

	entries := make([]domainRangeEntry, 0, len(keys))
	for _, k := range keys {
		rng := table[k]
		entries = append(entries, domainRangeEntry{
			PhysicalQuantity: k.PhysicalQuantity,
			Unit:             k.Unit,
			Signed:           k.Signed,
			Min:              floatPtrToInterface(rng.Min),
			Max:              floatPtrToInterface(rng.Max),
		})
	}
	return entries
}

// interfaceToFloat64Ptr converts YAML-decoded scalars (or nil) to a *float64.
func interfaceToFloat64Ptr(v interface{}) *float64 {
	switch t := v.(type) {
	case nil:
		return nil
	case float64:
		return &t
	case int:
		f := float64(t)
		return &f
	case int64:
		f := float64(t)
		return &f
	case float32:
		f := float64(t)
		return &f
	default:
		return nil
	}
}

// floatPtrToInterface converts an optional float64 pointer into a YAML scalar (or nil).
func floatPtrToInterface(v *float64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// domainDiff describes differences between two domain tables.
type domainDiff struct {
	missing []domainKey
	added   []domainKey
	changed []domainKey
}

// diffDomainTables compares baseline and editable domain tables and returns the differences.
func diffDomainTables(baseline, editable map[domainKey]domainRange) domainDiff {
	missing := make([]domainKey, 0)
	added := make([]domainKey, 0)
	changed := make([]domainKey, 0)

	for k, base := range baseline {
		if ed, ok := editable[k]; !ok {
			missing = append(missing, k)
		} else if !domainRangesEqual(base, ed) {
			changed = append(changed, k)
		}
	}
	for k := range editable {
		if _, ok := baseline[k]; !ok {
			added = append(added, k)
		}
	}

	sortKeys := func(keys []domainKey) []domainKey {
		sort.Slice(keys, func(i, j int) bool { return lessDomainKey(keys[i], keys[j]) })
		return keys
	}

	return domainDiff{
		missing: sortKeys(missing),
		added:   sortKeys(added),
		changed: sortKeys(changed),
	}
}

// ensureDir creates path (and parents) if it is non-empty, or fatals on error.
func ensureDir(path string) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		log.Fatalf("failed to create directory %s: %v", path, err)
	}
}
