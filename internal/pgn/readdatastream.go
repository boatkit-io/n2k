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
	"strings"
	"unicode/utf16"

	"golang.org/x/exp/constraints"
)

// isEOF method returns true if the offsets exactly equal the data length
func (s *DataStream) isEOF() bool {
	// For now, only call an exact EOF -- not sure if we need to be more loosy-goosy or not
	return s.byteOffset == uint16(len(s.data)) && s.bitOffset == 0
}

// skipBits method moves the read cursor ahead by the specified amount.
func (s *DataStream) skipBits(bitLength uint16) error {
	s.byteOffset += bitLength >> 3
	bitLength &= 7
	s.bitOffset += uint8(bitLength)
	if s.bitOffset >= 8 {
		s.byteOffset++
		s.bitOffset -= 8
	}

	if int(s.byteOffset) >= len(s.data) {
		return fmt.Errorf("reading byte(%d) off end of pgn (len:%d)", s.byteOffset, len(s.data))
	}

	return nil
}

// readLookupField method returns the specified length (max 64) data.
// Lookups don't have reserved values - all bit patterns are either defined enum values or unknown enum values.
func (s *DataStream) readLookupField(bitLength uint16) (uint64, error) {
	if bitLength > 64 {
		return 0, fmt.Errorf("requested %d bitLength in ReadLookupField", bitLength)
	}

	v, err := s.getNumberRaw(bitLength)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// readFloat32 method reads and returns a *float32
func (s *DataStream) readFloat32() (*float32, error) {
	val, err := s.getNumberRaw(32)
	if err != nil {
		return nil, err
	}
	val32 := uint32(val)
	floatVal := math.Float32frombits(val32)
	return &floatVal, nil
}

// readBinaryData method reads the specified length of data and returns it in a uint8 slice
func (s *DataStream) readBinaryData(bitLength uint16) ([]uint8, error) {
	if s.bitOffset != 0 {
		return nil, fmt.Errorf("binary data must be aligned on byte boundary")
	}
	numBytes := uint16(math.Ceil(float64(bitLength) / 8))
	bytesRemaining := len(s.data) - int(s.byteOffset)
	if bytesRemaining < int(numBytes) {
		return nil, fmt.Errorf("not enough bytes remaining to read %d bits", bitLength)
	}
	oddBits := bitLength & 0x7
	arr := make([]uint8, numBytes)

	for i := uint16(0); i < numBytes; i++ {
		arr[i] = s.data[s.byteOffset]
		s.byteOffset++
	}
	// if oddBits != 0, then we need to mask off the bits that are beyond the bitLength
	// we also need to adjust the bitOffset and byteOffset to account for the bits we're not returning
	if oddBits != 0 {
		arr[numBytes-1] &= uint8(0xFF) >> (8 - oddBits)
		s.bitOffset += uint8(oddBits)
		s.byteOffset--
	}
	return arr, nil
}

// readStringStartStopByte method reads a string encoded as described in reference:
// https://github.com/canboat/canboatjs/blob/b857a503323291b92dd0fe8c41ad6fa0d6bda088/lib/fromPgn.js#L752
/* func (s *PGNDataStream) readStringStartStopByte() (string, error) {
	// guaranteed to be aligned on byte boundary
	startByte, err := s.getNumberRaw(8)
	if err != nil {
		return "", err
	}
	// TO FIX: 0x0 or 0x1 indicates an empty string
	// This format "STRING_VAR" not used by existing PGN definitions.
	if startByte != 2 {
		return "", fmt.Errorf("[Wrong start byte:%08X]", startByte)
	}
	arr := make([]uint8, 0, 64)
	for {
		b, err := s.getNumberRaw(8)
		if err != nil {
			return "", err
		}
		if b == 1 {
			// Stop byte
			return string(arr), nil
		}
		arr = append(arr, uint8(b))
	}
}
*/

// readStringWithLengthAndControl method reads a string with length and control byte
// String has a terminating zero(s). We remove them.
// Length incudes the len/control bytes.
//
//	        "Name":"STRING_LAU",
//		"Description":"A varying length string containing double or single byte codepoints encoded with a length byte and terminating zero.",
//		"EncodingDescription": (length byte, then 0=UNICODE / 1=ASCII — see canboat schema).
//		"Comment": (character set details omitted; see canboat).
//
// Conflicts with this comment:
func (s *DataStream) readStringWithLengthAndControl() (string, error) {
	lc, err := s.readBinaryData(16)
	if err != nil {
		return "", err
	}
	if len(lc) == 0 {
		return "", fmt.Errorf("no data available for string length and control")
	}
	if lc[0] < 4 { // 2 is zero-length, 0 or 1 is an error, 3 means there's only the terminating zero. Minimum length for content is 4
		return "", nil
	}
	length := (uint16(lc[0]) - 2) * 8 // remove length and control bytes, calculate bitLength for remaining chars with terminating 0
	arr, err := s.readBinaryData(length)
	if err != nil {
		return "", err
	}
	arr = arr[:len(arr)-1] // remove the trailing 0

	// if control == 0, then it's UTF-16. Convert to UTF-8
	if lc[1] == 0 {
		var a16 []uint16
		for i := 0; i < len(arr); i += 2 {
			n := uint16(arr[i])<<8 | uint16(arr[i+1])
			a16 = append(a16, n)
		}
		runes := utf16.Decode(a16)
		return string(runes), nil
	}
	return string(arr), nil
}

// readStringWithLength method reads a string with leading length byte
// Canboat format "STRING_LZ"
// readStringWithLength reads a string with length prefix.
// The first byte contains the string length (not including the length byte or terminating zero).
// If bitLength is 0, reads to end of stream. Otherwise, reads up to bitLength bits.
// String has a terminating zero; we remove it.
//
//nolint:unparam // Why: bitLength is generated; decoders currently pass 0.
func (s *DataStream) readStringWithLength(bitLength uint16) (string, error) {
	// Read the length byte first
	lengthByte, err := s.readBinaryData(8) // Read 1 byte for length
	if err != nil {
		return "", err
	}
	if len(lengthByte) == 0 {
		return "", fmt.Errorf("no data available for string length byte")
	}

	strLen := lengthByte[0]
	if strLen == 0 {
		return "", nil // Empty string
	}

	// Calculate how many bytes to read for the string content
	// strLen is the number of characters, plus 1 for the terminating zero
	bytesToRead := uint16(strLen+1) * 8 // Convert to bits

	// If bitLength is specified and we have a limit, respect it
	if bitLength > 0 {
		remainingBits := bitLength - 8 // Subtract the length byte we already read
		if bytesToRead > remainingBits {
			bytesToRead = remainingBits
		}
	}

	// Read the string content
	arr, err := s.readBinaryData(bytesToRead)
	if err != nil {
		return "", err
	}
	if len(arr) == 0 {
		return "", fmt.Errorf("no data available for string content")
	}

	// Check if we have the terminating zero
	if len(arr) < int(strLen+1) || arr[strLen] != 0 {
		return "", fmt.Errorf("string not properly terminated")
	}

	// Return the string without the terminating zero
	return string(arr[:strLen]), nil
}

// readFixedString method reads a string of fixed length.
func (s *DataStream) readFixedString(bitLength uint16) (string, error) {
	arr, err := s.readBinaryData(bitLength)
	if err != nil {
		return "", err
	}
	str := string(arr)

	// String may be padded on end by 0, "@", or 0xFF, cull appropriately
	i := strings.IndexByte(str, 0)
	if i != -1 {
		str = str[:i]
	}
	i = strings.IndexByte(str, 0xFF)
	if i != -1 {
		str = str[:i]
	}
	i = strings.IndexRune(str, '@')
	if i != -1 {
		str = str[:i]
	}

	return str, nil
}

// getNumberRaw method reads up to 64 bits from the stream and returns it as a uint64.
// Took notes from:
// https://github.com/canboat/canboat/blob/732371ada8b0c6f33652c3ab61f0856abfd9e076/analyzer/pgn.c#L253
// Except that a bunch of it seems wrong... their examples reference MSB ordering of things but it appears
// to really be LSB, the way that would make sense...
func (s *DataStream) getNumberRaw(bitLength uint16) (uint64, error) {
	// Check if we have enough bits remaining in the stream before starting
	// Use the same calculation as remainingLength() method
	totalBitsInStream := len(s.data) * 8
	bitsConsumed := int(s.byteOffset)*8 + int(s.bitOffset)
	bitsRemaining := totalBitsInStream - bitsConsumed
	if int(bitLength) > bitsRemaining {
		return 0, fmt.Errorf("reading %d bits off end of pgn (remaining: %d bits)", bitLength, bitsRemaining)
	}

	var ret uint64
	outBitOffset := 0

	for bitLength > 0 {
		if int(s.byteOffset) >= len(s.data) {
			return 0, fmt.Errorf("reading byte(%d) off end of pgn (len:%d)", s.byteOffset, len(s.data))
		}

		bitsToGrab := 8 - s.bitOffset
		if bitLength < uint16(bitsToGrab) {
			bitsToGrab = uint8(bitLength)
		}

		b := s.data[s.byteOffset]
		b >>= s.bitOffset
		if bitsToGrab < 8 {
			mask := uint8(0xFF >> (8 - bitsToGrab))
			b &= mask
		}
		ret |= uint64(b) << uint64(outBitOffset)
		outBitOffset += int(bitsToGrab)
		bitLength -= uint16(bitsToGrab)
		s.bitOffset += bitsToGrab
		if s.bitOffset >= 8 {
			s.bitOffset -= 8
			s.byteOffset++
		}
	}

	return ret, nil
}

// getNullableNumberRaw method reads the specified length and returns a *uint64, or nil if missing or invalid.
// It uses the pre-calculated MaxRawValue from FieldSpec to determine validity.
func (s *DataStream) getNullableNumberRaw(spec *FieldSpec) (*uint64, error) {
	v, err := s.getNumberRaw(spec.BitLength)
	if err != nil {
		return nil, err
	}

	if spec.IsSigned {
		mask := uint64(1 << (spec.BitLength - 1))
		if (v & mask) > 0 { // negative signed number, so smaller than maxint, so just return
			return &v, nil
		}
	}

	// Use pre-calculated MaxRawValue instead of runtime calculation
	if v > spec.MaxRawValue { // either missing or invalid. We'll return nil either way
		return nil, nil
	}

	return &v, nil
}

// getSignedNullableNumber method returns a *int64 or nil if null
func (s *DataStream) getSignedNullableNumber(spec *FieldSpec) (*int64, error) {
	v, err := s.getNullableNumberRaw(spec)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}

	// Sign extend if negative
	signBit := uint64(1) << (spec.BitLength - 1)
	if (*v&signBit) != 0 && spec.BitLength < 64 {
		mask := uint64(math.MaxUint64) << spec.BitLength
		*v |= mask
	}

	vi := int64(*v)
	return &vi, nil
}

// readVariableDataWithSpec method reads and returns variable data using FieldSpec
/* func (s *DataStream) readVariableDataWithSpec(spec *FieldSpec) ([]uint8, error) {
	if spec == nil {
		return nil, fmt.Errorf("FieldSpec is nil")
	}

	if spec.BitLengthVariable {
		if spec.CanboatType == "STRING_LAU" {
			str, err := s.readStringWithLengthAndControl()
			if err != nil {
				return nil, err
			}
			return []uint8(str), nil
		}
	}

	len := (spec.BitLength + 7) &^ 0x7
	return s.readBinaryData(len)
} */

// ReadRaw reads a non-scaled integer field using pre-calculated FieldSpec metadata
func ReadRaw[T constraints.Integer](s *DataStream, spec *FieldSpec) (*T, error) {
	if spec == nil {
		return nil, fmt.Errorf("FieldSpec is nil")
	}
	if spec.BitLength > 64 {
		return nil, fmt.Errorf("requested %d bitLength exceeds 64 bits", spec.BitLength)
	}

	var rawValue uint64
	var err error

	if spec.ReservedCount == 0 {
		// No reserved values - all bit patterns are valid, never return nil
		rawValue, err = s.getNumberRaw(spec.BitLength)
		if err != nil {
			return nil, err
		}
	} else {
		// Has reserved values - use nullable logic
		v, err := s.getNullableNumberRaw(spec)
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, nil
		}
		rawValue = *v
	}

	// Apply offset for non-scaled fields
	if spec.Offset != 0 {
		if spec.IsSigned {
			signedVal := int64(rawValue)
			signedVal += spec.Offset
			rawValue = uint64(signedVal)
		} else {
			rawValue = uint64(int64(rawValue) + spec.Offset)
		}
	}

	// Handle signed conversion if needed
	if spec.IsSigned && spec.BitLength < 64 {
		if (rawValue & (1 << (spec.BitLength - 1))) != 0 {
			// Negative number - sign extend
			mask := uint64(0xFFFFFFFFFFFFFFFF) << spec.BitLength
			rawValue |= mask
		}
	}

	result := T(rawValue)
	return &result, nil
}

// ReadScaled reads a scaled float field using pre-calculated FieldSpec metadata
func ReadScaled[T constraints.Float](s *DataStream, spec *FieldSpec) (*T, error) {
	if spec == nil {
		return nil, fmt.Errorf("FieldSpec is nil")
	}
	if spec.BitLength > 64 {
		return nil, fmt.Errorf("requested %d bitLength exceeds 64 bits", spec.BitLength)
	}

	var val float64

	if spec.ReservedCount == 0 {
		// No reserved values - read raw value
		v, err := s.getNumberRaw(spec.BitLength)
		if err != nil {
			return nil, err
		}

		if spec.IsSigned {
			// Handle signed conversion
			if spec.BitLength < 64 && (v&(1<<(spec.BitLength-1))) != 0 {
				mask := uint64(0xFFFFFFFFFFFFFFFF) << spec.BitLength
				v |= mask
			}
			val = float64(int64(v))
		} else {
			// Unsigned value
			val = float64(v)
		}
	} else {
		// Has reserved values - use nullable logic
		if spec.IsSigned {
			rawValue, err := s.getSignedNullableNumber(spec)
			if err != nil {
				return nil, err
			}
			if rawValue == nil {
				return nil, nil
			}
			val = float64(*rawValue)
		} else {
			rawValue, err := s.getNullableNumberRaw(spec)
			if err != nil {
				return nil, err
			}
			if rawValue == nil {
				return nil, nil
			}
			val = float64(*rawValue)
		}
	}

	// Apply resolution scaling first
	if spec.Resolution != 0 && spec.Resolution != 1.0 {
		prec := calcPrecision(spec.Resolution)
		val = roundFloat(val*spec.Resolution, prec)
	}

	// Then add offset
	val += float64(spec.Offset)

	// Apply domain constraints if specified
	if spec.DomainMax != nil && val > *spec.DomainMax {
		val = *spec.DomainMax
	}
	if spec.DomainMin != nil && val < *spec.DomainMin {
		val = *spec.DomainMin
	}

	result := T(val)
	return &result, nil
}
