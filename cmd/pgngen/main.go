package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"

	//	"math"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/Masterminds/sprig/v3"
	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// resolution64BitCutoff is a heuristic for a cutoff to jump from float32 -> float64 for uint32->float conversion
const resolution64BitCutoff = 0.0000001

// MaxPGNLength is the maximum length of a PGN in bytes
const MaxPGNLength = 223 // 31*7 + 6

// log provides standard logging capability to the program.
var log = logrus.StandardLogger()

func main() {
	fmt.Println("Entered Main")
	builder := newCanboatConverter()
	builder.fixup()
	builder.filter()
	builder.validate()
	builder.write()

}

// canboatConverter is inflated from the json file canboat.json.
// The data is massaged and used to generate the output file.
// We filter PGNs that have never been seen into a separate list. If one is encountered we'll log it and its data, and return an UnknownPGN to
// allow processing to continue.
type canboatConverter struct {
	Comment        string
	CreatorCode    string
	License        string
	Version        string
	PhysicalUnits  []PhysicalUnit              `json:"PhysicalQuantities"`
	BitEnums       []BitEnumeration            `json:"LookupBitEnumerations"`
	Enums          []LookupEnumeration         `json:"LookupEnumerations"`
	IndirectEnums  []LookupIndirectEnumeration `json:"LookupIndirectEnumerations"`
	FieldTypeEnums []FieldTypeEnumeration      `json:"LookupFieldTypeEnumerations"`
	PGNs           []*PGN
	NeverSeenPGNs  []*PGN
	IncompletePGNS []*PGN
}

// PhysicalUnit, defined in Canboat.json, defines the units used by Canboat.
type PhysicalUnit struct {
	Name            string
	Description     string
	UnitDescription string
	Unit            string
}

// LookupEnumeration instances contain name/value pairs for constants used by NMEA data objects
type LookupEnumeration struct {
	Name     string
	MaxValue int
	Values   []EnumPair `json:"EnumValues"`
}

// LookupIndirectEnumeration instances contain name/value/value tuplets.
// It's used where the value is indexed first by device type, then by attribute.
type LookupIndirectEnumeration struct {
	Name     string
	MaxValue int
	Values   []EnumTriplet `json:"EnumValues"`
}

// EnumTriplet is used as elements of a LookupIndirectEnumeration
type EnumTriplet struct {
	Text   string `json:"Name"`
	Value1 int
	Value2 int
}

// EnumPair is used as elements of LookupEnumerations.
type EnumPair struct {
	Name  string // The generated name for the go const
	Text  string `json:"Name"`
	Value int
}

// BitEnumeration instances contain pairs of bit offsets and names.
type BitEnumeration struct {
	Name          string
	MaxValue      int
	EnumBitValues []BitEnumPair
}

// BitEnumPair is an element of a BitEnumeration
type BitEnumPair struct {
	Name  string // The generated name for the go const
	Label string `json:"Name"`
	Bit   int
}

// FieldTypeEnumeration instances contain a list of EnumFieldTypes for a given PGN.
type FieldTypeEnumeration struct {
	Name                string
	MaxValue            int
	EnumFieldTypeValues []EnumFieldType
}

// EnumFieldType contains the possible fields found in a FieldTypeEnumeration
type EnumFieldType struct {
	Name          string
	Value         uint32
	FieldType     string
	Resolution    float32
	Unit          string
	BitLength     uint8
	BitLookupName string `json:"LookupBitEnumeration"`
}

// PGN is the core data structure describing a NMEA message.
type PGN struct {
	PGN                          uint32
	Id                           string
	Description                  string
	Explanation                  string
	Type                         string
	Complete                     bool
	Missing                      []string
	FieldCount                   uint8
	Length                       uint32
	MinLength                    uint32
	TransmissionIrregular        bool
	BitLengthField               uint8
	RepeatingFieldSet1Size       uint8
	RepeatingFieldSet1StartField uint8
	RepeatingFieldSet1CountField uint8
	RepeatingFieldSet2Size       uint8
	RepeatingFieldSet2StartField uint8
	RepeatingFieldSet2CountField uint8
	Fields                       []PGNField
	FieldsRepeating1             []PGNField
	FieldsRepeating2             []PGNField
	AllFields                    []PGNField
}

// PGNField describes an individual field in a PGN.
type PGNField struct {
	Order                    uint8
	Id                       string
	Name                     string
	Description              string
	BitLength                uint16
	BitLengthVariable        bool
	BitLengthField           uint8
	BitOffset                uint16
	BitStart                 uint8
	FieldType                string
	Resolution               *float32
	Offset                   int64
	RangeMin                 float64
	RangeMax                 float64
	DomainMin                float64
	DomainMax                float64
	Match                    *int
	Signed                   bool
	Unit                     string
	LookupName               string `json:"LookupEnumeration"`
	BitLookupName            string `json:"LookupBitEnumeration"`
	IndirectLookupName       string `json:"LookupIndirectEnumeration"`
	IndirectLookupFieldOrder uint8  `json:"LookupIndirectEnumerationFieldOrder"`
	FieldTypeLookupName      string `json:"LookupFieldTypeEnumeration"`
}

// newCanboatConverter instantiates a new converter
func newCanboatConverter() *canboatConverter {
	c := new(canboatConverter)
	c.init()
	return c
}

// init initializes a canboatConverter from canboat.json.
func (conv *canboatConverter) init() {
	raw, _ := loadCachedWebContent("canboatjson", "https://raw.githubusercontent.com/canboat/canboat/master/docs/canboat.json")
	err := json.Unmarshal(raw, conv)
	if err != nil {
		log.Info(err)
	}
	log.Infof("Initially Parsed Lookup enums: %d", len(conv.Enums))
	log.Infof("Initially Parsed IndirectLookup enums: %d", len(conv.IndirectEnums))
	log.Infof("Initially Parsed BitLookup enums: %d", len(conv.BitEnums))
	log.Infof("Initially Parsed FieldTypeLookup enums: %d", len(conv.FieldTypeEnums))
	log.Infof("Parsed pgns: %d", len(conv.PGNs))
}

// fixup massages the imported data (details in the routines it invokes).
func (conv *canboatConverter) fixup() {
	conv.fixIDs()
	conv.fixEnumDefs()
	conv.fixRepeating()
}

// you builds separate slices for known, unknown (no Canboat samples), and incomplete PGNs
func (conv *canboatConverter) filter() {
	known := make([]*PGN, 0)
	unknown := make([]*PGN, 0)
	incomplete := make([]*PGN, 0)
	var keep bool
	for _, pgn := range conv.PGNs {
		keep = true
		if !pgn.Complete {
			if len(pgn.Missing) == 0 {
				panic("Complete is false but Missing is empty!")
			}
			for i := range pgn.Missing {
				if (pgn.Missing[i] == "Interval") || (pgn.Missing[i] == "Lookups") { // we don't care about these
					continue
				}
				keep = false
				if pgn.Missing[i] == "SampleData" { // we track these so we can provide sample data if we encounter them
					unknown = append(unknown, pgn)
				}
			}
		}
		if keep {
			known = append(known, pgn)
		} else {
			incomplete = append(incomplete, pgn)
		}
	}
	conv.PGNs = known
	conv.NeverSeenPGNs = unknown
	conv.IncompletePGNS = incomplete
	log.Infof("After filtering, known: %d, unknown: %d, incomplete: %d", len(conv.PGNs), len(conv.NeverSeenPGNs), len(conv.IncompletePGNS))
}

// write outputs the generated files. Most of the work occurs in the templates.
func (conv *canboatConverter) write() {
	// Get the project root directory
	projectRoot, err := filepath.Abs(".")
	if err != nil {
		panic(err)
	}

	// Create output directories
	internalPGNDir := filepath.Join(projectRoot, "internal", "pgn")
	publicN2kDir := filepath.Join(projectRoot, "pkg", "n2k")
	filterrawDir := filepath.Join(projectRoot, "cmd", "filterraw")

	if err := os.MkdirAll(internalPGNDir, 0755); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(publicN2kDir, 0755); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filterrawDir, 0755); err != nil {
		panic(err)
	}

	// Template function map
	funcMap := template.FuncMap{
		"convertFieldType":          convertFieldType,
		"getFieldSerializer":        getFieldSerializer,
		"getFieldDeserializer":      getFieldDeserializer,
		"fieldByteCount":            fieldByteCount,
		"concat":                    func(strs ...string) string { return strings.Join(strs, "") },
		"toNumber":                  toNumber,
		"isPointerFieldType":        isPointerFieldType,
		"constSize":                 constSize,
		"subtract":                  func(x, y uint8) uint8 { return x - y },
		"matchManufacturer":         matchManufacturer,
		"makeIndirectMap":           makeIndirectMap,
		"getReservedValueCount":     getReservedValueCount,
		"generateFieldSpecConstant": generateFieldSpecConstant,
		"getMaxRawValue":            calcMaxRawValue,
		"getMissingValue":           calcMissingValue,
		"calcMaxValidRawValue":      calcMaxValidRawValue,
		"calcMissingValue":          calcMissingValue,
		"derefOrZero": func(v *uint64) uint64 {
			if v == nil {
				return 0
			} else {
				return *v
			}
		},
		"derefInt":            func(ip *int) int { return *ip },
		"isNil":               func(fp *int) bool { return fp == nil },
		"contains":            func(in, substr string) bool { return strings.Contains(in, substr) },
		"hasMultipleVariants": hasMultipleVariants,
		"getVariantsForPgn":   getVariantsForPgn,
		"getDecoderConfig":    getDecoderConfig,
		"groupByPGN":          groupByPGN,
	}

	// Generate fast bits array
	fastBits := conv.generateFastBits()

	// Template data
	templateData := struct {
		PGNDoc   any
		ForDebug bool
		FastBits []byte
	}{
		PGNDoc:   conv,
		ForDebug: false,
		FastBits: fastBits,
	}

	// Generate internal PGN files
	internalTemplates := map[string]string{
<<<<<<< Updated upstream
		"types_generated.go":          "runtime/types.go.tmpl",
		"enums_generated.go":          "runtime/enums.go.tmpl",
		"consts_generated.go":         "runtime/consts.go.tmpl",
		"methods_generated.go":        "runtime/methods.go.tmpl",
=======
		"types_generated.go":   "runtime/types.go.tmpl",
		"enums_generated.go":   "runtime/enums.go.tmpl",
		"consts_generated.go":  "runtime/consts.go.tmpl",
		"methods_generated.go": "runtime/methods.go.tmpl",
		// "pgninfo_generated.go":        "runtime/pgninfo.go.tmpl", // Moved to cmd/filterraw
>>>>>>> Stashed changes
		"decoders_generated.go":       "runtime/decoders.go.tmpl",
		"encoders_generated.go":       "runtime/encoders.go.tmpl",
		"discriminators_generated.go": "runtime/discriminators.go.tmpl",
		"fieldspecs_generated.go":     "runtime/fieldspecs.go.tmpl",
		"fastbits_generated.go":       "runtime/fastbits.go.tmpl",
	}

	for filename, templatePath := range internalTemplates {
		fmt.Printf("Generating internal file: %s from template: %s\n", filename, templatePath)
		if err := conv.generateFile(filepath.Join(internalPGNDir, filename), templatePath, funcMap, templateData); err != nil {
			fmt.Printf("Error generating %s: %v\n", filename, err)
			panic(err)
		}
	}

	// Generate filterraw files
	filterrawTemplates := map[string]string{
		"pgn_data.go": "filterraw/pgn_data.go.tmpl",
	}

	for filename, templatePath := range filterrawTemplates {
		fmt.Printf("Generating filterraw file: %s from template: %s\n", filename, templatePath)
		if err := conv.generateFile(filepath.Join(filterrawDir, filename), templatePath, funcMap, templateData); err != nil {
			fmt.Printf("Error generating %s: %v\n", filename, err)
			panic(err)
		}
	}

	// Generate public API type aliases
	publicTemplates := map[string]string{
		"types_generated.go":  "public/types.go.tmpl",
		"enums_generated.go":  "public/enums.go.tmpl",
		"consts_generated.go": "public/consts.go.tmpl",
	}

	for filename, templatePath := range publicTemplates {
		fmt.Printf("Generating public file: %s from template: %s\n", filename, templatePath)
		if err := conv.generateFile(filepath.Join(publicN2kDir, filename), templatePath, funcMap, templateData); err != nil {
			fmt.Printf("Error generating %s: %v\n", filename, err)
			panic(err)
		}
	}

}

// generateFile generates a single file from a template
func (conv *canboatConverter) generateFile(outputPath, templatePath string, funcMap template.FuncMap, data interface{}) error {
	templateFullPath := filepath.Join("cmd/pgngen/templates", templatePath)
	fmt.Printf("  Reading template: %s\n", templateFullPath)
	// Read template file
	templateContent, err := os.ReadFile(templateFullPath)
	if err != nil {
		return err
	}

	// Create template
	t := template.Must(template.New(templatePath).Funcs(sprig.TxtFuncMap()).Funcs(funcMap).Parse(string(templateContent)))

	fmt.Printf("  Creating output file: %s\n", outputPath)
	// Create output file
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Execute template
	if err := t.Execute(f, data); err != nil {
		return err
	}

	fmt.Printf("  Successfully generated: %s\n", outputPath)
	return nil
}

// reservedNumericType returns true if the field type has reserved values.
func reservedNumericType(fieldType string) bool {
	// MMSI should be treated as string
	// DECIMAL is BCD, but no reserved values are specified
	return fieldType == "NUMBER" || fieldType == "DATE" || fieldType == "TIME" || fieldType == "PGN" || fieldType == "ISO_NAME" || fieldType == "DURATION" || fieldType == "DYNAMIC_FIELD_KEY" || fieldType == "DYNAMIC_FIELD_LENGTH"
}

// calcMaxRawValue calculates the maximum raw value for a field.
func calcMaxRawValue(field PGNField) uint64 {
	if field.BitLength == 0 { // only possible if no bitLength is specified in canboat.json
		return 0
	}
	if !reservedNumericType(field.FieldType) {
		return 0
	}
	maxRawVal := uint64(0xFFFFFFFFFFFFFFFF)
	maxRawVal >>= 64 - field.BitLength // the largest value representable in length of field
	if field.Signed {                  // high bit set means it's negative, so maximum positive value is 1 bit shorter
		maxRawVal >>= 1 // clears the high bit since we need the maximum positive value
	}
	return maxRawVal
}

// calcMaxValidRawValue calculates the maximum valid raw value accounting for reserved values
func calcMaxValidRawValue(field PGNField) uint64 {
	if field.BitLength == 0 || !reservedNumericType(field.FieldType) {
		return 0
	}

	reservedCount := getReservedValueCount(field)
	maxRaw := calcMaxRawValue(field)

	if reservedCount > 0 {
		maxRaw -= uint64(reservedCount)
	}

	return maxRaw
}

// calcMissingValue calculates the sentinel value representing missing (nil) data
func calcMissingValue(field PGNField) uint64 {
	reservedCount := getReservedValueCount(field)
	if reservedCount == 0 {
		// No reserved values means we can't represent missing - return 0 as safe default
		return 0
	}

	missing := uint64(0xFFFFFFFFFFFFFFFF)
	missing >>= 64 - field.BitLength // the largest value representable in length of field if unsigned
	if field.Signed {                // high bit set means it's negative, so maximum positive value is 1 bit shorter
		missing >>= 1 // missing flag is max positive value; negative value has high bit set
	}
	return missing
}

// needsScaling returns true if the field requires resolution/offset processing
func needsScaling(field PGNField) bool {
	hasResolution := field.Resolution != nil && *field.Resolution != 1.0
	hasOffset := field.Offset != 0
	return hasResolution || hasOffset
}

// generateFieldSpecConstant creates a FieldSpec constant definition
func generateFieldSpecConstant(pgn PGN, field PGNField) string {
	if !reservedNumericType(field.FieldType) {
		return "" // Only generate specs for numeric/float fields
	}

	reservedCount := getReservedValueCount(field)
	maxRawValue := calcMaxValidRawValue(field)
	missingValue := calcMissingValue(field)

	resolution := 1.0
	if field.Resolution != nil {
		resolution = float64(*field.Resolution)
	}

	// Handle domain constraints if available
	domainMin := "nil"
	domainMax := "nil"
	if field.DomainMin > 0 {
		domainMin = fmt.Sprintf("&[]float64{%g}[0]", field.DomainMin)
	}
	if field.DomainMax > 0 {
		domainMax = fmt.Sprintf("&[]float64{%g}[0]", field.DomainMax)
	}

	return fmt.Sprintf(`&FieldSpec{
		BitLength:     %d,
		BitOffset:     %d,
		MaxRawValue:   0x%X,
		MissingValue:  0x%X,
		Resolution:    %g,
		Offset:        %d,
		IsSigned:      %t,
		ReservedCount: %d,
		DomainMin:     %s,
		DomainMax:     %s,
	}`, field.BitLength, field.BitOffset, maxRawValue, missingValue, resolution, field.Offset, field.Signed, reservedCount, domainMin, domainMax)
}

// getReservedValueCount returns the number of reserved values at the top of a field's range.
// This is based purely on NMEA 2000 protocol standards, not domain constraints.
// Used by template.
func getReservedValueCount(field PGNField) uint8 {
	if !reservedNumericType(field.FieldType) {
		return 0
	}

	if field.BitLength == 0 { // only possible if no bitLength is specified in canboat.json
		return 0
	}

	// Use NMEA 2000 standard logic:
	// - Most fields reserve 1-2 values for "missing" and "invalid"
	// - Length >= 4 bits typically reserves 2 values
	// - Length < 4 bits typically reserves 1 value
	if field.BitLength >= 4 {
		return 2 // Reserve top 2 values (missing=0xFF..FF, invalid=0xFF..FE)
	} else {
		return 1 // Reserve top 1 value (missing=max value)
	}
}

// fixIDs uppercases the first letter of PGN Ids and assures names are unique.
// It then invokes a function to fixup each field.
func (conv *canboatConverter) fixIDs() {
	pgnDeDuper := NewDeDuper()
	for i := range conv.PGNs {
		fieldDeDuper := NewDeDuper()
		// Capitalize first letter of the Ids (currently lowercase)
		conv.PGNs[i].Id = capitalizeFirstChar(conv.PGNs[i].Id)
		if firstTime, _ := pgnDeDuper.unique(conv.PGNs[i].Id); !firstTime {
			panic("PGN ID not unique: " + conv.PGNs[i].Id)
		}
		for j := range conv.PGNs[i].Fields {
			fixField(&conv.PGNs[i].Fields[j], *fieldDeDuper)
			// For variable length fields, the length of such a field is passed
			// in another field. When we find such a field we keep the value in
			// the PGN so it can be used by the decoder for that PGN.
			if conv.PGNs[i].Fields[j].BitLengthField != 0 {
				conv.PGNs[i].BitLengthField = conv.PGNs[i].Fields[j].BitLengthField
			}
		}
	}
}

// fixDomainLimits adds domain limits to fields that have them
func fixDomainLimits(field *PGNField) {
	// For specific fields, add domain-specific validation
	switch field.Id {
	case "Latitude":
		field.DomainMin = float64(-90.0)
		field.DomainMax = float64(90.0)
	case "Longitude":
		field.DomainMin = float64(-180.0)
		field.DomainMax = float64(180.0)
	case "NumberOfBitsInBinaryDataField":
		field.DomainMin = float64(0.0)
		field.DomainMax = float64(MaxPGNLength*8) - float64(field.BitOffset+field.BitLength) // following binary data field starts at bitOffset+bitLength
	}
}

// fixField capitializes Id's first char, assures field name is unique, and forces lookup names.
func fixField(field *PGNField, dedup DeDuper) {
	field.Id = capitalizeFirstChar(field.Id)
	if len(field.FieldTypeLookupName) > 0 {
		convertToConst(&field.FieldTypeLookupName)
	}
	if field.FieldType == "LOOKUP" && len(field.LookupName) == 0 {
		log.Infof("Lookup without Enumeration Name: %s", field.Id)
	}
	if firstTime, _ := dedup.unique(field.Id); !firstTime {
		panic("field ID not unique: " + field.Id)
	}
	if len(field.LookupName) > 0 {
		convertToConst(&field.LookupName)
	}
	if len(field.BitLookupName) > 0 {
		convertToConst(&field.BitLookupName)
	}
	if len(field.IndirectLookupName) > 0 {
		convertToConst(&field.IndirectLookupName)
	}
	fixDomainLimits(field)
}

// fixRepeating identifies the range of repeating fields and extracts them to their own slice(s).
func (builder *canboatConverter) fixRepeating() {
	for i := range builder.PGNs {
		pgn := builder.PGNs[i]
		pgn.AllFields = make([]PGNField, 0)
		pgn.AllFields = append(pgn.AllFields, pgn.Fields...)
		if pgn.RepeatingFieldSet2Size > 0 { // work back from the end
			pgn.FieldsRepeating2 = pgn.Fields[pgn.RepeatingFieldSet2StartField-1 : pgn.RepeatingFieldSet2StartField-1+pgn.RepeatingFieldSet2Size]
			pgn.Fields = pgn.Fields[0 : pgn.RepeatingFieldSet2StartField-1]
		} else {
			pgn.FieldsRepeating2 = []PGNField{}
		}
		if pgn.RepeatingFieldSet1Size > 0 {
			startField := pgn.RepeatingFieldSet1StartField - 1
			endField := startField + pgn.RepeatingFieldSet1Size
			pgn.FieldsRepeating1 = pgn.Fields[startField:endField]
			pgn.Fields = pgn.Fields[0:startField]
		} else {
			pgn.FieldsRepeating1 = []PGNField{}
		}
	}
	builder.zeroBitOffsets()

}

// fixEnumDefs checks that enum names are unique and makes them legal golang identifiers.
func (builder *canboatConverter) fixEnumDefs() {
	constDeDuper := NewDeDuper()
	constValDeDuper := NewDeDuper()
	for i := range builder.Enums {
		convertToConst(&builder.Enums[i].Name)
		if firstTime, uniqueName := constDeDuper.unique(builder.Enums[i].Name); !firstTime {
			builder.Enums[i].Name = uniqueName
		}
		for j := range builder.Enums[i].Values {
			enumPair := &builder.Enums[i].Values[j]
			candidateName := generateConstName(builder.Enums[i].Name, enumPair.Text, enumPair.Value)
			_, uniqueName := constValDeDuper.unique(candidateName)
			enumPair.Name = uniqueName
		}
	}
	for i := range builder.IndirectEnums {
		convertToConst(&builder.IndirectEnums[i].Name)
		if firstTime, uniqueName := constDeDuper.unique(builder.IndirectEnums[i].Name); !firstTime {
			builder.IndirectEnums[i].Name = uniqueName
		}
		for range builder.IndirectEnums[i].Values { // not strictly necessary since we aren't creating identifiers from them
		}
	}
	for i := range builder.BitEnums {
		convertToConst(&builder.BitEnums[i].Name)
		if firstTime, uniqueName := constDeDuper.unique(builder.BitEnums[i].Name); !firstTime {
			builder.BitEnums[i].Name = uniqueName
		}
		for j := range builder.BitEnums[i].EnumBitValues {
			bitEnumPair := &builder.BitEnums[i].EnumBitValues[j]
			candidateName := generateConstName(builder.BitEnums[i].Name, bitEnumPair.Label, bitEnumPair.Bit)
			_, uniqueName := constValDeDuper.unique(candidateName)
			bitEnumPair.Name = uniqueName
		}
	}
	for i := range builder.FieldTypeEnums {
		convertToConst(&builder.FieldTypeEnums[i].Name)
		if firstTime, uniqueName := constDeDuper.unique(builder.FieldTypeEnums[i].Name); !firstTime {
			builder.FieldTypeEnums[i].Name = uniqueName
		}
	}
}

// validate assures that pgns with multiple definitions and the same Manufacturer ID are all single or all fast.
// It also warns with the number of multiply defined pgns each with a different Manufacturer ID.
// If more than one such warning is emitted we need to fail the build and update any code to handle the new special case.
func (builder *canboatConverter) validate() {
	specials := 0
	pgns := make(map[uint32][]*PGN)
	for i := range builder.PGNs {
		if pgns[builder.PGNs[i].PGN] == nil {
			pgns[builder.PGNs[i].PGN] = make([](*PGN), 0)
		}
		pgns[builder.PGNs[i].PGN] = append(pgns[builder.PGNs[i].PGN], builder.PGNs[i])
	}
	for _, pi := range pgns {
		var fast, single int
		if len(pi) == 1 {
			continue
		}
		//		fmt.Printf("%d: has %d entries\n", pi[0].PGN, len(pi))
		for _, p := range pi {
			if !isProprietaryPGN(p.PGN) {
				continue
			}
			manIds := make(map[int]string) //manId[]"Fast" or "Single"
			manId := getManId(p)
			if p.Type == "Fast" {
				fast++
				if manIds[manId] == "Single" {
					panic("same manId both single and fast for pgn:" + p.Id)
				} else {
					manIds[manId] = "Fast"
				}
			} else {
				single++
				if manIds[manId] == "Fast" {
					panic("same manId both single and fast for pgn:" + p.Id)
				} else {
					manIds[manId] = "Single"
				}
			}
		}
		if fast > 0 && single > 0 {
			specials++
			log.Infof("PGN: %d has %d fast and %d single instances\n", pi[0].PGN, fast, single)
		}
	}
	if specials > 0 {
		panic("New special case(s) added to canboat.json. Resolve and update this check.")
	}

	// Validate match field uniqueness for PGN variants
	builder.validateMatchFieldUniqueness()

	// Validate Fast flags against isFast function
	builder.validateFastFlags()

	// Validate fields with resolution != 1
	builder.validateResolutionFields()
}

// validateResolutionFields checks if fields with resolution != 1 have correct RangeMax values
func (builder *canboatConverter) validateResolutionFields() {
	var issues []string

	for _, pgn := range builder.PGNs {
		for _, field := range pgn.Fields {
			if field.Resolution != nil && *field.Resolution != 1.0 && *field.Resolution != 0 {
				// Calculate maximum expressible raw value
				// The 2 largest values are reserved (maxRawValue and maxRawValue-1)
				maxRawValue := uint64(1) << field.BitLength
				if field.Signed {
					maxRawValue = uint64(1) << (field.BitLength - 1)
				}

				// Account for 2 reserved values
				maxExpressibleRaw := maxRawValue - 2

				// Convert to scaled value
				maxScaledValue := float64(maxExpressibleRaw)*float64(*field.Resolution) + float64(field.Offset)

				// Compare with RangeMax using tolerance of ±1 resolution step
				tolerance := float64(*field.Resolution) * 2
				if field.RangeMax > 0 && math.Abs(maxScaledValue-field.RangeMax) > tolerance {
					issue := fmt.Sprintf("PGN %s field %s: max expressible %.6f != RangeMax %.6f (resolution %.6f, bitLength %d, signed %t)",
						pgn.Id, field.Id, maxScaledValue, field.RangeMax, *field.Resolution, field.BitLength, field.Signed)
					issues = append(issues, issue)
				}
			}
		}
	}

	if len(issues) > 0 {
		log.Infof("Found %d fields with resolution != 1 that have incorrect RangeMax values:", len(issues))
		/* for _, issue := range issues {
			log.Infof("  %s", issue)
		} */
	} else {
		log.Infof("All fields with resolution != 1 have correct RangeMax values")
	}
}

// validateMatchFieldUniqueness checks that PGN variants with the same PGN number have unique match field sets
func (builder *canboatConverter) validateMatchFieldUniqueness() {
	pgnGroups := make(map[uint32][]*PGN)

	// Group PGNs by PGN number
	for _, pgn := range builder.PGNs {
		pgnGroups[pgn.PGN] = append(pgnGroups[pgn.PGN], pgn)
	}

	for pgnNum, variants := range pgnGroups {
		if len(variants) <= 1 {
			continue // No variants to check
		}

		// Extract match fields for each variant
		variantMatchFields := make(map[string]map[string]int) // variantId -> fieldName -> matchValue

		for _, variant := range variants {
			matchFields := make(map[string]int)
			for _, field := range variant.Fields {
				if field.Match != nil {
					matchFields[field.Id] = *field.Match
				}
			}
			variantMatchFields[variant.Id] = matchFields
		}

		// Check for conflicts between variants
		variantIds := make([]string, 0, len(variants))
		for id := range variantMatchFields {
			variantIds = append(variantIds, id)
		}

		for i := 0; i < len(variantIds); i++ {
			for j := i + 1; j < len(variantIds); j++ {
				variant1 := variantIds[i]
				variant2 := variantIds[j]
				matchFields1 := variantMatchFields[variant1]
				matchFields2 := variantMatchFields[variant2]

				// Check if match field sets are identical
				if mapsEqual(matchFields1, matchFields2) {
					// Only warn for now - some PGNs may have special handling
					log.Warnf("PGN %d: variants %s and %s have identical match fields: %v - may need special handling",
						pgnNum, variant1, variant2, matchFields1)
				}
			}
		}

		// Check for variants with no match fields (when there are multiple variants)
		// Skip this check for now as some PGNs may have special handling
		noMatchFields := make([]string, 0)
		for _, variant := range variants {
			if len(variantMatchFields[variant.Id]) == 0 {
				noMatchFields = append(noMatchFields, variant.Id)
			}
		}

		if len(noMatchFields) > 0 && len(variants) > 1 {
			log.Warnf("PGN %d: variants %v have no match fields but PGN has multiple variants - may need special handling",
				pgnNum, noMatchFields)
		}
	}

	log.Infof("Match field uniqueness validation passed")
}

// isFast returns true if the specified PGN is a Fast packet based on PGN ranges
// This duplicates the function in internal/pgn/pgninfo.go for validation purposes
func isFast(pgn uint32) bool {
	switch {
	case pgn >= 59392 && pgn <= 60928: // 0xE800-0xEE00
		return false // ISO 11783, Single frame
	case pgn == 61184: // 0xEF00
		return false // Manufacturer proprietary, Single frame
	case pgn >= 61440 && pgn <= 65279: // 0xF000-0xFEFF
		return false // Standardized, Single frame
	case pgn >= 65280 && pgn <= 65535: // 0xFF00-0xFFFF
		return false // Manufacturer proprietary, Single frame
	case pgn >= 126208 && pgn <= 126464: // 0x1ED00-0x1EE00
		return true // Standardized protocol, Fast packet
	case pgn == 126720: // 0x1EF00
		return true // Manufacturer proprietary, Fast packet
	case pgn >= 126976 && pgn <= 130815: // 0x1F000-0x1FEFF
		// Mixed range - most PGNs are single frame, only specific ones are fast
		// For now, default to single frame (false) - specific fast PGNs can be added as needed
		return false // Standardized, mixed single/fast (default to single)
	case pgn >= 130816 && pgn <= 131071: // 0x1FF00-0x1FFFF
		return true // Manufacturer proprietary, Fast packet
	default:
		return false // Unknown PGN, assume single frame
	}
}

// validateFastFlags compares the Fast flag from source data against the isFast function
func (builder *canboatConverter) validateFastFlags() {
	var mismatches []string

	for _, pgn := range builder.PGNs {
		expectedFast := isFast(pgn.PGN)
		actualFast := (pgn.Type == "Fast")

		if expectedFast != actualFast {
			mismatch := fmt.Sprintf("PGN %d (%s): source Type=%s (Fast=%t), isFast()=%t",
				pgn.PGN, pgn.Id, pgn.Type, actualFast, expectedFast)
			mismatches = append(mismatches, mismatch)
		}
	}

	if len(mismatches) > 0 {
		log.Warnf("Fast flag validation found %d mismatches - source data may have inconsistencies:", len(mismatches))
		for _, mismatch := range mismatches {
			log.Warnf("  %s", mismatch)
		}
		log.Warnf("Using isFast() function results (based on NMEA 2000 specification) over source data")
	} else {
		log.Infof("Fast flag validation passed - all %d PGNs match isFast() function", len(builder.PGNs))
	}
}

// generateFastBits creates a bit array for PGNs >= 126208 indicating if they are fast packets
func (builder *canboatConverter) generateFastBits() []byte {
	// PGNs 126208-131071 = 4864 PGNs = 608 bytes
	const maxPgn = 131071
	const minPgn = 126208
	const bitArraySize = (maxPgn - minPgn + 1 + 7) / 8 // Round up to nearest byte

	bits := make([]byte, bitArraySize)

	// Set bits for fast PGNs based on source data
	for _, pgn := range builder.PGNs {
		if pgn.PGN >= minPgn && pgn.PGN <= maxPgn {
			if pgn.Type == "Fast" {
				bitIndex := pgn.PGN - minPgn
				byteIndex := bitIndex / 8
				bitOffset := bitIndex % 8
				bits[byteIndex] |= 1 << bitOffset
			}
		}
	}

	return bits
}

// mapsEqual compares two maps for equality
func mapsEqual(m1, m2 map[string]int) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v := range m1 {
		if m2[k] != v {
			return false
		}
	}
	return true
}

// zeroBitOffsets sets the BitOffset fields of repeating fields
// to 0 (issue filed with canboat)
func (builder *canboatConverter) zeroBitOffsets() {
	for _, p := range builder.PGNs {
		if len(p.FieldsRepeating1) > 0 {
			for i := range p.FieldsRepeating1 {
				p.FieldsRepeating1[i].BitOffset = 0
			}
		}
		if len(p.FieldsRepeating2) > 0 {
			for i := range p.FieldsRepeating2 {
				p.FieldsRepeating2[i].BitOffset = 0
			}
		}
	}
}

// isPointerFieldType returns true if the underlying type of a field is a pointer.
// Used by template.
func isPointerFieldType(field PGNField) bool {
	return strings.HasPrefix(convertFieldType(field), "*")
}

// getRawTypeString returns the appropriate type string for WriteRaw based on bit length and signedness
func getRawTypeString(bitLength uint16, signed bool) string {
	if signed {
		switch {
		case bitLength > 32:
			return "int64"
		case bitLength > 16:
			return "int32"
		case bitLength > 8:
			return "int16"
		default:
			return "int8"
		}
	} else {
		switch {
		case bitLength > 32:
			return "uint64"
		case bitLength > 16:
			return "uint32"
		case bitLength > 8:
			return "uint16"
		default:
			return "uint8"
		}
	}
}

// toNumber converts its input to an integer.
// Used by template.
func toNumber(str string) int {
	v, e := strconv.Atoi(str)
	if e != nil {
		panic(e)
	}
	return v
}

// constSize returns (as a string) the smallest uint needed to represent the const.
// Used by template.
func constSize(max int) string {

	switch {
	case max < 256:
		return "uint8"
	case max < 65536:
		return "uint16"
	case max < 4294967296:
		return "uint32"
	default:
		return "uint64"

	}
}

// fieldByteCount calculates the number of bytes required to read to extract a field's value.
// Used by template.
func fieldByteCount(field PGNField) uint16 {
	return uint16(math.Ceil((float64(field.BitLength) + float64(field.BitOffset)) / 8))
}

// getUnitType maps the canboat units into our tugboat unit library's categories/types
func getUnitType(unitName string) (string, string) {
	switch unitName {
	case "m":
		return "Distance", "Meter"
	case "m/s":
		return "Velocity", "MetersPerSecond"

	case "L":
		return "Volume", "Liter"

	case "K":
		return "Temperature", "Kelvin"

	case "Pa":
		return "Pressure", "Pa"

	case "L/h":
		return "Flow", "LitersPerHour"

	default:
		return "", ""
	}
}

// convertFieldType returns a string describing the golang type for a PGN field.
// used by template.
// for lookups the name of the lookup is returned.
func convertFieldType(field PGNField) string {
	switch field.FieldType {

	// assert DATE is resolution 1, unsigned
	// assert PhysicalQuantity PRESSURE, TEMPERATURE have FieldType NUMBER

	case "LOOKUP":
		return field.LookupName
	case "BITLOOKUP":
		return field.BitLookupName
	case "INDIRECT_LOOKUP":
		return field.IndirectLookupName
	case "FIELDTYPE_LOOKUP":
		return field.FieldTypeLookupName
	case "FIELD_INDEX":
		return "*uint8"
	case "NUMBER", "DATE", "TIME", "MMSI", "PGN", "ISO_NAME", "DURATION", "DYNAMIC_FIELD_KEY", "DYNAMIC_FIELD_LENGTH":
		// If it has a unit, use that
		unitType, _ := getUnitType(field.Unit)
		if unitType != "" {
			return "*units." + unitType
		}

		if field.Resolution != nil && *field.Resolution != 1.0 {
			// Let's actually make it a float
			if *field.Resolution <= resolution64BitCutoff {
				return "*float64"
			}
			return "*float32"
		}
		if field.Offset != 0 {
			return "*float32"

		}

		var baseType string
		switch {
		case field.BitLength > 64:
			panic("Too many bits for " + field.FieldType + ": " + strconv.FormatInt(int64(field.BitLength), 10))
		case field.BitLength > 32:
			baseType = "int64"
		case field.BitLength > 16:
			baseType = "int32"
		case field.BitLength > 8:
			baseType = "int16"
		default:
			baseType = "int8"
		}

		if !field.Signed {
			baseType = "u" + baseType
		}

		return "*" + baseType
	case "FLOAT":
		if field.BitLength > 32 {
			return "*float64"
		}
		return "*float32"
	case "DECIMAL", "BINARY", "DYNAMIC_FIELD_VALUE", "VARIABLE":
		return "[]uint8"
	case "STRING_FIX", "STRING_VAR", "STRING_LZ", "STRING_LAU":
		return "string"
	default:
		panic("Can't convert type: " + field.FieldType)
	}
}

// getFieldSerializer returns a string that when evaluates its value into the output stream.
// Used by template
func getFieldSerializer(pgn PGN, field PGNField, substruct string) string {
	var outstr, pre, value, post string
	var isUnit bool
	reservedCount := getReservedValueCount(field)

	// Generate FieldSpec reference for numeric fields
	specRef := fmt.Sprintf("p.GetFieldSpec(\"%s\")", field.Id)

	if field.Unit != "" { // set conv to invoke conversion to default type
		unitType, _ := getUnitType(field.Unit)
		if isUnit = unitType != ""; isUnit {
			return fmt.Sprintf("err = stream.writeUnit(p."+substruct+"%s, %s)", field.Id, specRef)
		}
	}
	switch field.FieldType {
	case "RESERVED":
		outstr = fmt.Sprintf("err = stream.writeReserved(%d, %d)", field.BitLength, field.BitOffset)
	case "SPARE":
		outstr = fmt.Sprintf("err = stream.writeSpare(%d, %d)", field.BitLength, field.BitOffset)
	case "LOOKUP", "BITLOOKUP", "INDIRECT_LOOKUP", "FIELDTYPE_LOOKUP":
		outstr = fmt.Sprintf("err = stream.putNumberRaw(uint64(p."+substruct+"%s), %d, %d)", field.Id, field.BitLength, field.BitOffset)
	case "NUMBER", "TIME", "DATE", "MMSI", "FIELD_INDEX", "DYNAMIC_FIELD_KEY", "DYNAMIC_FIELD_LENGTH", "DURATION", "PGN", "ISO_NAME":
		if field.Resolution != nil && (*field.Resolution != 1.0 || isUnit) || field.Offset != 0 {
			// Use WriteScaled for fields that need resolution/offset processing
			//floatType := "float32"
			//if field.Resolution != nil && *field.Resolution <= resolution64BitCutoff {
			//	floatType = "float64"
			//}
			pre = "err = WriteScaled(stream, "
			if !isPointerFieldType(field) {
				value = "&"
			}
			value += "p." + substruct + "%s"
			post = fmt.Sprintf(", %s)", specRef)
			outstr = fmt.Sprintf(pre+value+post, field.Id)
		} else {
			// Use WriteRaw for non-scaled fields
			//typeStr := getRawTypeString(field.BitLength, field.Signed)
			fieldRef := "p." + substruct + "%s"
			if !isPointerFieldType(field) {
				fieldRef = "&" + fieldRef
			}
			outstr = fmt.Sprintf("err = WriteRaw(stream, %s, %s)", fieldRef, specRef)
			outstr = fmt.Sprintf(outstr, field.Id)
		}
	case "FLOAT":
		if field.BitLength > 32 {
			pre = "err = stream.writeFloat64("
		} else {
			pre = "err = stream.writeFloat32("
		}
		value += "p." + substruct + "%s"
		post = ", %d, %d, %d)"
		outstr = fmt.Sprintf(pre+value+post, field.Id, field.BitLength, field.BitOffset, reservedCount)
	case "VARIABLE":
		outstr = fmt.Sprintf("err = stream.writeBinary(p."+substruct+"%s, %d, %d )", field.Id, field.BitLength, field.BitOffset)
	case "BINARY":
		if field.BitLength > 0 {
			outstr = fmt.Sprintf("err = stream.writeBinary(p."+substruct+"%s, %d, %d )", field.Id, field.BitLength, field.BitOffset)
		} else {
			outstr = fmt.Sprintf("err = stream.writeBinary(p."+substruct+"%s,"+"binaryLength, %d)", field.Id, field.BitOffset)
		}
	case "STRING_FIX":
		outstr = fmt.Sprintf("err = stream.writeStringFix([]uint8(p."+substruct+"%s), %d, %d )", field.Id, field.BitLength, field.BitOffset)
	case "STRING_LAU":
		outstr = fmt.Sprintf("err = stream.writeStringLau(p."+substruct+"%s, %d )", field.Id, field.BitOffset)
	case "STRING_LZ":
		outstr = fmt.Sprintf("err = stream.writeStringWithLength(p."+substruct+"%s, %d, %d )", field.Id, field.BitLength, field.BitOffset)
	case "DYNAMIC_FIELD_VALUE":
		outstr = fmt.Sprintf("err = stream.writeBinary(p."+substruct+"%s, valueLength, 0)", field.Id)
	default:
		outstr = fmt.Sprintf("log.Infof(\"field %s, index %d, type %s, unhandled\")", field.Name, field.Order, field.FieldType)
	}

	return outstr
}

// getFieldDeserializer returns a string that when evaluated returns its value from the input stream.
// Used by template.
func getFieldDeserializer(pgn PGN, field PGNField) [2]string {

	switch field.FieldType {
	case "LOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for LOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.readLookupField(%d)", field.BitLength), field.LookupName + "(v)"}
	case "BITLOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for BITLOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.readLookupField(%d)", field.BitLength), field.BitLookupName + "(v)"}
	case "INDIRECT_LOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for INDIRECT_LOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.readLookupField(%d)", field.BitLength), field.IndirectLookupName + "(v)"}
	case "FIELDTYPE_LOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for FIELDTYPE_LOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.readLookupField(%d)", field.BitLength), field.FieldTypeLookupName + "(v)"}
	case "FIELD_INDEX":
		specRef := fmt.Sprintf("val.GetFieldSpec(\"%s\")", field.Id)
		return [2]string{fmt.Sprintf("ReadRaw[uint8](stream, %s)", specRef), ""}
	case "NUMBER", "TIME", "DATE", "MMSI", "PGN", "ISO_NAME", "DURATION", "DYNAMIC_FIELD_KEY", "DYNAMIC_FIELD_LENGTH":
		specRef := fmt.Sprintf("val.GetFieldSpec(\"%s\")", field.Id)
		var outerVal string

		if needsScaling(field) {
			// Use ReadScaled for fields that need resolution/offset processing
			// Choose float64 for high-precision fields, float32 for others (matches struct field types)
			floatType := "float32"
			if field.Resolution != nil && *field.Resolution <= resolution64BitCutoff {
				floatType = "float64"
			}
			outerVal = fmt.Sprintf("ReadScaled[%s](stream, %s)", floatType, specRef)
		} else {
			// Use ReadRaw for non-scaled fields
			typeStr := getRawTypeString(field.BitLength, field.Signed)
			outerVal = fmt.Sprintf("ReadRaw[%s](stream, %s)", typeStr, specRef)
		}

		unitConv := ""
		unitType, unitName := getUnitType(field.Unit)
		if unitType != "" {
			unitConv = fmt.Sprintf("nullableUnit(units.%s, v, units.New%s)", unitName, unitType)
		}

		return [2]string{outerVal, unitConv}
	case "FLOAT":
		if field.BitLength != 32 {
			panic("No deserializer for IEEE Float with bitlength non-32")
		}
		return [2]string{"stream.readFloat32()", ""}
	case "DECIMAL":
		return [2]string{fmt.Sprintf("stream.readBinaryData(%d)", field.BitLength), ""}
	case "STRING_VAR":
		return [2]string{"stream.readStringStartStopByte()", ""}
	case "STRING_LAU":
		return [2]string{"stream.readStringWithLengthAndControl()", ""}
	case "STRING_FIX":
		return [2]string{fmt.Sprintf("stream.readFixedString(%d)", field.BitLength), ""}
	case "STRING_LZ":
		return [2]string{fmt.Sprintf("stream.readStringWithLength(%d)", field.BitLength), ""}
	case "BINARY":
		if field.BitLength > 0 {
			return [2]string{fmt.Sprintf("stream.readBinaryData(%d)", field.BitLength), ""}
		}
		return [2]string{"stream.readBinaryData(binaryLength)", ""}
	case "VARIABLE":
		specRef := fmt.Sprintf("val.GetFieldSpec(\"%s\")", field.Id)
		return [2]string{fmt.Sprintf("stream.readVariableDataWithSpec(%s)", specRef), ""}
	case "DYNAMIC_FIELD_VALUE":
		return [2]string{"stream.readBinaryData(valueLength)", ""}
	default:
		panic("No deserializer for type: " + field.FieldType)
	}
}

// matchManufacturer returns the required Match value of the Manufacturer Code as a string.
// Used by template.
func matchManufacturer(pgn PGN) string {
	for _, field := range pgn.Fields {
		if field.Id == "ManufacturerCode" {
			if field.Match != nil {
				return strconv.Itoa(*field.Match)
			}
		}
	}
	return "0"
}

// makeIndirectMap returns a map[int]map[int][string] initialized from its argument.
// Used by template.
func makeIndirectMap(iEnum LookupIndirectEnumeration) map[int]map[int]string {
	inMap := make(map[int]map[int]string)
	for _, item := range iEnum.Values {
		if inMap[item.Value1] == nil {
			inMap[item.Value1] = make(map[int]string)
		}
		inMap[item.Value1][item.Value2] = item.Text
	}
	return inMap
}

// Utility functions

// cacheFromWeb updates a cache file (if needed) with the contents of a URL.
func cacheFromWeb(name, url string) (string, error) {
	// get stats on cached file (name+cache)
	// if not exist or expired, get contents from web and save in cached file
	// read cached file into buffer and return
	var cacheDuration = 1 * time.Hour
	var cachedName = name + ".cache"
	fstat, err := os.Stat(cachedName)
	if err != nil || time.Since(fstat.ModTime()) > cacheDuration {
		log.Infof("Downloading source data...")

		req, _ := http.NewRequest("GET", url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return cachedName, err
		}
		defer func() {
			if err = resp.Body.Close(); err != nil {
				log.WithError(err).Warn("error closing http response body")
			}
		}()

		f, err := os.OpenFile(cachedName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			log.WithError(err).Fatalf("failed to open cache file %s", cachedName)
		}

		bar := progressbar.DefaultBytes(
			resp.ContentLength,
			fmt.Sprintf("Downloading %s", name),
		)
		if _, err = io.Copy(io.MultiWriter(f, bar), resp.Body); err != nil {
			log.WithError(err).Fatalf("failed to write to cache file %s", cachedName)
		}

		if err := f.Close(); err != nil {
			log.WithError(err).Fatalf("failed to close cache file %s", cachedName)
		}
	} else {
		log.Infof("Using cached file %s", name)
	}
	return cachedName, nil
}

// loadCachedWebContent updates the cache contents and returns it as a byte slice.
func loadCachedWebContent(name, url string) ([]byte, error) {
	cachedName, err := cacheFromWeb(name, url)
	if err != nil {
		panic(err)
	}
	f, err := os.Open(cachedName)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err = f.Close(); err != nil {
			log.WithError(err).Warnf("failed to close cache file %s", cachedName)
		}
	}()
	cacheContent, err := io.ReadAll(f)
	if err != nil {
		panic(err)
	}
	return cacheContent, nil
}

// capitalizeFirstChar forces the first character to upper case and converts "1st" to "First".
func capitalizeFirstChar(raw string) string {
	title := strings.ToUpper(raw[0:1]) + raw[1:]
	if len(title) > 3 && title[0:3] == "1st" {
		title = "First" + title[3:]
	}
	return title
}

// getManId returns the required Manufacturer Code ID for the defined PGN.
// Only call on Proprietary PGNs!
func getManId(p *PGN) int {
	for _, field := range p.Fields {
		if field.Id == "ManufacturerCode" {
			if field.Match != nil {
				return *field.Match
			} else {
				log.Infof("Proprietary PGN %d does not match on ManufacturerCode", p.PGN)
				return 0
			}
		}
	}
	return 0
}

// digitToWord is a map for converting leading digits to words.
var digitToWord = map[string]string{
	"0": "Zero", "1": "One", "2": "Two", "3": "Three", "4": "Four",
	"5": "Five", "6": "Six", "7": "Seven", "8": "Eight", "9": "Nine",
}

// generateConstName creates a Go constant identifier from a display string.
// It handles leading digits and truncates long names.
func generateConstName(enumName, text string, value int) string {
	// Handle leading digit
	if len(text) > 0 && unicode.IsDigit(rune(text[0])) {
		parts := strings.Fields(text)
		if word, ok := digitToWord[parts[0]]; ok {
			parts[0] = word
			text = strings.Join(parts, " ")
		}
	}

	words := strings.Fields(text)
	caser := cases.Title(language.English)
	camelCase := caser.String(text)

	var builder strings.Builder
	for _, r := range camelCase {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	candidateName := builder.String()

	// If there are more than 3 words, truncate at 30 characters
	if len(words) > 3 && len(candidateName) > 30 {
		candidateName = candidateName[:30]
	}

	// If the resulting name is empty, use the fallback.
	if len(candidateName) == 0 {
		return fmt.Sprintf("%s%d", enumName, value)
	}

	// Ensure the first character is a letter.
	if !unicode.IsLetter(rune(candidateName[0])) {
		// Prepend the enumeration name to guarantee a valid identifier start.
		candidateName = enumName + candidateName
	}

	return candidateName
}

// convertToConst changes XXX_YYY to XxxYyyConst (all go consts have global namespace scope).
func convertToConst(name *string) {
	parts := strings.Split(strings.ToLower(*name), "_")
	s := ""
	makeTitle := cases.Title(language.English)
	for i := range parts {
		s += makeTitle.String(parts[i])
	}
	*name = s + "Const"
}

// isProprietaryPGN evaluates if its input falls into the proprietary PGN ranges.
// Duplicated from n2k/pgninfo.go. Could export it there and use it here, but
// we're generating a source file for that package, so a bootstrapping problem...
func isProprietaryPGN(pgn uint32) bool {
	if pgn >= 0x0EF00 && pgn <= 0x0EFFF {
		// proprietary PDU1 (addressed) single-frame range 0EF00 to 0xEFFF (61184 - 61439) messages.
		// Addressed means that you send it to specific node on the bus. This you can easily use for responding,
		// since you know the sender. For sender it is bit more complicate since your device address may change
		// due to address claiming. There is N2kDeviceList module for handling devices on bus and find them by
		// "NAME" (= 64 bit value set by SetDeviceInformation ).
		return true
	} else if pgn >= 0x0FF00 && pgn <= 0x0FFFF {
		// proprietary PDU2 (non addressed) single-frame range 0xFF00 to 0xFFFF (65280 - 65535).
		// Non addressed means that destination wil be 255 (=broadcast) so any cabable device can handle it.
		return true
	} else if pgn >= 0x1EF00 && pgn <= 0x1EFFF {
		// proprietary PDU1 (addressed) fast-packet PGN range 0x1EF00 to 0x1EFFF (126720 - 126975)
		return true
	} else if pgn >= 0x1FF00 && pgn <= 0x1FFFF {
		// proprietary PDU2 (non addressed) fast packet range 0x1FF00 to 0x1FFFF (130816 - 131071)
		return true
	}

	return false
}

// hasMultipleVariants checks if a PGN has multiple variants
func hasMultipleVariants(pgn PGN) bool {
	// This will be implemented to check if PGN has multiple variants
	// For now, return false - will be updated when we implement the discriminator logic
	return false
}

// getVariantsForPgn returns all variants for a PGN
func getVariantsForPgn(pgn PGN) []PGN {
	// This will be implemented to return variants for a PGN
	// For now, return empty slice - will be updated when we implement the discriminator logic
	return []PGN{}
}

type DecoderConfig struct {
	Repeat1            bool
	Repeat2            bool
	Repeat1CountField  uint8
	Repeat2CountField  uint8
	BitLengthField     uint8
	DynamicLengthField uint8
}

func getDecoderConfig(pgn PGN) DecoderConfig {
	dynamicLengthField := uint8(0)
	for _, field := range pgn.AllFields {
		if field.FieldType == "DYNAMIC_FIELD_LENGTH" {
			dynamicLengthField = field.Order
		}
	}
	return DecoderConfig{
		Repeat1:            pgn.RepeatingFieldSet1Size > 0,
		Repeat2:            pgn.RepeatingFieldSet2Size > 0,
		Repeat1CountField:  pgn.RepeatingFieldSet1CountField,
		Repeat2CountField:  pgn.RepeatingFieldSet2CountField,
		BitLengthField:     pgn.BitLengthField,
		DynamicLengthField: dynamicLengthField,
	}
}

// groupByPGN groups PGNs by their PGN number
func groupByPGN(pgns []*PGN) map[uint32][]*PGN {
	groups := make(map[uint32][]*PGN)
	for _, pgn := range pgns {
		groups[pgn.PGN] = append(groups[pgn.PGN], pgn)
	}
	return groups
}
