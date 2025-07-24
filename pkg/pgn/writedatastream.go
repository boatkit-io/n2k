package pgn

import (
	"fmt"
	"math"
	"reflect"

	"github.com/boatkit-io/tugboat/pkg/units"
)

// writeReserved fills the specified number of bits at the specified offset with 1s
func (s *DataStream) writeReserved(bitLength uint16, bitOffset uint16) error {
	return s.putNumberRaw(0xFFFFFFFFFFFFFFFF, bitLength, bitOffset)
}

// writeSpare fills the specified number of bits at the specified offset with 0s
func (s *DataStream) writeSpare(bitLength uint16, bitOffset uint16) error {
	return s.putNumberRaw(0, bitLength, bitOffset)
}

// writeStringLau writes the specified length of value at the specified offset
func (s *DataStream) writeStringLau(value string, bitOffset uint16) error {
	var out []uint8
	if len(value) == 0 {
		out = []uint8{0x2, 0x1} // we'll encode as UTF8
	} else {
		out = []uint8{
			uint8(len(value) + 3),
			0x1} // we'll encode as UTF8
		out = append(out, value...)
		out = append(out, 0x0)
	}
	length := uint16(len(out) * 8)
	return s.writeBinary(out, length, bitOffset)
}

// writeStringWithLength writes the specified length of value at the specified offset
func (s *DataStream) writeStringWithLength(value string, bitLength uint16, bitOffset uint16) error {
	length := uint8(len(value)) + 1 //  string length plus terminator
	fieldLength := uint8(bitLength / 8)
	if length+1 > fieldLength { // field must contain the length byte, the string, and the terminator
		return fmt.Errorf("attempt to write string with length longer than field's length")
	}
	out := make([]uint8, fieldLength) // allocate the field's length, filled with zeros
	out[0] = uint8(length)
	for i := range value {
		out[i+1] = value[i]
	}
	return s.writeBinary(out, bitLength, bitOffset)
}

// writeStringFix writes the fixed string, first padding its length as necessary.
// padding has been seen as "@", 0x00, and 0xff. we use the latter.
func (s *DataStream) writeStringFix(value []uint8, bitLength uint16, bitOffset uint16) error {
	byteCount := bitLength / 8
	for i := len(value); i < int(byteCount); i++ {
		value = append(value, 0xff)
	}
	return s.writeBinary(value, bitLength, bitOffset)

}

// writeBinary writes the specified length of value at the specified offset
func (s *DataStream) writeBinary(value []uint8, bitLength uint16, bitOffset uint16) error {
	var numBytes uint16
	if s.getBitOffset() != uint32(bitOffset) && bitOffset != 0 { // bitOffset == 0 can mean we don't know the offset, sadly
		return fmt.Errorf("attempt to write field at wrong offset in writeBinary: %d, %d", s.getBitOffset(), bitOffset)
	}
	// if length of value in bits is less than bitlength, pad with 0 (FF?)
	// Binary values always start on a byte boundary, so we don't have to worry about the field being misaligned.
	// the value can be any bit length, so we need to update the datastream fields after moving the slice in
	if bitLength == 0 { // we'll write the whole value assuming it fits
		numBytes = uint16(len(value))
	} else { // we'll write the value up to the bitlength
		numBytes = uint16(math.Ceil(float64(bitLength) / 8))
	}
	if numBytes > MaxPGNLength-(bitOffset/8) {
		numBytes = MaxPGNLength - (bitOffset / 8)
		if numBytes == 0 {
			return fmt.Errorf("attempt to write binary field at maximum pgn length")
		}
	}
	if bitLength != 0 { // bitlengthVariable is false, we write the bits we have. No way to specify #bits, so always mod 8=0
		if value == nil {
			value = make([]uint8, numBytes)
		}
		if uint16(len(value)) < numBytes {
			value = append(value, make([]uint8, (int(numBytes)-len(value)))...)
		}
	}
	if s.bitOffset != 0 { // must be byte aligned field
		return fmt.Errorf("BINARY field must be byte aligned")
	}
	for index := 0; index < int(numBytes); index++ {
		s.data[s.byteOffset] = value[index]
		s.byteOffset++
	}
	oddBits := uint8(bitLength % 8)
	if oddBits != 0 {
		s.byteOffset--
		s.bitOffset = uint8(bitLength % 8)
		s.data[s.byteOffset] &= uint8(0xFF) >> (8 - oddBits)
	}
	return nil
}

// writeInt8 writes the specified length of the signed value at the specified offset
func (s *DataStream) writeInt8(value *int8, length uint16, bitOffset uint16, reservedValuesCount int) error {
	var value64 *int64
	if value != nil {
		value64 = new(int64)
		*value64 = int64(*value)
	}
	return s.writeSignedNumber(value64, length, bitOffset, reservedValuesCount)
}

// writeInt16 writes the specified length of the signed value at the specified offset
func (s *DataStream) writeInt16(value *int16, length uint16, bitOffset uint16, reservedValuesCount int) error {
	var value64 *int64
	if value != nil {
		value64 = new(int64)
		*value64 = int64(*value)
	}
	return s.writeSignedNumber(value64, length, bitOffset, reservedValuesCount)
}

// writeInt32 writes the specified length of the signed value at the specified offset
func (s *DataStream) writeInt32(value *int32, length uint16, bitOffset uint16, reservedValuesCount int) error {
	var value64 *int64
	if value != nil {
		value64 = new(int64)
		*value64 = int64(*value)
	}
	return s.writeSignedNumber(value64, length, bitOffset, reservedValuesCount)
}

// writeInt64 writes the specified length of the signed value at the specified offset
//
//lint:ignore U1000 // future
func (s *DataStream) writeInt64(value *int64, length uint16, bitOffset uint16, reservedValuesCount int) error {

	return s.writeSignedNumber(value, length, bitOffset, reservedValuesCount)
}

// writeUint8 writes the specified length of the unsigned value at the specified offset
func (s *DataStream) writeUint8(value *uint8, length uint16, bitOffset uint16, reservedValuesCount int) error {
	var value64 *uint64
	if value != nil {
		value64 = new(uint64)
		*value64 = uint64(*value)
	}
	return s.writeUnsignedNumber(value64, length, bitOffset, reservedValuesCount)
}

// writeUint16 writes the specified length of the unsigned value at the specified offset
func (s *DataStream) writeUint16(value *uint16, length uint16, bitOffset uint16, reservedValuesCount int) error {
	var value64 *uint64
	if value != nil {
		value64 = new(uint64)
		*value64 = uint64(*value)
	}
	return s.writeUnsignedNumber(value64, length, bitOffset, reservedValuesCount)
}

// writeUint32 writes the specified length of the unsigned value at the specified offset
func (s *DataStream) writeUint32(value *uint32, length uint16, bitOffset uint16, reservedValuesCount int) error {
	var value64 *uint64
	if value != nil {
		value64 = new(uint64)
		*value64 = uint64(*value)
	}
	return s.writeUnsignedNumber(value64, length, bitOffset, reservedValuesCount)
}

// writeUint64 writes the specified length of the unsigned value at the specified offset
func (s *DataStream) writeUint64(value *uint64, length uint16, bitOffset uint16, reservedValuesCount int) error {
	return s.writeUnsignedNumber(value, length, bitOffset, reservedValuesCount)
}

// writeUnsignedNumber writes the specified length of the unsigned value at the specified offset
func (s *DataStream) writeUnsignedNumber(value *uint64, length uint16, bitOffset uint16, reservedValuesCount int) error {
	var outVal uint64
	if value == nil {
		if reservedValuesCount == 0 {
			return fmt.Errorf("cannot write nil value to field with no reserved values")
		}
		outVal = missingValue(length, false, reservedValuesCount)
	} else {
		outVal = *value
		if reservedValuesCount > 0 {
			maxVal := calcMaxPositiveValue(length, false, reservedValuesCount)
			if outVal > maxVal {
				//			return fmt.Errorf("attempt to write unsigned value greater than max value")
				outVal = maxVal
			}
		}
		// If reservedValuesCount == 0, no validation needed - all values are valid
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

// writeSignedNumber writes the specified length of the signed value at the specified offset
func (s *DataStream) writeSignedNumber(value *int64, length uint16, bitOffset uint16, reservedValuesCount int) error {
	var outVal uint64
	if value == nil {
		if reservedValuesCount == 0 {
			return fmt.Errorf("cannot write nil value to field with no reserved values")
		}
		outVal = missingValue(length, true, reservedValuesCount)
	} else {
		outVal = uint64(*value)
		if reservedValuesCount > 0 {
			maxVal := calcMaxPositiveValue(length, true, reservedValuesCount)
			if *value > int64(maxVal) {
				//			return fmt.Errorf("attempt to write signed value greater than max value")
				outVal = maxVal
			}
		}
		// If reservedValuesCount == 0, no validation needed - all values are valid
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

// checkNilInterface returns true if the interface is nil
func checkNilInterface(i interface{}) bool {
	iv := reflect.ValueOf(i)
	if !iv.IsValid() {
		return true
	}
	switch iv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Func, reflect.Interface:
		return iv.IsNil()
	default:
		return false
	}
}

// writeUnit writes units package values
// value must be converted to the canboat unit before calling
func (s *DataStream) writeUnit(value any, length uint16, resolution float32, bitOffset uint16, offset int64, signed bool, reservedValuesCount int) error {
	if checkNilInterface(value) {
		if signed {
			return s.writeSignedResolution32(nil, length, resolution, bitOffset, offset, reservedValuesCount)
		}
		return s.writeUnsignedResolution32(nil, length, resolution, bitOffset, offset, reservedValuesCount)
	}

	// Convert to canboat's default unit based on the type
	var canboatValue float32
	switch v := value.(type) {
	case *units.Distance:
		canboatValue = v.Convert(units.Meter).Value
	case *units.Velocity:
		canboatValue = v.Convert(units.MetersPerSecond).Value
	case *units.Volume:
		canboatValue = v.Convert(units.Liter).Value
	case *units.Temperature:
		canboatValue = v.Convert(units.Kelvin).Value
	case *units.Pressure:
		canboatValue = v.Convert(units.Pa).Value
	case *units.Flow:
		canboatValue = v.Convert(units.LitersPerHour).Value
	default:
		// invalid unit, return error
		return fmt.Errorf("invalid unit type: %T", value)
	}

	if signed {
		return s.writeSignedResolution32(&canboatValue, length, resolution, bitOffset, offset, reservedValuesCount)
	}
	return s.writeUnsignedResolution32(&canboatValue, length, resolution, bitOffset, offset, reservedValuesCount)
}

// writeFloat32 writes the specified length of value at the specified offset
func (s *DataStream) writeFloat32(value *float32, bitLength uint16, bitOffset uint16, reservedValuesCount int) error {

	return s.writeSignedResolution32(value, bitLength, 1, bitOffset, 0, reservedValuesCount)
}

// writeFloat64 writes the specified length of value at the specified offset
/* func (s *DataStream) writeFloat64(value *float64, bitLength uint16, bitOffset uint16) error {

	return s.writeSignedResolution64(value, bitLength, 1, bitOffset, 0)
} */

// writeSignedResolution32 backs out the resolution and offset and writes the resulting signed value
func (s *DataStream) writeSignedResolution32(value *float32, length uint16, resolution float32, bitOffset uint16, offset int64, reservedValuesCount int) error {
	var value64 *float64
	if value != nil {
		value64 = new(float64)
		*value64 = float64(*value)
	}
	return s.writeSignedResolution64(value64, length, float64(resolution), bitOffset, offset, reservedValuesCount)
}

// writeSignedResolution64 backs out the resolution and offset and writes the resulting signed value
func (s *DataStream) writeSignedResolution64(value *float64, length uint16, resolution float64, bitOffset uint16, offset int64, reservedValuesCount int) error {
	if value == nil {
		if reservedValuesCount == 0 {
			return fmt.Errorf("cannot write nil value to field with no reserved values")
		}
		return s.putNumberRaw(missingValue(length, true, reservedValuesCount), length, bitOffset)
	}

	// For 32-bit fields, preserve IEEE 754 float bit pattern if no resolution/offset
	if length == 32 && resolution == 1 && offset == 0 {
		bits := math.Float32bits(float32(*value))
		return s.putNumberRaw(uint64(bits), length, bitOffset)
	}

	val := *value

	// First subtract offset
	val -= float64(offset)

	// Then apply resolution scaling
	if resolution != 0 && resolution != 1 && resolution != 1.0 {
		prec := calcPrecision(resolution)
		scaledVal := val / float64(resolution)
		roundedVal := roundFloat(scaledVal, prec)
		val = roundedVal
	}

	// For 32-bit fields, check for overflow before conversion
	if length == 32 {
		if val >= 0 {
			if val > float64(math.MaxInt32-2) {
				val = float64(math.MaxInt32 - 2) // Leave room for reserved values
			}
		} else {
			if val < float64(math.MinInt32) {
				val = float64(math.MinInt32)
			}
		}
		intVal := int32(val)
		return s.putNumberRaw(uint64(intVal), length, bitOffset)
	}

	return s.putNumberRaw(uint64(int64(val)), length, bitOffset)
}

// writeUnsignedResolution32 backs out the resolution and offset and writes the resulting unsigned value
func (s *DataStream) writeUnsignedResolution32(value *float32, length uint16, resolution float32, bitOffset uint16, offset int64, reservedValuesCount int) error {
	var value64 *float64
	if value != nil {
		value64 = new(float64)
		*value64 = float64(*value)
	}
	return s.writeUnsignedResolution64(value64, length, float64(resolution), bitOffset, offset, reservedValuesCount)
}

// writeUnsignedResolution64 backs out the resolution and offset and writes the resulting unsigned value
func (s *DataStream) writeUnsignedResolution64(value *float64, length uint16, resolution float64, bitOffset uint16, offset int64, reservedValuesCount int) error {
	var outVal uint64
	var val float64
	if value == nil {
		if reservedValuesCount == 0 {
			return fmt.Errorf("cannot write nil value to field with no reserved values")
		}
		outVal = missingValue(length, false, reservedValuesCount)
	} else {
		val = *value
		if resolution != 0 && resolution != 1 && resolution != 1.0 {
			val = val * float64(1.0/resolution)
			val = roundFloat(val, calcPrecision(float64(resolution)))
		}
		val -= float64(offset)
		outVal = uint64(val)
		if reservedValuesCount > 0 {
			maxValid := calcMaxPositiveValue(length, false, reservedValuesCount)
			if outVal > maxValid {
				outVal = maxValid // pin at maximum value
			}
		}
		// If reservedValuesCount == 0, no validation needed - all values are valid
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

// putNumberRaw method writes up to 64 bits to the stream from a uint64 argument.
// Cribbed the getNumberRaw function
func (s *DataStream) putNumberRaw(value uint64, bitLength uint16, bitOffset uint16) error {
	if s.getBitOffset() != uint32(bitOffset) && bitOffset != 0 { // bitOffset == 0 can mean we don't know the offset, sadly
		return fmt.Errorf("attempt to write field at wrong offset in putNumberRaw: %d, %d", s.getBitOffset(), bitOffset)
	}

	for bitLength > 0 {
		if int(s.byteOffset) >= cap(s.data) {
			return fmt.Errorf("attempt to write byte(%d) off end of pgn (len:%d)", s.byteOffset, cap(s.data))
		}

		startBit := s.bitOffset
		bitsLeft := 8 - startBit
		bitsToWrite := bitsLeft
		if bitLength < uint16(bitsLeft) { // also we could be writing less than 8 bits
			bitsToWrite = uint8(bitLength)
		}

		mask := uint8(0xFF >> uint8(8-bitsToWrite))
		outByte := uint8(value) & mask
		if bitsToWrite <= bitsLeft {
			outByte <<= (startBit)
		}

		value >>= uint64(bitsToWrite)
		s.data[s.byteOffset] |= uint8(outByte)
		bitLength -= uint16(bitsToWrite)
		s.bitOffset += bitsToWrite
		if s.bitOffset >= 8 {
			s.bitOffset -= 8
			s.byteOffset++
		}
	}
	return nil
}
