package pgn

import (
	"fmt"
	"math"
	"reflect"

	"github.com/boatkit-io/tugboat/pkg/units"
)

func (s *DataStream) writeReserved(bitLength uint16, bitOffset uint16) error {
	return s.putNumberRaw(0xFFFFFFFFFFFFFFFF, bitLength, bitOffset)
}

func (s *DataStream) writeSpare(bitLength uint16, bitOffset uint16) error {
	return s.putNumberRaw(0, bitLength, bitOffset)
}

func (s *DataStream) writeFloat32(value float32, bitLength uint16, bitOffset uint16) error {

	return s.putNumberRaw(uint64(math.Float32bits(value)), bitLength, bitOffset)
}

func (s *DataStream) writeStringLau(value string, bitLength uint16, bitOffset uint16) error {
	out := []uint8{
		uint8(len(value) + 3),
		0x0,
	}
	out = append(out, value...)
	out = append(out, 0x0)
	return s.writeBinary(out, bitLength, bitOffset)
}

func (s *DataStream) writeStringWithLength(value string, bitLength uint16, bitOffset uint16) error {
	length := uint8(len(value)) + 1 //  string length plus terminator
	fieldLength := uint8(bitLength % 8)
	if length+1 > fieldLength { // field must contain the length byte, the string, and the terminator
		return fmt.Errorf("Attempt to write string with length longer than field's length")
	}
	out := make([]uint8, fieldLength, fieldLength) // allocate the field's length, filled with zeros
	out[0] = uint8(length)
	for i := range value {
		out[i+1] = value[i]
	}
	return s.writeBinary(out, bitLength, bitOffset)
}

func (s *DataStream) writeBinary(value []uint8, bitLength uint16, bitOffset uint16) error {
	if s.getBitOffset() != uint32(bitOffset) && bitOffset != 0 { // bitOffset == 0 can mean we don't know the offset, sadly
		return fmt.Errorf("attempt to write field at wrong offset in putNumberRaw: %d, %d", s.getBitOffset(), bitOffset)
	}
	// if length of value in bits is less than bitlength, we'll call it an error for now
	// Binary values always start on a byte boundary, so we don't have to worry about the field being misaligned.
	// the value can be any bit length, so we need to update the datastream fields after moving the slice in
	bitsAvailable := uint16(len(value) * 8)
	if bitLength != 0 { // bitlengthVariable is false, we write the bits we have. No way to specify #bits, so always mod 8=0
		if bitsAvailable < bitLength {
			return fmt.Errorf("Binary value must be >= size to be written. Have: %d, Need: %d", bitsAvailable, bitLength)
		} else {
			bitsAvailable = bitLength
		}
	}
	if s.bitOffset != 0 { // must be byte aligned field
		return fmt.Errorf("BINARY field must be byte aligned.")
	}
	numBytes := uint16(math.Ceil(float64(bitLength) / 8))
	for index := 0; index < int(numBytes); index++ {
		s.data[s.byteOffset] = value[index]
		s.byteOffset++
	}
	oddBits := uint8(bitLength % 8)
	if oddBits != 0 {
		s.byteOffset--
		s.bitOffset = uint8(bitLength % 8)
		s.data[s.byteOffset] &= (0xFF << oddBits)
	}
	return nil
}

func (s *DataStream) writeInt8(value *int8, length uint16, bitOffset uint16) error {
	var outVal, maxVal uint64
	maxVal = calcMaxPositiveValue(length, true)
	if value == nil || uint64(*value) > maxVal {
		outVal = missingValue(length, true)
	} else {
		outVal = uint64(*value)
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeInt16(value *int16, length uint16, bitOffset uint16) error {
	var outVal, maxVal uint64
	maxVal = calcMaxPositiveValue(length, true)
	if value == nil || uint64(*value) > maxVal {
		outVal = missingValue(length, true)
	} else {
		outVal = uint64(*value)
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeInt32(value *int32, length uint16, bitOffset uint16) error {
	var outVal, maxVal uint64
	maxVal = calcMaxPositiveValue(length, true)
	if value == nil || uint64(*value) > maxVal {
		outVal = missingValue(length, true)
	} else {
		outVal = uint64(*value)
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeInt64(value *int64, length uint16, bitOffset uint16) error {
	var outVal, maxVal uint64
	maxVal = calcMaxPositiveValue(length, true)
	if value == nil || uint64(*value) > maxVal {
		outVal = missingValue(length, true)
	} else {
		outVal = uint64(*value)
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeUint8(value *uint8, length uint16, bitOffset uint16) error {
	var outVal, maxVal uint64
	maxVal = calcMaxPositiveValue(length, false)
	if value == nil || uint64(*value) > maxVal {
		outVal = missingValue(length, false)
	} else {
		outVal = uint64(*value)
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeUint16(value *uint16, length uint16, bitOffset uint16) error {
	var outVal, maxVal uint64
	maxVal = calcMaxPositiveValue(length, false)
	if value == nil || uint64(*value) > maxVal {
		outVal = missingValue(length, false)
	} else {
		outVal = uint64(*value)
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeUint32(value *uint32, length uint16, bitOffset uint16) error {
	var outVal, maxVal uint64
	maxVal = calcMaxPositiveValue(length, false)
	if value == nil || uint64(*value) > maxVal {
		outVal = missingValue(length, false)
	} else {
		outVal = uint64(*value)
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeUint64(value *uint64, length uint16, bitOffset uint16) error {
	var outVal, maxVal uint64
	maxVal = calcMaxPositiveValue(length, false)
	if value == nil || uint64(*value) > maxVal {
		outVal = missingValue(length, false)
	} else {
		outVal = uint64(*value)
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

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

func (s *DataStream) writeUnit(value units.Units, length uint16, divideBy float32, bitOffset uint16, offset int64) error {
	var val *float32
	if checkNilInterface(value) {
		return s.writeSignedResolution32(nil, length, divideBy, bitOffset, offset)
	}
	val = value.GetValue()
	return s.writeSignedResolution32(val, length, divideBy, bitOffset, offset)
}

func (s *DataStream) writeSignedResolution32(value *float32, length uint16, divideBy float32, bitOffset uint16, offset int64) error {
	var value64 *float64
	if value != nil {
		value64 = new(float64)
		*value64 = float64(*value)
	}
	return s.writeSignedResolution64(value64, length, divideBy, bitOffset, offset)
}

func (s *DataStream) writeSignedResolution64(value *float64, length uint16, divideBy float32, bitOffset uint16, offset int64) error {
	var outVal uint64
	if value == nil {
		outVal = missingValue(length, true)
	} else {
		if divideBy != 0 {
			outVal = uint64(*value/float64(divideBy) - float64(offset))
		} else {
			outVal = uint64(*value - float64(offset))
		}
		maxValid := calcMaxPositiveValue(length, true)
		if outVal > maxValid {
			outVal = maxValid // pin at maximum value
		}
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeUnsignedResolution32(value *float32, length uint16, divideBy float32, bitOffset uint16, offset int64) error {
	var value64 *float64
	if value != nil {
		value64 = new(float64)
		*value64 = float64(*value)
	}
	return s.writeUnsignedResolution64(value64, length, divideBy, bitOffset, offset)
}

func (s *DataStream) writeUnsignedResolution64(value *float64, length uint16, divideBy float32, bitOffset uint16, offset int64) error {
	var outVal uint64
	if value == nil {
		outVal = missingValue(length, false)
	} else {
		if divideBy != 0 {
			outVal = uint64(*value/float64(divideBy) - float64(offset))
		} else {
			outVal = uint64(*value - float64(offset))
		}
		maxValid := calcMaxPositiveValue(length, false)
		if outVal > maxValid {
			outVal = maxValid // pin at maximum value
		}
	}
	return s.putNumberRaw(outVal, length, bitOffset)
}

func (s *DataStream) writeUnsignedNumber(value *uint64, length uint16, bitOffset uint16) error {
	maxVal := calcMaxPositiveValue(length, false)
	if *value > maxVal {
		*value = maxVal
	}
	return s.putNumberRaw(*value, length, bitOffset)
}

func (s *DataStream) writeSignedNumber(value int64, length uint16, bitOffset uint16) error {
	maxVal := calcMaxPositiveValue(length, false)
	if value > int64(maxVal) {
		value = int64(maxVal)
	}
	return s.putNumberRaw(uint64(value), length, bitOffset)
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
