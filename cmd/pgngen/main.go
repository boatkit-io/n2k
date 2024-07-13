// Command pgngen generates the file pgninfo_generated.go.
// The generated file provides go declarations and functions to assist conversion from
// strongly typed go structures and NMEA 2000 frames.
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

// log provides standard logging capability to the program.
var log = logrus.StandardLogger()

// pgninfoTemplate is the template used to generate the output file.
//
//go:embed templates/pgninfo.go.tmpl
var pgninfoTemplate string

func main() {
	fmt.Println("Entered Main")
	builder := newCanboatConverter()
	builder.fixup()
	builder.filter()
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
	RangeMin                 float32
	RangeMax                 float32
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
	conv.validate()
}

// filter builds separate slices for known, unknown (no Canboat samples), and incomplete PGNs
func (conv *canboatConverter) filter() {
	known := make([]*PGN, 0)
	unknown := make([]*PGN, 0)
	incomplete := make([]*PGN, 0)
	var keep bool
	for _, pgn := range conv.PGNs {
		keep = true
		if !pgn.Complete {
			switch len(pgn.Missing) {
			case 0:
				panic("Complete is false but Missing is empty!")
			case 1:
				keep = true // pgn.Missing[0] == "Interval"
			default:
				incomplete = append(incomplete, pgn)
				for _, miss := range pgn.Missing {
					if miss == "SampleData" {
						keep = false
						unknown = append(unknown, pgn)
					}
				}
			}
		}
		if keep {
			known = append(known, pgn)
		}
	}
	conv.PGNs = known
	conv.NeverSeenPGNs = unknown
	conv.IncompletePGNS = incomplete
	log.Infof("After filtering, known: %d, unknown: %d, incomplete: %d", len(conv.PGNs), len(conv.NeverSeenPGNs), len(conv.IncompletePGNS))
}

// write outputs the pgninfo_generated.go file. Most of the work occurs in the template.
func (conv *canboatConverter) write() {
	if f, err := os.Create(filepath.Join("pkg", "pgn", "pgninfo_generated.go")); err != nil {
		panic(err)
	} else {
		t := template.Must(template.New("pgninfo").Funcs(sprig.TxtFuncMap()).Funcs(template.FuncMap{
			"convertFieldType":     convertFieldType,
			"getFieldDeserializer": getFieldDeserializer,
			"fieldByteCount":       fieldByteCount,
			"concat":               func(strs ...string) string { return strings.Join(strs, "") },
			"toNumber":             toNumber,
			"toVarName":            toVarName,
			"isPointerFieldType":   isPointerFieldType,
			"constSize":            constSize,
			"subtract":             func(x, y uint8) uint8 { return x - y },
			"matchManufacturer":    matchManufacturer,
			"makeIndirectMap":      makeIndirectMap,
			"derefInt":             func(ip *int) int { return *ip },
			"isNil":                func(fp *int) bool { return fp == nil },
			"contains":             func(in, substr string) bool { return strings.Contains(in, substr) },
		}).Parse(pgninfoTemplate))

		templateData := struct {
			PGNDoc any
		}{
			PGNDoc: conv,
		}

		if err := t.Execute(f, templateData); err != nil {
			panic(err)
		}
		f.Close()
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
			fixupField(&conv.PGNs[i].Fields[j], *fieldDeDuper)
			// For variable length fields, the length of such a field is passed
			// in another field. When we find such a field we keep the value in
			// the PGN so it can be used by the decoder for that PGN.
			if conv.PGNs[i].Fields[j].BitLengthField != 0 {
				conv.PGNs[i].BitLengthField = conv.PGNs[i].Fields[j].BitLengthField
			}
		}
	}
}

// fixupField capitializes Id's first char, assures field name is unique, and forces lookup names.
func fixupField(field *PGNField, dedup DeDuper) {
	field.Id = capitalizeFirstChar(field.Id)
	if len(field.FieldTypeLookupName) > 0 {
		convertToConst(&field.FieldTypeLookupName)
	}
	if field.FieldType == "LOOKUP" && len(field.LookupName) == 0 {
		log.Infof("Lookup without Enumeration Name: " + field.Id)
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
}

// fixEnumDefs checks that enum names are unique and makes them legal golang identifiers.
func (builder *canboatConverter) fixEnumDefs() {
	constDeDuper := NewDeDuper()
	for i := range builder.Enums {
		convertToConst(&builder.Enums[i].Name)
		if firstTime, _ := constDeDuper.unique(builder.Enums[i].Name); !firstTime {
			panic("Enum name not unique: " + builder.Enums[i].Name)
		}
		for j := range builder.Enums[i].Values {
			forceFirstLetter(&builder.Enums[i].Values[j].Text)
		}
	}
	for i := range builder.IndirectEnums {
		convertToConst(&builder.IndirectEnums[i].Name)
		if firstTime, _ := constDeDuper.unique(builder.IndirectEnums[i].Name); !firstTime {
			panic("IndirectEnum name not unique: " + builder.IndirectEnums[i].Name)
		} // else don't need to change name
		for j := range builder.IndirectEnums[i].Values { // not strictly necessary since we aren't creating identifiers from them
			forceFirstLetter(&builder.IndirectEnums[i].Values[j].Text)
		}
	}
	for i := range builder.BitEnums {
		convertToConst(&builder.BitEnums[i].Name)
		if firstTime, _ := constDeDuper.unique(builder.BitEnums[i].Name); !firstTime {
			panic("BitEnum name not unique: " + builder.BitEnums[i].Name)
		} // else don't need to change name
		for j := range builder.BitEnums[i].EnumBitValues {
			forceFirstLetter(&builder.BitEnums[i].EnumBitValues[j].Label)
		}
	}
	for i := range builder.FieldTypeEnums {
		convertToConst(&builder.FieldTypeEnums[i].Name)
		if firstTime, _ := constDeDuper.unique(builder.FieldTypeEnums[i].Name); !firstTime {
			panic("FieldTypeEnum name not unique: " + builder.FieldTypeEnums[i].Name)
		} // else don't need to change name
	}
}

// validate assures that pgns with multiple definitions and the same Manufacturer ID are all single or all fast.
// It also warns the with the number of multiply defined pgns each with a different Manufacturer ID.
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
}

// varNameReplacer points to a function that changes various substrings with values legal in golang identifiers.
// Used by toVarName (and so by template).
var varNameReplacer = strings.NewReplacer(" ", "", "/", "", "+1", "Plus1", "-1", "Minus1", "+", "", "-", "", "(", "", ")", "", "#", "", ".", "", ":", "", "%", "Percent", "&", "And", ",", "")

// varDeDuper is used to assure a variable name is unique.
// Used by toVarName (and so by template).
var varDeDuper = NewDeDuper()

// toVarName massages its input to a legal golang identifier.
// Used by template.
func toVarName(str string) string {
	str = strings.Title(str) //nolint:staticcheck
	str = varNameReplacer.Replace(str)
	_, str = varDeDuper.unique(str)
	return str
}

// isPointerFieldType returns true if the underlying type of a field is a pointer.
// Used by template.
func isPointerFieldType(field PGNField) bool {
	return strings.HasPrefix(convertFieldType(field), "*")
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
		return "uinit32"
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
	case "NUMBER", "DATE", "TIME", "MMSI":
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
	case "DECIMAL", "BINARY", "KEY_VALUE", "VARIABLE":
		return "[]uint8"
	case "STRING_FIX", "STRING_VAR", "STRING_LZ", "STRING_LAU":
		return "string"
	default:
		panic("Can't convert type: " + field.FieldType)
	}
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
		return [2]string{fmt.Sprintf("stream.readUInt8(%d)", field.BitLength), ""}
	case "NUMBER", "TIME", "DATE", "MMSI":
		var outerVal string
		if field.Signed {
			switch {
			case field.Resolution != nil && *field.Resolution <= resolution64BitCutoff:
				outerVal = fmt.Sprintf("stream.readSignedResolution64Override(%d, %g)", field.BitLength, *field.Resolution)
			case field.Resolution != nil && *field.Resolution != 1.0:
				outerVal = fmt.Sprintf("stream.readSignedResolution(%d, %g)", field.BitLength, *field.Resolution)
			case field.BitLength > 32:
				outerVal = fmt.Sprintf("stream.readInt64(%d)", field.BitLength)
			case field.BitLength > 16:
				outerVal = fmt.Sprintf("stream.readInt32(%d)", field.BitLength)
			case field.BitLength > 8:
				outerVal = fmt.Sprintf("stream.readInt16(%d)", field.BitLength)
			default:
				outerVal = fmt.Sprintf("stream.readInt8(%d)", field.BitLength)
			}
		} else {
			switch {
			case field.Resolution != nil && *field.Resolution != 1.0:
				outerVal = fmt.Sprintf("stream.readUnsignedResolution(%d, %g)", field.BitLength, *field.Resolution)
			case field.BitLength > 32:
				outerVal = fmt.Sprintf("stream.readUInt64(%d)", field.BitLength)
			case field.BitLength > 16:
				outerVal = fmt.Sprintf("stream.readUInt32(%d)", field.BitLength)
			case field.BitLength > 8:
				outerVal = fmt.Sprintf("stream.readUInt16(%d)", field.BitLength)
			default:
				outerVal = fmt.Sprintf("stream.readUInt8(%d)", field.BitLength)
			}
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
		return [2]string{"stream.readStringWithLength()", ""}
	case "BINARY":
		if field.BitLength > 0 {
			return [2]string{fmt.Sprintf("stream.readBinaryData(%d)", field.BitLength), ""}
		}
		return [2]string{"stream.readBinaryData(binaryLength)", ""}
	case "VARIABLE":
		return [2]string{"stream.readVariableData(*val.Pgn, manufacturer, fieldIndex)", ""}
	case "KEY_VALUE":
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
		defer resp.Body.Close()

		f, _ := os.OpenFile(cachedName, os.O_CREATE|os.O_WRONLY, 0644)

		bar := progressbar.DefaultBytes(
			resp.ContentLength,
			fmt.Sprintf("Downloading %s", name),
		)
		_, _ = io.Copy(io.MultiWriter(f, bar), resp.Body)

		f.Close()
	} else {
		log.Infof(fmt.Sprintf("Using cached file %s", name))
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
	defer f.Close()
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

// forceFirstLetter assures the name starts with a letter (required for use as a golang identifier).
func forceFirstLetter(name *string) {
	current := *name
	if !unicode.IsLetter(rune(current[0])) {
		current = "A" + current
		*name = current
	}
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
