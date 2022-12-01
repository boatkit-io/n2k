package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	builder := NewPGNBuilder()
	builder.fixup()
	builder.write()

}

// We suck the json into this structure, massage the result, and use a template to
// generate the output source file
type PGNBuilder struct {
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

func NewPGNBuilder() *PGNBuilder {
	b := new(PGNBuilder)
	b.init()
	return b
}

func (self *PGNBuilder) init() {
	raw, _ := loadCachedWebContent("canboatjson", "https://github.com/canboat/canboat/raw/master/docs/canboat.json")
	err := json.Unmarshal(raw, self)
	if err != nil {
		log.Info(err)
	}
	log.Infof("\nInitially Parsed Bitfield enums: %d", len(self.BitEnums))
	log.Infof("Initially Parsed Lookup enums: %d", len(self.Enums))
	log.Infof("Initially Parsed IndirectLookup enums: %d", len(self.IndirectEnums))
	log.Infof("Parsed pgns: %d", len(self.PGNs))
}

func (self *PGNBuilder) fixup() {
	self.fixIDs()
	self.fixEnumDefs()
	self.fixRepeating()
	self.validate()
}

func (self *PGNBuilder) write() {
	if f, err := os.Create("pgninfo_generated.go"); err != nil {
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
			PGNDoc: self,
		}

		if err := t.Execute(f, templateData); err != nil {
			panic(err)
		}
		f.Close()
	}
}

// capitalize initial letter for each ID
// dedup field names within a pgn (latest canboat assures field names unique, so we'll just verify)
func (self *PGNBuilder) fixIDs() {
	pgnDeDuper := NewDeDuper()
	for i := range self.PGNs {
		fieldDeDuper := NewDeDuper()
		// Capitalize first letter of the Ids (currently lowercase)
		self.PGNs[i].Id = capitalizeFirstChar(self.PGNs[i].Id)
		if !pgnDeDuper.isUnique(self.PGNs[i].Id) {
			panic("PGN ID not unique: " + self.PGNs[i].Id)
		}
		for j := range self.PGNs[i].Fields {
			fixupField(&self.PGNs[i].Fields[j], *fieldDeDuper)
			if self.PGNs[i].Fields[j].BitLengthField != 0 {
				self.PGNs[i].BitLengthField = self.PGNs[i].Fields[j].BitLengthField
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

func (self *PGNBuilder) fixRepeating() {
	for i := range self.PGNs {
		pgn := &self.PGNs[i]
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

		self.PGNs[i].AllFields = append(pgn.Fields, pgn.FieldsRepeating1...)
		self.PGNs[i].AllFields = append(pgn.Fields, pgn.FieldsRepeating2...)
	}
}

func (self *PGNBuilder) fixEnumDefs() {
	constDeDuper := NewDeDuper()
	for i := range self.Enums {
		convertToConst(&self.Enums[i].Name)
		if !constDeDuper.isUnique(self.Enums[i].Name) {
			panic("Enum name not unique: " + self.Enums[i].Name)
		}
		for j := range self.Enums[i].Values {
			forceFirstLetter(&self.Enums[i].Values[j].Text)
		}
	}
	for i := range self.IndirectEnums {
		convertToConst(&self.IndirectEnums[i].Name)
		if !constDeDuper.isUnique(self.IndirectEnums[i].Name) {
			panic("IndirectEnum name not unique: " + self.IndirectEnums[i].Name)
		}
		for j := range self.IndirectEnums[i].Values { // not strictly necessary since we aren't creating identifiers from them
			forceFirstLetter(&self.IndirectEnums[i].Values[j].Text)
		}
	}
	for i := range self.BitEnums {
		convertToConst(&self.BitEnums[i].Name)
		if self.BitEnums[i].Name != constDeDuper.unique(self.BitEnums[i].Name) {
			panic("BitEnum name not unique: " + self.BitEnums[i].Name)
		}
		for j := range self.BitEnums[i].EnumBitValues {
			forceFirstLetter(&self.BitEnums[i].EnumBitValues[j].Label)
		}
	}
}

func (self *PGNBuilder) validate() {
	pgns := make(map[uint32][]*PGN)
	for i := range self.PGNs {
		if pgns[self.PGNs[i].PGN] == nil {
			pgns[self.PGNs[i].PGN] = make([](*PGN), 0)
		}
		pgns[self.PGNs[i].PGN] = append(pgns[self.PGNs[i].PGN], &self.PGNs[i])
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
		return "unit64"

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
		return "[]uint8"
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
		return [2]string{fmt.Sprintf("stream.ReadLookupField(%d)", field.BitLength), field.LookupName}
	case "BITLOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for BITLOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.ReadLookupField(%d)", field.BitLength), field.BitLookupName}
	case "INDIRECT_LOOKUP":
		if field.BitLength > 32 {
			panic("No deserializer for INDIRECT_LOOKUP with bitlength > 32")
		}
		return [2]string{fmt.Sprintf("stream.ReadLookupField(%d)", field.BitLength), field.IndirectLookupName}
	case "NUMBER", "TIME", "DATE", "MMSI":
		var outerVal string
		if field.Signed {
			switch {
			case field.Resolution != nil && *field.Resolution != 1.0:
				outerVal = fmt.Sprintf("stream.ReadSignedResolution(%d, %g)", field.BitLength, *field.Resolution)
			case field.BitLength > 32:
				outerVal = fmt.Sprintf("stream.ReadInt64(%d)", field.BitLength)
			case field.BitLength > 16:
				outerVal = fmt.Sprintf("stream.ReadInt32(%d)", field.BitLength)
			case field.BitLength > 8:
				outerVal = fmt.Sprintf("stream.ReadInt16(%d)", field.BitLength)
			default:
				outerVal = fmt.Sprintf("stream.ReadInt8(%d)", field.BitLength)
			}
		} else {
			switch {
			case field.Resolution != nil && *field.Resolution != 1.0:
				outerVal = fmt.Sprintf("stream.ReadUnsignedResolution(%d, %g)", field.BitLength, *field.Resolution)
			case field.BitLength > 32:
				outerVal = fmt.Sprintf("stream.ReadUInt64(%d)", field.BitLength)
			case field.BitLength > 16:
				outerVal = fmt.Sprintf("stream.ReadUInt32(%d)", field.BitLength)
			case field.BitLength > 8:
				outerVal = fmt.Sprintf("stream.ReadUInt16(%d)", field.BitLength)
			default:
				outerVal = fmt.Sprintf("stream.ReadUInt8(%d)", field.BitLength)
			}
		}
		return [2]string{outerVal, ""}
	case "FLOAT":
		if field.BitLength != 32 {
			panic("No deserializer for IEEE Float with bitlength non-32")
		}
		return [2]string{"stream.ReadFloat32()", ""}
	case "DECIMAL":
		return [2]string{fmt.Sprintf("stream.ReadBinaryData(%d)", field.BitLength), ""}
	case "STRING_VAR":
		return [2]string{"stream.ReadStringStartStopByte()", ""}
	case "STRING_LAU":
		return [2]string{"stream.ReadStringWithLengthAndControl()", ""}
	case "STRING_FIX":
		return [2]string{fmt.Sprintf("stream.ReadFixedString(%d)", field.BitLength), ""}
	case "STRING_LZ":
		return [2]string{"stream.ReadStringWithLength()", ""}
	case "BINARY":
		if field.BitLength > 0 {
			return [2]string{fmt.Sprintf("stream.ReadBinaryData(%d)", field.BitLength), ""}
		}
		return [2]string{"stream.ReadBinaryData(binaryLength)", ""}
	case "VARIABLE":
		//		return [2]string{"stream.ReadVariableData(pgn, fieldIndex)", ""}
		return [2]string{"stream.ReadBinaryData(0)", ""}
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

// duplicated from n2k/PgnInfo.go. Could export it there and use it here, but
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

type DeDuper struct {
	used map[string]int
}

func NewDeDuper() *DeDuper {
	return &DeDuper{
		used: make(map[string]int),
	}
}

func (self DeDuper) isUnique(name string) bool {
	_, exists := self.used[name]
	return !exists
}

func (self DeDuper) unique(name string) string {
	count := self.used[name]
	count++
	self.used[name] = count
	if count > 1 {
		name += strconv.FormatInt(int64(count), 10)
	}
	return name
}
