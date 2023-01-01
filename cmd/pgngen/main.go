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

var log = logrus.StandardLogger()

//go:embed templates/pgninfo.go.tmpl
var pgninfoTemplate string

// Transforms a json file from canboat to generate a source file for the n2k package
func main() {
	fmt.Println("Entered Main")
	builder := newCanboatConverter()
	builder.fixup()
	builder.write()

}

// We suck the json into this structure, massage the result, and use a template to
// generate the output source file
type canboatConverter struct {
	Comment       string
	CreatorCode   string
	License       string
	Version       string
	BitEnums      []BitEnumeration            `json:"LookupBitEnumerations"`
	Enums         []LookupEnumeration         `json:"LookupEnumerations"`
	IndirectEnums []LookupIndirectEnumeration `json:"LookupIndirectEnumerations"`
	PGNs          []PGN
}

type LookupEnumeration struct {
	Name     string
	MaxValue int
	Values   []EnumPair `json:"EnumValues"`
}

type LookupIndirectEnumeration struct {
	Name     string
	MaxValue int
	Values   []EnumTriplet `json:"EnumValues"`
}

type EnumTriplet struct {
	Text   string `json:"Name"`
	Value1 int
	Value2 int
}

type EnumPair struct {
	Text  string `json:"Name"`
	Value int
}

type BitEnumeration struct {
	Name          string
	MaxValue      int
	EnumBitValues []BitEnumPair
}
type BitEnumPair struct {
	Label string `json:"Name"`
	Bit   int
}

type PGN struct {
	PGN                          uint32
	Id                           string
	Description                  string
	Explanation                  string
	Type                         string
	Complete                     bool
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
	Units                    string
	LookupName               string `json:"LookupEnumeration"`
	BitLookupName            string `json:"LookupBitEnumeration"`
	IndirectLookupName       string `json:"LookupIndirectEnumeration"`
	IndirectLookupFieldOrder uint8  `json:"LookupIndirectEnumerationFieldOrder"`
}

func newCanboatConverter() *canboatConverter {
	c := new(canboatConverter)
	c.init()
	return c
}

func (conv *canboatConverter) init() {
	raw, _ := loadCachedWebContent("canboatjson", "https://github.com/canboat/canboat/raw/master/docs/canboat.json")
	err := json.Unmarshal(raw, conv)
	if err != nil {
		log.Info(err)
	}
	log.Infof("\nInitially Parsed Bitfield enums: %d", len(conv.BitEnums))
	log.Infof("Initially Parsed Lookup enums: %d", len(conv.Enums))
	log.Infof("Initially Parsed IndirectLookup enums: %d", len(conv.IndirectEnums))
	log.Infof("Parsed pgns: %d", len(conv.PGNs))
}

func (conv *canboatConverter) fixup() {
	conv.fixIDs()
	conv.fixEnumDefs()
	conv.fixRepeating()
	conv.validate()
}

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
		}).Parse(pgninfoTemplate))

		templateData := struct {
			PGNDoc interface{}
		}{
			PGNDoc: conv,
		}

		if err := t.Execute(f, templateData); err != nil {
			panic(err)
		}
		f.Close()
	}
}

// capitalize initial letter for each ID
// dedup field names within a pgn (latest canboat assures field names unique, so we'll just verify)
func (conv *canboatConverter) fixIDs() {
	pgnDeDuper := NewDeDuper()
	for i := range conv.PGNs {
		fieldDeDuper := NewDeDuper()
		// Capitalize first letter of the Ids (currently lowercase)
		conv.PGNs[i].Id = capitalizeFirstChar(conv.PGNs[i].Id)
		if !pgnDeDuper.isUnique(conv.PGNs[i].Id) {
			panic("PGN ID not unique: " + conv.PGNs[i].Id)
		}
		for j := range conv.PGNs[i].Fields {
			fixupField(&conv.PGNs[i].Fields[j], *fieldDeDuper)
			if conv.PGNs[i].Fields[j].BitLengthField != 0 {
				conv.PGNs[i].BitLengthField = conv.PGNs[i].Fields[j].BitLengthField
			}
		}
	}
}

func fixupField(field *PGNField, dedup DeDuper) {
	field.Id = capitalizeFirstChar(field.Id)
	if field.FieldType == "LOOKUP" && len(field.LookupName) == 0 {
		log.Infof("Lookup without Enumeration Name: " + field.Id)
	}
	if field.Id != dedup.unique(field.Id) {
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

func (builder *canboatConverter) fixRepeating() {
	for i := range builder.PGNs {
		pgn := &builder.PGNs[i]
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

		builder.PGNs[i].AllFields = append(pgn.Fields, pgn.FieldsRepeating1...)
		builder.PGNs[i].AllFields = append(pgn.Fields, pgn.FieldsRepeating2...)
	}
}

func (builder *canboatConverter) fixEnumDefs() {
	constDeDuper := NewDeDuper()
	for i := range builder.Enums {
		convertToConst(&builder.Enums[i].Name)
		if !constDeDuper.isUnique(builder.Enums[i].Name) {
			panic("Enum name not unique: " + builder.Enums[i].Name)
		}
		for j := range builder.Enums[i].Values {
			forceFirstLetter(&builder.Enums[i].Values[j].Text)
		}
	}
	for i := range builder.IndirectEnums {
		convertToConst(&builder.IndirectEnums[i].Name)
		if !constDeDuper.isUnique(builder.IndirectEnums[i].Name) {
			panic("IndirectEnum name not unique: " + builder.IndirectEnums[i].Name)
		}
		for j := range builder.IndirectEnums[i].Values { // not strictly necessary since we aren't creating identifiers from them
			forceFirstLetter(&builder.IndirectEnums[i].Values[j].Text)
		}
	}
	for i := range builder.BitEnums {
		convertToConst(&builder.BitEnums[i].Name)
		if builder.BitEnums[i].Name != constDeDuper.unique(builder.BitEnums[i].Name) {
			panic("BitEnum name not unique: " + builder.BitEnums[i].Name)
		}
		for j := range builder.BitEnums[i].EnumBitValues {
			forceFirstLetter(&builder.BitEnums[i].EnumBitValues[j].Label)
		}
	}
}

func (builder *canboatConverter) validate() {
	pgns := make(map[uint32][]*PGN)
	for i := range builder.PGNs {
		if pgns[builder.PGNs[i].PGN] == nil {
			pgns[builder.PGNs[i].PGN] = make([](*PGN), 0)
		}
		pgns[builder.PGNs[i].PGN] = append(pgns[builder.PGNs[i].PGN], &builder.PGNs[i])
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
			log.Infof("PGN: %d has %d fast and %d single instances\n", pi[0].PGN, fast, single)
		}
	}
}

// functions invoked by template

var varNameReplacer = strings.NewReplacer(" ", "", "/", "", "+1", "Plus1", "-1", "Minus1", "+", "", "-", "", "(", "", ")", "", "#", "", ".", "", ":", "", "%", "Percent", "&", "And", ",", "")
var varDeDuper = NewDeDuper()

func toVarName(str string) string {
	str = strings.Title(str) //nolint:staticcheck
	str = varNameReplacer.Replace(str)
	str = varDeDuper.unique(str)
	return str
}

func isPointerFieldType(field PGNField) bool {
	return strings.HasPrefix(convertFieldType(field), "*")
}

func toNumber(str string) int {
	v, e := strconv.Atoi(str)
	if e != nil {
		panic(e)
	}
	return v
}

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

func fieldByteCount(field PGNField) uint16 {
	return uint16(math.Ceil((float64(field.BitLength) + float64(field.BitOffset)) / 8))
}

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

	case "NUMBER", "DATE", "TIME", "MMSI":
		if field.Resolution != nil && *field.Resolution != 1.0 {
			// Let's actually make it a float
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
	case "DECIMAL":
		return "[]uint8"
	case "VARIABLE", "BINARY":
		return "interface{}"
	case "STRING_FIX", "STRING_VAR", "STRING_LZ", "STRING_LAU":
		return "string"
	default:
		panic("Can't convert type: " + field.FieldType)
	}
}

func getFieldDeserializer(pgn PGN, field PGNField) [2]string {
	switch field.FieldType {
	case "LOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for LOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.readLookupField(%d)", field.BitLength), field.LookupName}
	case "BITLOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for BITLOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.readLookupField(%d)", field.BitLength), field.BitLookupName}
	case "INDIRECT_LOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for INDIRECT_LOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.readLookupField(%d)", field.BitLength), field.IndirectLookupName}
	case "NUMBER", "TIME", "DATE", "MMSI":
		var outerVal string
		if field.Signed {
			switch {
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
		return [2]string{outerVal, ""}
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
		return [2]string{"stream.readVariableData(pgn, fieldIndex)", ""}
	//	return [2]string{"stream.readBinaryData(0)", ""}
	default:
		panic("No deserializer for type: " + field.FieldType)
	}
}

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

func capitalizeFirstChar(raw string) string {
	title := strings.ToUpper(raw[0:1]) + raw[1:]
	if len(title) > 3 && title[0:3] == "1st" {
		title = "First" + title[3:]
	}
	return title
}

// only call on Proprietary PGNs
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

func forceFirstLetter(name *string) {
	current := *name
	if !unicode.IsLetter(rune(current[0])) {
		current = "A" + current
		*name = current
	}
}

// convert XXX_YYY to XxxYyyConst (all have global namespace scope)
func convertToConst(name *string) {
	parts := strings.Split(strings.ToLower(*name), "_")
	s := ""
	makeTitle := cases.Title(language.English)
	for i := range parts {
		s += makeTitle.String(parts[i])
	}
	*name = s + "Const"
}

// duplicated from n2k/pgninfo.go. Could export it there and use it here, but
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
