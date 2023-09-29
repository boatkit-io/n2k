package pgn

import (
	"fmt"
	"math"
	"strings"
)

// PGNDataStream instances provide methods to read data types from a stream.
// byteOffset and bitOffset combine to act as the read "cursor".
// The low level read functions update the cursor.
type PGNDataStream struct {
	data []uint8

	byteOffset uint16
	bitOffset  uint8
}

// NewPgnDataStream returns a new PGNDataStream. Call it with the data from a complete Packet.
func NewPgnDataStream(data []uint8) *PGNDataStream {
	return &PGNDataStream{
		data:       data,
		byteOffset: 0,
		bitOffset:  0,
	}
}

// resetToStart method resets the stream. Commented out since its currently unused.
// func (s *PGNDataStream) resetToStart() {
//	s.byteOffset = 0
//	s.bitOffset = 0
// }

// isEOF method returns true if the offsets exactly equal the data length
func (s *PGNDataStream) isEOF() bool {
	// For now, only call an exact EOF -- not sure if we need to be more loosy-goosy or not
	return s.byteOffset == uint16(len(s.data)) && s.bitOffset == 0
}

// skipBits method moves the read cursor ahead by the specified amount.
func (s *PGNDataStream) skipBits(bitLength uint16) error {
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

// getBitOffset method returns the read cursor in bits.
func (s *PGNDataStream) getBitOffset() uint32 {
	return uint32(s.byteOffset)*8 + uint32(s.bitOffset)
}

// readLookupField method returns the specified length (max 64) data.
func (s *PGNDataStream) readLookupField(bitLength uint16) (uint64, error) {
	if bitLength > 64 {
		return 0, fmt.Errorf("requested %d bitLength in ReadLookupField", bitLength)
	}

	v, err := s.getNumberRaw(bitLength)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// readSignedResolution method reads the specified length of data, scales it, and returns as a *float32.
func (s *PGNDataStream) readSignedResolution(bitLength uint16, multiplyBy float32) (*float32, error) {
	if bitLength > 64 {
		return nil, fmt.Errorf("requested %d bitLength in ReadSignedResolution", bitLength)
	}

	v, err := s.getSignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := float32(*v) * multiplyBy
	return &vo, nil
}

// readUnsignedResolution method reads the specified data as an unsigned number and scales it.
func (s *PGNDataStream) readUnsignedResolution(bitLength uint16, multiplyBy float32) (*float32, error) {
	if bitLength > 64 {
		return nil, fmt.Errorf("requested %d bitLength in ReadUnsignedResolution", bitLength)
	}

	v, err := s.getUnsignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := float32(*v) * multiplyBy
	return &vo, nil
}

// readUInt64 method reads and returns *uint64
func (s *PGNDataStream) readUInt64(bitLength uint16) (*uint64, error) {
	if bitLength > 64 {
		return nil, fmt.Errorf("requested %d bitLength in ReadUInt64", bitLength)
	}

	v, err := s.getUnsignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v, nil
}

// readUInt32 method reads and returns a *uint32
func (s *PGNDataStream) readUInt32(bitLength uint16) (*uint32, error) {
	if bitLength > 32 {
		return nil, fmt.Errorf("requested %d bitLength in ReadUInt32", bitLength)
	}

	v, err := s.getUnsignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := uint32(*v)
	return &vo, nil
}

// readUInt16 method reads and returns a *uint16
func (s *PGNDataStream) readUInt16(bitLength uint16) (*uint16, error) {
	if bitLength > 16 {
		return nil, fmt.Errorf("requested %d bitLength in ReadUInt16", bitLength)
	}

	v, err := s.getUnsignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := uint16(*v)
	return &vo, nil
}

// readUInt8 method reads and returns a *uint8
func (s *PGNDataStream) readUInt8(bitLength uint16) (*uint8, error) {
	if bitLength > 8 {
		return nil, fmt.Errorf("requested %d bitLength in ReadUInt8", bitLength)
	}

	v, err := s.getUnsignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := uint8(*v)
	return &vo, nil
}

// readInt64 method reads and returns a *int64
func (s *PGNDataStream) readInt64(bitLength uint16) (*int64, error) {
	if bitLength > 64 {
		return nil, fmt.Errorf("requested %d bitLength in ReadInt64", bitLength)
	}

	v, err := s.getSignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := int64(*v)
	return &vo, nil
}

// readInt32 method reads and returns a *int32
func (s *PGNDataStream) readInt32(bitLength uint16) (*int32, error) {
	if bitLength > 32 {
		return nil, fmt.Errorf("requested %d bitLength in ReadInt32", bitLength)
	}

	v, err := s.getSignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := int32(*v)
	return &vo, nil
}

// readInt16 method reads and returns a *int16
func (s *PGNDataStream) readInt16(bitLength uint16) (*int16, error) {
	if bitLength > 16 {
		return nil, fmt.Errorf("requested %d bitLength in ReadInt16", bitLength)
	}

	v, err := s.getSignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := int16(*v)
	return &vo, nil
}

// readInt8 method reads and returns a *int8
func (s *PGNDataStream) readInt8(bitLength uint16) (*int8, error) {
	if bitLength > 8 {
		return nil, fmt.Errorf("requested %d bitLength in ReadInt8", bitLength)
	}

	v, err := s.getSignedNullableNumber(bitLength)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := int8(*v)
	return &vo, nil
}

// readFloat32 method reads and returns a *float32
func (s *PGNDataStream) readFloat32() (*float32, error) {
	v, err := s.readUInt32(32)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := math.Float32frombits(*v)
	return &vo, nil
}

// readBinaryData method reads the specified length of data and returns it in a uint8 slice
func (s *PGNDataStream) readBinaryData(bitLength uint16) ([]uint8, error) {
	// For now, reuse getNumberRaw, 64 bits at a time
	numBytes := uint16(math.Ceil(float64(bitLength) / 8))
	arr := make([]uint8, numBytes)

	idx := 0
	for i := uint16(0); i < bitLength; i += 64 {
		num := uint16(64)
		if bitLength-i < 64 {
			num = bitLength - i
		}
		v, err := s.getNumberRaw(num)
		if err != nil {
			return nil, err
		}
		for h := uint16(0); h < num; h += 8 {
			arr[idx] = uint8((v & (0xFF << h)) >> h)
			idx++
		}
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
// String has a terminating zero.
// Length incudes the len/control bytes.
//
//	        "Name":"STRING_LAU",
//		"Description":"A varying length string containing double or single byte codepoints encoded with a length byte and terminating zero.",
//		"EncodingDescription":"The length of the string is determined by a starting length byte. The 2nd byte contains 0 for UNICODE or 1 for ASCII.",
//		"Comment":"It is unclear what character sets are allowed/supported. For single byte, assume ASCII. For UNICODE, assume UTF-16, but this has not been seen in the wild yet.",
//
// Conflicts with this comment:
// Control 0 = ASCII, nonzero = UTF8 -- TBD how to address this in the future
func (s *PGNDataStream) readStringWithLengthAndControl() (string, error) {
	lc, err := s.readBinaryData(16)
	if err != nil {
		return "", err
	}
	len := (uint16(lc[0]) - 2) * 8 // remove length and control bytes, leaves chars with terminating 0
	// control := lc[1]
	arr, err := s.readBinaryData(len)
	if err != nil {
		return "", err
	}
	return string(arr), nil
}

// readStringWithLength method reads a string with leading length byte
// Canboat format "STRING_LZ"
// String has a terminating zero
// Length does not seem to include length byte here
func (s *PGNDataStream) readStringWithLength() (string, error) {
	len, err := s.readUInt8(8)
	if err != nil {
		return "", err
	}
	if len == nil {
		return "", fmt.Errorf("null length in ReadStringWithLength")
	}
	arr, err := s.readBinaryData(uint16(*len * 8))
	if err != nil {
		return "", err
	}
	return string(arr), nil
}

// readFixedString method reads a string of fixed length.
func (s *PGNDataStream) readFixedString(bitLength uint16) (string, error) {
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
func (s *PGNDataStream) getNumberRaw(bitLength uint16) (uint64, error) {
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
			mask := uint8(0xFF >> uint8(8-bitsToGrab))
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

// getNullableNumberRaw method reads the specified length and returns a *uint64, or nil if maxvalue.
func (s *PGNDataStream) getNullableNumberRaw(bitLength uint16, signed bool) (*uint64, error) {
	v, err := s.getNumberRaw(bitLength)
	if err != nil {
		return nil, err
	}

	// Check for max value -> nil
	maxVal := uint64(0xFFFFFFFFFFFFFFFF)
	maxVal >>= 64 - bitLength
	if signed {
		maxVal >>= 1
	}
	if v == maxVal {
		return nil, nil
	}

	return &v, nil
}

// getUnsignedNullableNumber method returns a *uint64 or nil if null
func (s *PGNDataStream) getUnsignedNullableNumber(bitLength uint16) (*uint64, error) {
	return s.getNullableNumberRaw(bitLength, false)
}

// getSignedNullableNumber method returns a *int64 or nil if null
func (s *PGNDataStream) getSignedNullableNumber(bitLength uint16) (*int64, error) {
	v, err := s.getNullableNumberRaw(bitLength, true)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}

	// Check if negative (max bit set)
	mask := uint64(1 << (bitLength - 1))
	if *v&mask > 0 {
		*v ^= mask
		vi := -int64(mask) + int64(*v)
		return &vi, nil
	}
	vi := int64(*v)
	return &vi, nil
}

// readVariableData method reads and returns the value of pgn.fieldIndex as an interface{}
func (s *PGNDataStream) readVariableData(pgn uint32, manID ManufacturerCodeConst, fieldIndex uint8) ([]uint8, error) {
	field, err := GetFieldDescriptor(pgn, manID, fieldIndex)
	if err == nil {
		if field.BitLengthVariable {
			if field.CanboatType == "STRING_LAU" {
				str, err := s.readStringWithLengthAndControl()
				if err != nil {
					return nil, err
				}
				return []uint8(str), nil
			}
		}
		len := (field.BitLength + 7) &^ 0x7
		return s.readBinaryData(len)
	} else {
		return nil, err
	}
}
