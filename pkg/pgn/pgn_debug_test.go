package pgn

import (
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/constraints"
)

// calcMax calculates the actual maximum value for a field
func calcMax(field *FieldDescriptor) uint64 {
	val := calcMaxPositiveValue(field.BitLength, field.Signed, field.ReservedValuesCount) + uint64(field.Offset)
	if field.Resolution != 0 {
		val = val * uint64(field.Resolution)
	}
	return val
}

// calcNumericValue is a generic helper function that handles numeric value calculation
func calcNumericValue[T constraints.Integer | constraints.Float](field *FieldDescriptor, testType int) T {

	if field.Id == "SignalSnr" {
		switch testType {
		case TestTypeRandom:
			// Generate value within range
			val := rand.Float64()*(field.RangeMax-field.RangeMin) + field.RangeMin
			// Quantize to resolution
			val = math.Round(val/float64(field.Resolution)) * float64(field.Resolution)
			return T(val)
		}
	}
	switch testType {
	case TestTypeZero:
		return 0
	case TestTypeMin:
		if field.Offset != 0 {
			return T(field.RangeMin - float64(field.Offset))
		} else if field.DomainMin != 0 {
			return T(field.DomainMin)
		}
		return T(field.RangeMin)
	case TestTypeMax:
		var val float64
		if field.Offset != 0 {
			val = float64(calcMax(field))
		} else {
			if field.DomainMax != 0 {
				val = field.DomainMax
			} else {
				val = field.RangeMax
			}
		}

		// For 32-bit unit types, ensure we don't exceed their range
		if reflect.TypeOf(T(0)).Bits() == 32 {
			if val > float64(math.MaxInt32) {
				val = float64(math.MaxInt32)
			}
			if val < 0 {
				val = float64(math.MinInt32)
			}

			// For unit types, ensure the value is aligned with the resolution
			if field.Resolution > 1 {
				val = math.Round(val/float64(field.Resolution)) * float64(field.Resolution)
			}
		}
		return T(val)
	case TestTypeRandom:
		// Check if T is a floating point type using reflect
		if reflect.TypeOf(T(0)).Kind() == reflect.Float64 || reflect.TypeOf(T(0)).Kind() == reflect.Float32 {
			var max, min float64
			if field.DomainMin != 0 || field.DomainMax != 0 {
				max = field.DomainMax
				min = field.DomainMin
			} else {
				max = float64(calcMax(field))
				min = field.RangeMin
			}
			val := rand.Float64()*(max-min) + min

			// Apply resolution quantization for any non-unity resolution
			if field.Resolution != 1 && field.Resolution != 1.0 {
				val = math.Round(val/float64(field.Resolution)) * float64(field.Resolution)
			}

			// Apply the same 32-bit handling
			if reflect.TypeOf(T(0)).Bits() == 32 {
				if val > float64(math.MaxInt32) {
					val = float64(math.MaxInt32)
				}
				if val < float64(math.MinInt32) {
					val = float64(math.MinInt32)
				}
			}
			return T(val)
		}

		// For integer types, handle large ranges safely
		var min, max float64
		if field.DomainMin != 0 || field.DomainMax != 0 {
			min = field.DomainMin
			max = field.DomainMax
		} else {
			min = field.RangeMin
			max = field.RangeMax
		}

		if max > float64(math.MaxInt) {
			// For large ranges, use Float64 and convert to integer
			range64 := max - min
			randomFloat := rand.Float64() * range64
			return T(randomFloat + min)
		}

		return T(rand.Intn(int(max-min+1)) + int(min))
	}
	return 0
}

// calcValue sets the value of the field to the testType
func calcValue(field *FieldDescriptor, testType int, t reflect.Type) reflect.Value {
	val := reflect.New(t)

	// For TestTypeZero, check if field has reserved values
	if testType == TestTypeZero && field.ReservedValuesCount == 0 {
		// For fields with no reserved values, write actual zero values instead of nil
		// But only for basic numeric types, not custom enum types
		switch t.Kind() {
		case reflect.Int, reflect.Int64:
			val.Elem().Set(reflect.ValueOf(int64(0)).Convert(val.Elem().Type()))
		case reflect.Int8:
			val.Elem().Set(reflect.ValueOf(int8(0)).Convert(val.Elem().Type()))
		case reflect.Int16:
			val.Elem().Set(reflect.ValueOf(int16(0)).Convert(val.Elem().Type()))
		case reflect.Int32:
			val.Elem().Set(reflect.ValueOf(int32(0)).Convert(val.Elem().Type()))
		case reflect.Uint8:
			val.Elem().Set(reflect.ValueOf(uint8(0)).Convert(val.Elem().Type()))
		case reflect.Uint16:
			val.Elem().Set(reflect.ValueOf(uint16(0)).Convert(val.Elem().Type()))
		case reflect.Uint32:
			val.Elem().Set(reflect.ValueOf(uint32(0)).Convert(val.Elem().Type()))
		case reflect.Uint, reflect.Uint64:
			val.Elem().Set(reflect.ValueOf(uint64(0)).Convert(val.Elem().Type()))
		case reflect.Float32:
			val.Elem().Set(reflect.ValueOf(float32(0)).Convert(val.Elem().Type()))
		case reflect.Float64:
			val.Elem().Set(reflect.ValueOf(float64(0)).Convert(val.Elem().Type()))
		default:
			// For custom types (like enums), fall through to normal logic
		}
		// If we handled a basic type, return it; otherwise fall through to normal logic
		if t.Kind() >= reflect.Int && t.Kind() <= reflect.Float64 {
			return val
		}
	}

	switch t.Kind() {
	case reflect.Int, reflect.Int64:
		result := calcNumericValue[int64](field, testType)
		val.Elem().Set(reflect.ValueOf(result))
	case reflect.Int8:
		result := calcNumericValue[int8](field, testType)
		val.Elem().Set(reflect.ValueOf(result))
	case reflect.Int16:
		result := calcNumericValue[int16](field, testType)
		val.Elem().Set(reflect.ValueOf(result))
	case reflect.Int32:
		result := calcNumericValue[int32](field, testType)
		val.Elem().Set(reflect.ValueOf(result))
	case reflect.Uint8:
		result := calcNumericValue[uint8](field, testType)
		val.Elem().Set(reflect.ValueOf(result).Convert(val.Elem().Type()))
	case reflect.Uint16:
		result := calcNumericValue[uint16](field, testType)
		val.Elem().Set(reflect.ValueOf(result).Convert(val.Elem().Type()))
	case reflect.Uint32:
		result := calcNumericValue[uint32](field, testType)
		val.Elem().Set(reflect.ValueOf(result))
	case reflect.Uint, reflect.Uint64:
		result := calcNumericValue[uint64](field, testType)
		val.Elem().Set(reflect.ValueOf(result))
	case reflect.Float32:
		result := calcNumericValue[float32](field, testType)
		val.Elem().Set(reflect.ValueOf(result))
	case reflect.Float64:
		result := calcNumericValue[float64](field, testType)
		val.Elem().Set(reflect.ValueOf(result))
	}
	return val
}

func genBytes(numBytes uint16, testType int, field *FieldDescriptor) []uint8 {
	switch testType {
	case TestTypeZero, TestTypeMin, TestTypeMax:
		bytes := make([]uint8, numBytes)
		return bytes
	case TestTypeRandom:
		switch field.CanboatType {
		case "STRING_FIX":
			// if field.BitLength is a multiple of 8, then it's a fixed length string
			var length int
			if field.BitLength%8 == 0 {
				length = int(field.BitLength / 8)
			} else {
				// generate ascii string of length 1 to random length less than numBytes
				length = rand.Intn(int(numBytes)) + 1
			}
			bytes := make([]uint8, numBytes)
			for i := 0; i < length; i++ {
				bytes[i] = uint8(rand.Intn(26) + 65) // ASCII A-Z
			}
			// pad with 0, 0xFF, or "@" characters
			padSeed := uint8(rand.Intn(3))
			padChar := uint8('@')
			switch padSeed {
			case 0:
				padChar = uint8(0)
			case 1:
				padChar = uint8(0xFF)
			}
			for i := length; i < int(numBytes); i++ {
				bytes[i] = padChar
			}
			return bytes
		case "STRING_LAU":
			isUnicode := rand.Intn(2) == 0
			if isUnicode {
				// generate unicode string of random length between 5 and 20
				length := rand.Intn(int(15)) + 5
				// if even, add 1 byte to length for null terminator
				if length%2 == 0 {
					length++
				}
				bytes := make([]uint8, length)
				bytes[0] = uint8(length) // and byte 1 is 0 for Unicode
				numRunes := length / 2
				for i := 0; i < numRunes; i++ {
					bytes[i*2+1] = uint8(rand.Intn(65536)) // Unicode range 0-65535
				} // null terminator is at the end
				return bytes
			} else {
				// generate ascii string of length 1 to random length less than numBytes
				length := rand.Intn(int(numBytes)) + 1
				bytes := make([]uint8, length)
				for i := 0; i < length; i++ {
					bytes[i] = uint8(rand.Intn(26) + 65) // ASCII A-Z
				}
				// mask to field.BitLength if field.BitLength is not multiple of 8
				if field.BitLength%8 != 0 {
					bytes[len(bytes)-1] = bytes[len(bytes)-1] & uint8(field.BitLength%8)
				}
				return bytes
			}
		case "STRING_LZ":
			// generate ascii string of length 1 to random length less than numBytes
			// STRING_LZ has a length byte and is null terminated
			length := rand.Intn(int(numBytes)-2) + 1
			bytes := make([]uint8, length+2)
			bytes[0] = uint8(length)
			for i := 0; i < length; i++ {
				bytes[i+1] = uint8(rand.Intn(26) + 65) // ASCII A-Z
			}
			return bytes
		case "BINARY", "VARIABLE":
			limit := numBytes
			if limit == 0 {
				limit = 8
			}
			bytes := make([]uint8, limit)
			for i := 0; i < int(limit); i++ {
				bytes[i] = uint8(rand.Intn(256))
			}
			if field.BitLength%8 != 0 {
				bytes[len(bytes)-1] = bytes[len(bytes)-1] & uint8(field.BitLength%8)
			}
			return bytes
		default: // do nothing
		}
	}
	return []uint8{}
}

// initializeElement sets the value of the field to the testType
func initializeElement(elem reflect.Value, testType int) {
	if !elem.IsValid() || elem.Kind() != reflect.Ptr {
		return
	}

	elemValue := elem.Elem()
	for i := 0; i < elemValue.NumField(); i++ {
		field := elemValue.Field(i)
		switch testType {
		case TestTypeZero:
			// Zero values are already set by default
			continue

		case TestTypeMin:
			switch field.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				field.SetInt(math.MinInt64)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				field.SetUint(0)
			case reflect.Float32, reflect.Float64:
				field.SetFloat(-math.MaxFloat64)
			case reflect.Bool:
				field.SetBool(false)
			case reflect.String:
				field.SetString("")
			case reflect.Slice:
				field.Set(reflect.MakeSlice(field.Type(), 0, 0))
			case reflect.Ptr:
				if !field.IsNil() {
					field.Set(reflect.Zero(field.Type()))
				}
			}

		case TestTypeMax:
			switch field.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				field.SetInt(math.MaxInt64)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				field.SetUint(math.MaxUint64)
			case reflect.Float32, reflect.Float64:
				field.SetFloat(math.MaxFloat64)
			case reflect.Bool:
				field.SetBool(true)
			case reflect.String:
				field.SetString("ZZZZZZZZZZ") // Max-like string value
			case reflect.Slice:
				field.Set(reflect.MakeSlice(field.Type(), 10, 10)) // Max-like slice length
			}

		case TestTypeRandom:
			switch field.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				field.SetInt(rand.Int63())
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				field.SetUint(rand.Uint64())
			case reflect.Float32, reflect.Float64:
				field.SetFloat(rand.Float64())
			case reflect.Bool:
				field.SetBool(rand.Intn(2) == 1)
			case reflect.String:
				length := rand.Intn(20) + 1
				bytes := make([]byte, length)
				for i := range bytes {
					bytes[i] = byte(rand.Intn(26) + 65) // A-Z
				}
				field.SetString(string(bytes))
			case reflect.Slice:
				length := rand.Intn(5) + 1
				field.Set(reflect.MakeSlice(field.Type(), length, length))
			}
		}
	}
}

// Then modify setRepeatingState to use this new function
func setRepeatingState(o reflect.Value, testType int) {
	if !o.IsValid() {
		return
	}

	switch testType {
	case TestTypeZero:
		o.Set(reflect.MakeSlice(o.Type(), 1, 1))
		elem := reflect.New(o.Type().Elem())
		o.Index(0).Set(elem.Elem())
		initializeElement(elem, testType)

	case TestTypeMin, TestTypeMax:
		o.Set(reflect.MakeSlice(o.Type(), 1, 1))
		elem := reflect.New(o.Type().Elem())
		o.Index(0).Set(elem.Elem())
		initializeElement(elem, testType)

	case TestTypeRandom:
		numElems := rand.Intn(3) + 1
		o.Set(reflect.MakeSlice(o.Type(), numElems, numElems))
		for i := 0; i < numElems; i++ {
			elem := reflect.New(o.Type().Elem())
			o.Index(i).Set(elem.Elem())
			initializeElement(elem, testType)
		}
	}
}

// setState sets the state of the object o to the testType
func setState(o reflect.Value, n PgnInfo, testType int) {
	nextOffset := uint16(0)
	// first check if fields named Repeating1 or Repeating2 exist
	repeating1 := o.Elem().FieldByName("Repeating1")
	repeating2 := o.Elem().FieldByName("Repeating2")
	if repeating1.IsValid() && repeating1.IsNil() {
		//		repeating1.Set(reflect.MakeSlice(repeating1.Type(), 1, 1))
		setRepeatingState(repeating1, testType)
	}
	if repeating2.IsValid() && repeating2.IsNil() {
		//		repeating2.Set(reflect.MakeSlice(repeating2.Type(), 1, 1))
		setRepeatingState(repeating2, testType)
	}
	for i := 1; i <= len(n.Fields); i++ { // n.Fields uses field.Order, which starts at 1
		field := n.Fields[i]
		fieldValue := o.Elem().FieldByName(field.Id)
		if field.BitOffset != 0 {
			nextOffset = field.BitOffset + field.BitLength
		}
		// Handle Match value for uint8/uint16, pointer and non-pointer
		if field.Match != -1 {
			if fieldValue.Type().Kind() == reflect.Ptr {
				// For pointer types (*uint8, *uint16)
				newVal := reflect.New(fieldValue.Type().Elem())
				newVal.Elem().SetUint(uint64(field.Match))
				fieldValue.Set(newVal)
			} else {
				// For non-pointer types (uint8, uint16)
				fieldValue.SetUint(uint64(field.Match))
			}
			continue
		}
		if n.Repeating1CountField != 0 && field.Order == n.Repeating1CountField {
			length := uint64(repeating1.Len())
			value := reflect.New(fieldValue.Type().Elem())
			value.Elem().SetUint(length)
			fieldValue.Set(value)
			continue
		}
		if n.Repeating2CountField != 0 && field.Order == n.Repeating2CountField {
			length := uint64(repeating2.Len())
			value := reflect.New(fieldValue.Type().Elem())
			value.Elem().SetUint(length)
			fieldValue.Set(value)
			continue
		}
		// Then handle the different types
		if strings.HasPrefix(field.GolangType, "*units") {
			fieldValue := o.Elem().FieldByName(field.Id)
			if !fieldValue.IsValid() {
				continue
			}
			fType := fieldValue.Type()
			unitVal := reflect.New(fType.Elem())

			// Set the Value field of the unit type
			valueField := unitVal.Elem().FieldByName("Value")
			if valueField.IsValid() {
				// Dereference the pointer returned by calcValue
				calculatedValue := calcValue(field, testType, valueField.Type())
				if calculatedValue.IsValid() {
					valueField.Set(calculatedValue.Elem())
				}
			}

			o.Elem().FieldByName(field.Id).Set(unitVal)
			continue
		} else if strings.HasPrefix(field.GolangType, "*") {
			fieldValue := o.Elem().FieldByName(field.Id)
			if !fieldValue.IsValid() {
				continue
			}
			fType := fieldValue.Type()
			o.Elem().FieldByName(field.Id).Set(calcValue(field, testType, fType.Elem()))
			continue
		}

		switch field.GolangType {
		case "": // skip Reserved and Spare canboat types
			continue
		case "*uint8":
			fieldValue := o.Elem().FieldByName(field.Id)
			if !fieldValue.IsValid() {
				continue
			}
			calcResult := calcValue(field, testType, fieldValue.Type().Elem())
			if calcResult.IsValid() {
				fieldValue.Set(calcResult)
			}
		case "*uint16":
			fieldValue := o.Elem().FieldByName(field.Id)
			if !fieldValue.IsValid() {
				continue
			}
			calcResult := calcValue(field, testType, fieldValue.Type().Elem())
			if calcResult.IsValid() {
				// Special handling for NumberOfBitsInBinaryDataField to ensure it doesn't exceed buffer capacity
				if field.Id == "NumberOfBitsInBinaryDataField" {
					value := calcResult.Elem().Uint()
					// Limit to a reasonable value that fits in the buffer
					maxBits := uint64(254*8 - nextOffset) // 254 bytes * 8 bits - bits already used
					if value > maxBits {
						value = maxBits
					}
					// Create a new value with the limited value
					newVal := reflect.New(fieldValue.Type().Elem())
					newVal.Elem().SetUint(value)
					fieldValue.Set(newVal)
				} else {
					fieldValue.Set(calcResult)
				}
			}
		case "[]uint8":
			numBytes := uint16(0)
			if !field.BitLengthVariable {
				numBytes = uint16(math.Ceil(float64(field.BitLength) / 8))
			} else { // bitLengthVariable is true, we need to calculate max remaining bytes
				// if there's a field named NumberOfBitsInBinaryDataField, use it to calculate numBytes
				numberOfBitsInBinaryDataField := o.Elem().FieldByName("NumberOfBitsInBinaryDataField")
				if numberOfBitsInBinaryDataField.IsValid() {
					if numberOfBitsInBinaryDataField.Kind() == reflect.Ptr {
						numBytes = uint16(numberOfBitsInBinaryDataField.Elem().Uint()) / 8
					} else {
						numBytes = uint16(numberOfBitsInBinaryDataField.Uint()) / 8
					}
				} else {
					numBytes = MaxPGNLength - (nextOffset / 8)
				}

				// Ensure we don't exceed the available buffer space
				maxAvailableBytes := uint16(254) - (nextOffset / 8)
				if numBytes > maxAvailableBytes {
					numBytes = maxAvailableBytes
				}
			}
			o.Elem().FieldByName(field.Id).Set(reflect.ValueOf(genBytes(numBytes, testType, field)))
		default:
			fieldValue := o.Elem().FieldByName(field.Id)
			if !fieldValue.IsValid() {
				continue
			}
			calcResult := calcValue(field, testType, fieldValue.Type())
			if calcResult.IsValid() {
				// Special handling for NumberOfBitsInBinaryDataField to ensure it doesn't exceed buffer capacity
				if field.Id == "NumberOfBitsInBinaryDataField" {
					value := calcResult.Elem().Uint()
					// Limit to a reasonable value that fits in the buffer
					maxBits := uint64(254*8 - nextOffset) // 254 bytes * 8 bits - bits already used
					if value > maxBits {
						value = maxBits
					}
					// Create a new value with the limited value
					newVal := reflect.New(fieldValue.Type().Elem())
					newVal.Elem().SetUint(value)
					fieldValue.Set(newVal)
				} else {
					fieldValue.Set(calcResult.Elem())
				}
			}
		}
	}
}

// consts for test types: zero, min, max, random
const (
	TestTypeZero   = iota // All fields set to zero/nil/empty values
	TestTypeMin           // All numeric fields set to minimum allowed values
	TestTypeMax           // All numeric fields set to maximum allowed values
	TestTypeRandom        // All fields set to random valid values
)

func TestPgns(t *testing.T) {
	var mInfo *MessageInfo
	for i := range pgnList {
		for _, testType := range []int{TestTypeZero, TestTypeMin, TestTypeMax, TestTypeRandom} {
			next := pgnList[i]
			/* if next.Id != "AisAddressedBinaryMessage" {
				continue
			} */
			if strings.HasPrefix(next.Id, "Nmea") {
				continue
			}

			decoder := next.Decoder

			oType := reflect.ValueOf(next.Instance).Elem().Type()
			original := reflect.New(oType)
			setState(original, next, testType)
			buffer := make([]uint8, 254)
			stream := NewDataStream(buffer)
			if oPgn, ok := original.Interface().(PgnStruct); ok {
				info, err := oPgn.Encode(stream)
				mInfo = info
				assert.NoError(t, err)
				if err != nil {
					t.Errorf("PGN %s testType %d ", next.Id, testType)
					continue
				}
			}
			stream.data = stream.data[0:stream.byteOffset] // trim stream data to length
			stream.resetToStart()
			decoded, err := decoder(*mInfo, stream)
			assert.NoError(t, err)

			// Ignore timestamp field, as it's not part of Encode/Decode
			//	original.Info.Timestamp = time.Time{}
			//	decodedIso.Info.Timestamp = time.Time{}

			// ... in your test:

			opts := cmp.Options{
				cmpopts.EquateEmpty(),
				cmpopts.EquateApprox(0.001, 0.001),
			}

			diff := cmp.Diff(original.Elem().Interface(), decoded, opts)
			if diff != "" {
				t.Errorf("PGN %s testType %d ", next.Id, testType)
			}
			assert.Empty(t, diff) // diff will be empty if the structs are equal
		}
	}
}

func TestFieldResolutions(t *testing.T) {
	for _, pgn := range pgnList {
		for _, field := range pgn.Fields {
			if field.Resolution != 0 && field.Resolution != 1 && field.Resolution != 1.0 {
				if !strings.HasPrefix(field.GolangType, "*units") &&
					field.GolangType != "*float32" &&
					field.GolangType != "*float64" {
					t.Errorf("PGN %s field %s has resolution %f but type %s",
						pgn.Id, field.Id, field.Resolution, field.GolangType)
				}
			}
		}
	}
}
