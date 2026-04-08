// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package pgn

import (
	"fmt"
	"math"
	"reflect"

	"golang.org/x/exp/constraints"

	"github.com/boatkit-io/tugboat/pkg/units"
)

// writeReserved fills the specified number of bits at the specified offset with 1s
func (s *DataStream) writeReserved(bitLength, bitOffset uint16) error {
	return s.putNumberRaw(0xFFFFFFFFFFFFFFFF, bitLength, bitOffset)
}

// writeSpare fills the specified number of bits at the specified offset with 0s
func (s *DataStream) writeSpare(bitLength, bitOffset uint16) error {
	return s.putNumberRaw(0, bitLength, bitOffset)
}

// writeStringLau writes the specified length of value at the specified offset
func (s *DataStream) writeStringLau(value string, bitOffset uint16) error {
	var out []uint8
	if value == "" {
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

// writeStringWithLength writes the specified length of value at the specified offset.
//
//nolint:unparam // bitLength is fixed by generated encoders (often 0); kept for symmetry with FieldSpec.
func (s *DataStream) writeStringWithLength(value string, bitLength, bitOffset uint16) error {
	length := uint8(len(value)) + 1 //  string length plus terminator
	fieldLength := uint8(bitLength / 8)
	if length+1 > fieldLength { // field must contain the length byte, the string, and the terminator
		return fmt.Errorf("attempt to write string with length longer than field's length")
	}
	out := make([]uint8, fieldLength) // allocate the field's length, filled with zeros
	out[0] = length
	for i := range value {
		out[i+1] = value[i]
	}
	return s.writeBinary(out, bitLength, bitOffset)
}

// writeStringFix writes the fixed string, first padding its length as necessary.
// padding has been seen as "@", 0x00, and 0xff. we use the latter.
func (s *DataStream) writeStringFix(value []uint8, bitLength, bitOffset uint16) error {
	byteCount := bitLength / 8
	for i := len(value); i < int(byteCount); i++ {
		value = append(value, 0xff)
	}
	return s.writeBinary(value, bitLength, bitOffset)
}

// writeBinary writes the specified length of value at the specified offset
func (s *DataStream) writeBinary(value []uint8, bitLength, bitOffset uint16) error {
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

// checkNilInterface returns true if the interface is nil
func checkNilInterface(i interface{}) bool {
	iv := reflect.ValueOf(i)
	if !iv.IsValid() {
		return true
	}
	//nolint:exhaustive // Why: Only Ptr, Slice, Map, Func, Interface may be nil; every other Kind is never nil.
	switch iv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Func, reflect.Interface:
		return iv.IsNil()
	default:
		return false
	}
}

// writeUnit writes units package values using FieldSpec
// value must be converted to the canboat unit before calling
func (s *DataStream) writeUnit(value any, spec *FieldSpec) error {
	if checkNilInterface(value) {
		return WriteScaled[float32](s, nil, spec)
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

	return WriteScaled(s, &canboatValue, spec)
}

// writeFloat32 writes the specified length of value at the specified offset.
//
//nolint:unparam // Parameters mirror other writers; floats are always 32-bit with no reserved band in generated code.
func (s *DataStream) writeFloat32(value *float32, bitLength, bitOffset uint16, reservedValuesCount int) error {
	// This is called for fields with canboat type FLOAT.
	// These fields use IEEE 754 32-bit floating point bits and have no reserved values.
	if bitLength != 32 {
		return fmt.Errorf("attempt to write float32 with bitLength != 32")
	}
	if reservedValuesCount != 0 {
		return fmt.Errorf("attempt to write float32 with reservedValuesCount != 0")
	}
	val := math.Float32bits(*value)
	return s.putNumberRaw(uint64(val), bitLength, bitOffset)
}

// writeFloat64 writes the specified length of value at the specified offset
/* func (s *DataStream) writeFloat64(value *float64, bitLength uint16, bitOffset uint16) error {

	return s.writeSignedResolution64(value, bitLength, 1, bitOffset, 0)
} */

// putNumberRaw method writes up to 64 bits to the stream from a uint64 argument.
// Cribbed the getNumberRaw function
func (s *DataStream) putNumberRaw(value uint64, bitLength, bitOffset uint16) error {
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

		mask := uint8(0xFF >> (8 - bitsToWrite))
		outByte := uint8(value) & mask
		if bitsToWrite <= bitsLeft {
			outByte <<= (startBit)
		}

		value >>= uint64(bitsToWrite)
		s.data[s.byteOffset] |= outByte
		bitLength -= uint16(bitsToWrite)
		s.bitOffset += bitsToWrite
		if s.bitOffset >= 8 {
			s.bitOffset -= 8
			s.byteOffset++
		}
	}
	return nil
}

// WriteRaw writes a non-scaled integer field using pre-calculated FieldSpec metadata
func WriteRaw[T constraints.Integer](s *DataStream, value *T, spec *FieldSpec) error {
	if spec == nil {
		return fmt.Errorf("FieldSpec is nil")
	}

	var outVal uint64

	if value == nil {
		if spec.ReservedCount == 0 {
			return fmt.Errorf("cannot write nil value to field with no reserved values")
		}
		outVal = spec.MissingValue
	} else {
		var rawValue uint64

		// Handle signed vs unsigned conversion properly to avoid sign extension issues
		if spec.IsSigned {
			// For signed fields, convert to int64 first to preserve sign, then handle within field bit width
			signedVal := int64(*value)

			// Apply offset for non-scaled fields (subtract offset to get wire value)
			if spec.Offset != 0 {
				signedVal -= spec.Offset
			}

			// For signed fields, we need to handle the range validation differently
			// Check if the value is within the valid range for the field
			maxPositive := int64(1<<(spec.BitLength-1) - 1)  // Maximum positive value
			minNegative := -int64(1 << (spec.BitLength - 1)) // Minimum negative value

			// Apply reserved value constraints
			if spec.ReservedCount > 0 {
				maxPositive -= int64(spec.ReservedCount)
			}

			// Clamp to valid range
			if signedVal > maxPositive {
				signedVal = maxPositive
			}
			if signedVal < minNegative {
				signedVal = minNegative
			}

			// Convert to unsigned representation within the field's bit width
			// This prevents sign extension issues when casting to uint64
			bitMask := uint64(1<<spec.BitLength - 1)
			rawValue = uint64(signedVal) & bitMask
		} else {
			// For unsigned fields, direct conversion is safe
			rawValue = uint64(*value)

			// Apply offset for non-scaled fields (subtract offset to get wire value)
			if spec.Offset != 0 {
				rawValue = uint64(int64(rawValue) - spec.Offset)
			}

			// Validate against max value if reserved values exist
			if spec.ReservedCount > 0 && rawValue > spec.MaxRawValue {
				rawValue = spec.MaxRawValue // Pin to maximum valid value
			}
		}

		outVal = rawValue
	}

	return s.putNumberRaw(outVal, spec.BitLength, 0)
}

// WriteScaled writes a scaled float field using pre-calculated FieldSpec metadata
func WriteScaled[T constraints.Float](s *DataStream, value *T, spec *FieldSpec) error {
	if spec == nil {
		return fmt.Errorf("FieldSpec is nil")
	}

	if value == nil {
		if spec.ReservedCount == 0 {
			return fmt.Errorf("cannot write nil value to field with no reserved values")
		}
		return s.putNumberRaw(spec.MissingValue, spec.BitLength, 0)
	}

	val := float64(*value)

	// Apply domain constraints (clamping)
	if spec.DomainMax != nil && val > *spec.DomainMax {
		val = *spec.DomainMax
	}
	if spec.DomainMin != nil && val < *spec.DomainMin {
		val = *spec.DomainMin
	}

	// First subtract offset
	val -= float64(spec.Offset)

	// Then apply resolution scaling (divide to convert back to raw)
	if spec.Resolution != 0 && spec.Resolution != 1.0 {
		prec := calcPrecision(spec.Resolution)
		scaledVal := val / spec.Resolution
		val = roundFloat(scaledVal, prec)
	}

	// Handle range validation for scaled values
	var outVal uint64
	if spec.IsSigned {
		// For signed fields, handle negative values
		intVal := int64(val)

		// Validate against max positive value if reserved values exist
		if spec.ReservedCount > 0 {
			maxVal := int64(spec.MaxRawValue)
			minVal := -int64(1 << (spec.BitLength - 1))
			if intVal > maxVal {
				intVal = maxVal
			}
			if intVal < minVal {
				intVal = minVal
			}
		}

		outVal = uint64(intVal)
	} else {
		// For unsigned fields
		if val < 0 {
			val = 0
		}

		outVal = uint64(val)

		// Validate against max value if reserved values exist
		if spec.ReservedCount > 0 && outVal > spec.MaxRawValue {
			outVal = spec.MaxRawValue
		}
	}

	return s.putNumberRaw(outVal, spec.BitLength, 0)
}
