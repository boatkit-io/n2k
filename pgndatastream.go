package n2k

import (
	"fmt"
	"math"
	"strings"
)

type PGNDataStream struct {
	data []uint8

	byteOffset uint16
	bitOffset  uint8
}

func NewPgnDataStream(data []uint8) *PGNDataStream {
	return &PGNDataStream{
		data:       data,
		byteOffset: 0,
		bitOffset:  0,
	}
}

func (s *PGNDataStream) ResetToStart() {
	s.byteOffset = 0
	s.bitOffset = 0
}

func (s *PGNDataStream) IsEOF() bool {
	// For now, only call an exact EOF -- not sure if we need to be more loosy-goosy or not
	return s.byteOffset == uint16(len(s.data)) && s.bitOffset == 0
}

func (s *PGNDataStream) SkipBits(bitLength uint16) error {
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

func (s *PGNDataStream) GetBitOffset() uint32 {
	return uint32(s.byteOffset)*8 + uint32(s.bitOffset)
}

func (s *PGNDataStream) ReadLookupField(bitLength uint16) (uint64, error) {
	if bitLength > 64 {
		return 0, fmt.Errorf("requested %d bitLength in ReadLookupField", bitLength)
	}

	v, err := s.getNumberRaw(bitLength)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (s *PGNDataStream) ReadSignedResolution(bitLength uint16, multiplyBy float32) (*float32, error) {
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

func (s *PGNDataStream) ReadUnsignedResolution(bitLength uint16, multiplyBy float32) (*float32, error) {
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

func (s *PGNDataStream) ReadUInt64(bitLength uint16) (*uint64, error) {
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

func (s *PGNDataStream) ReadUInt32(bitLength uint16) (*uint32, error) {
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

func (s *PGNDataStream) ReadUInt16(bitLength uint16) (*uint16, error) {
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

func (s *PGNDataStream) ReadUInt8(bitLength uint16) (*uint8, error) {
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

func (s *PGNDataStream) ReadInt64(bitLength uint16) (*int64, error) {
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

func (s *PGNDataStream) ReadInt32(bitLength uint16) (*int32, error) {
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

func (s *PGNDataStream) ReadInt16(bitLength uint16) (*int16, error) {
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

func (s *PGNDataStream) ReadInt8(bitLength uint16) (*int8, error) {
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

func (s *PGNDataStream) ReadFloat32() (*float32, error) {
	v, err := s.ReadUInt32(32)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	vo := math.Float32frombits(*v)
	return &vo, nil
}

func (s *PGNDataStream) ReadBinaryData(bitLength uint16) ([]uint8, error) {
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

// Reference: https://github.com/canboat/canboatjs/blob/b857a503323291b92dd0fe8c41ad6fa0d6bda088/lib/fromPgn.js#L752
func (s *PGNDataStream) ReadStringStartStopByte() (string, error) {
	// guaranteed to be aligned on byte boundary
	startByte, err := s.getNumberRaw(8)
	if err != nil {
		return "", err
	}
	// TO FIX: 0x0 or 0x1 indicates and empty string
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

// Canboat format "STRING_LAU"
// NOTE: String has a terminating zero
func (s *PGNDataStream) ReadStringWithLengthAndControl() (string, error) {
	lc, err := s.ReadBinaryData(16)
	if err != nil {
		return "", err
	}
	// Note -- length incudes len/control bytes
	len := (uint16(lc[0]) - 2) * 8
	//         "Name":"STRING_LAU",
	//	"Description":"A varying length string containing double or single byte codepoints encoded with a length byte and terminating zero.",
	//	"EncodingDescription":"The length of the string is determined by a starting length byte. The 2nd byte contains 0 for UNICODE or 1 for ASCII.",
	//	"Comment":"It is unclear what character sets are allowed/supported. For single byte, assume ASCII. For UNICODE, assume UTF-16, but this has not been seen in the wild yet.",
	// Conflicts with this comment:
	// Control 0 = ASCII, nonzero = UTF8 -- TBD how to address this in the future
	// control := lc[1]
	arr, err := s.ReadBinaryData(len)
	if err != nil {
		return "", err
	}
	return string(arr), nil
}

// Canboat format "STRING_LZ"
// NOTE: string has a terminating zero
func (s *PGNDataStream) ReadStringWithLength() (string, error) {
	// Note -- length does not seem to include length byte here
	len, err := s.ReadUInt8(8)
	if err != nil {
		return "", err
	}
	if len == nil {
		return "", fmt.Errorf("null length in ReadStringWithLength")
	}
	arr, err := s.ReadBinaryData(uint16(*len * 8))
	if err != nil {
		return "", err
	}
	return string(arr), nil
}

func (s *PGNDataStream) ReadFixedString(bitLength uint16) (string, error) {
	arr, err := s.ReadBinaryData(bitLength)
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

func (s *PGNDataStream) getUnsignedNullableNumber(bitLength uint16) (*uint64, error) {
	return s.getNullableNumberRaw(bitLength, false)
}

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

func (s *PGNDataStream) ReadVariableData(pgn uint32, fieldIndex uint8) (interface{}, error) {
	if pi, piKnown := pgnInfoLookup[pgn]; !piKnown {
		p := pi[0].FieldInfo[int(fieldIndex)]
		switch p.FieldType {
		case "LOOKUP":
			if p.BitLength > 32 {
				return nil, fmt.Errorf("Read for Lookup > 32 bits")
			}
			return s.ReadLookupField(p.BitLength)
		case "BITLOOKUP":
			if p.BitLength > 32 {
				return nil, fmt.Errorf("Read for BitLookup > 32 bits")
			}
			return s.ReadLookupField(p.BitLength)
		case "INDIRECT_LOOKUP":
			if p.BitLength > 32 {
				return nil, fmt.Errorf("Read for BitLookup > 32 bits")
			}
			return s.ReadLookupField(p.BitLength)
		case "NUMBER", "TIME", "DATE", "MMSI":
			if p.Signed {
				switch {
				case p.Resolution != 1.0:
					return s.ReadSignedResolution(p.BitLength, p.Resolution)
				case p.BitLength > 32:
					return s.ReadInt64(p.BitLength)
				case p.BitLength > 16:
					return s.ReadInt32(p.BitLength)
				case p.BitLength > 8:
					return s.ReadInt16(p.BitLength)
				default:
					return s.ReadInt8(p.BitLength)
				}
			} else {
				switch {
				case p.Resolution != 1.0:
					return s.ReadUnsignedResolution(p.BitLength, p.Resolution)
				case p.BitLength > 32:
					return s.ReadUInt64(p.BitLength)
				case p.BitLength > 16:
					return s.ReadUInt32(p.BitLength)
				case p.BitLength > 8:
					return s.ReadUInt16(p.BitLength)
				default:
					return s.ReadUInt8(p.BitLength)
				}
			}
		case "FLOAT":
			if p.BitLength != 32 {
				return nil, fmt.Errorf("No deserializer for IEEE Float with bitlength non-32")
			}
			return s.ReadFloat32()
		case "DECIMAL":
			return s.ReadBinaryData(p.BitLength)
		case "STRING_VAR":
			return s.ReadStringStartStopByte()
		case "STRING_LAU":
			return s.ReadStringWithLengthAndControl()
		case "STRING_FIX":
			return s.ReadFixedString(p.BitLength)
		case "STRING_LZ":
			return s.ReadStringWithLength()
		case "BINARY":
			if p.BitLength > 0 {
				return s.ReadBinaryData(p.BitLength)
			} else {
				return nil, fmt.Errorf("Can't support dynamically sized binary data in this context")
			}
		case "VARIABLE":
			return nil, fmt.Errorf("can't display recursive variable field ")
		default:
			return nil, fmt.Errorf("No deserializer for type: " + p.FieldType)
		}
	}
	return nil, nil
}
