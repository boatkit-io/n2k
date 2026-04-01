package pgn

import (
	"fmt"
	"math"
	"strings"
)

// PGNDataStream provides a bit-level sequential reader over raw NMEA 2000 message data.
// It is the core decoding primitive: generated PGN decoder functions create a PGNDataStream
// from a packet's payload bytes and then call its typed read methods to extract each field
// in the order defined by the PGN specification.
//
// NMEA 2000 fields are not always byte-aligned -- many are packed at odd bit widths
// (e.g., 3-bit lookup fields, 11-bit manufacturer codes). The stream tracks a combined
// byte+bit cursor so that sub-byte and cross-byte reads work transparently.
//
// Nullable semantics: In the NMEA 2000 encoding, the maximum representable value for a
// field's bit width signals "data not available" (null). The read methods that return
// pointer types use this convention: they return nil when the raw value equals the
// null sentinel, avoiding the need for a separate validity flag.
type PGNDataStream struct {
	// data holds the raw message payload bytes to be decoded sequentially.
	data []uint8

	// byteOffset is the whole-byte portion of the current read cursor position.
	byteOffset uint16
	// bitOffset is the sub-byte portion (0-7) of the current read cursor position.
	// Together with byteOffset, the absolute bit position is: byteOffset*8 + bitOffset.
	bitOffset uint8
}

// NewPgnDataStream creates a PGNDataStream positioned at the beginning of the supplied
// byte slice. The caller should pass the complete reassembled payload of an NMEA 2000
// packet (for fast-packet PGNs, the frames must already be assembled before calling this).
// The stream does not copy the data -- it references the original slice.
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

// isEOF returns true when the read cursor has reached exactly the end of the data.
// It requires an exact match (byte cursor == length AND bit cursor == 0), meaning
// partial-byte overruns are not treated as EOF. This strict check catches cases where
// the stream was advanced to an unexpected position, which may indicate a decoding bug.
func (s *PGNDataStream) isEOF() bool {
	return s.byteOffset == uint16(len(s.data)) && s.bitOffset == 0
}

// skipBits advances the read cursor by bitLength bits without reading data.
// This is used to skip over reserved or unused fields in a PGN definition.
// Returns an error if the skip would move the cursor past the end of the data.
func (s *PGNDataStream) skipBits(bitLength uint16) error {
	// Add the whole-byte portion of the skip to byteOffset.
	s.byteOffset += bitLength >> 3 // equivalent to bitLength / 8
	bitLength &= 7                 // keep only the remaining sub-byte bits

	// Add the remaining sub-byte bits, handling overflow past a byte boundary.
	s.bitOffset += uint8(bitLength)
	if s.bitOffset >= 8 {
		s.byteOffset++
		s.bitOffset -= 8
	}

	// Bounds check: ensure we haven't moved past the end of the data slice.
	if int(s.byteOffset) >= len(s.data) {
		return fmt.Errorf("reading byte(%d) off end of pgn (len:%d)", s.byteOffset, len(s.data))
	}

	return nil
}

// getBitOffset returns the absolute read cursor position in bits from the start of the stream.
// Useful for diagnostics and for verifying that a decoder consumed the expected number of bits.
func (s *PGNDataStream) getBitOffset() uint32 {
	return uint32(s.byteOffset)*8 + uint32(s.bitOffset)
}

// readLookupField reads an unsigned integer of the given bit width (max 64 bits) and
// returns its raw value. Unlike the nullable read methods, this never returns nil -- a
// max-value result is still returned as-is. This is appropriate for enumeration / lookup
// fields where every bit pattern has a defined meaning (e.g., manufacturer codes, status enums).
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

// readSignedResolution reads a signed integer field and applies a resolution scaling factor,
// returning the result as *float32. Many NMEA 2000 fields encode physical values as integers
// with a fixed resolution (e.g., heading stored as radians * 10000). The multiplyBy parameter
// is the inverse of that encoding factor, converting the integer back to real-world units.
// Returns nil when the field contains the NMEA 2000 null sentinel (signed max value).
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

// readSignedResolution64Override is the float64 variant of readSignedResolution. It exists
// for fields where the resolution is fine enough that float32 precision is insufficient --
// most notably latitude and longitude, which use very small resolution values applied to
// large 32-bit integers. Without float64, rounding errors would place positions many meters
// from their true location.
func (s *PGNDataStream) readSignedResolution64Override(bitLength uint16, multiplyBy float64) (*float64, error) {
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
	vo := float64(*v) * multiplyBy
	return &vo, nil
}

// readUnsignedResolution reads an unsigned integer field, applies the resolution scaling factor
// (multiplyBy), and returns the physical value as *float32. Returns nil for the NMEA 2000
// null sentinel (all-ones for the given bit width).
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

// readUInt64 reads an unsigned integer of up to 64 bits and returns it as *uint64.
// Returns nil if the raw value equals the null sentinel (all bits set for the given width).
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

// readUInt32 reads an unsigned integer of up to 32 bits and returns it as *uint32.
// Returns nil for the NMEA 2000 null sentinel. Errors if bitLength exceeds 32.
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

// readUInt16 reads an unsigned integer of up to 16 bits and returns it as *uint16.
// Returns nil for the NMEA 2000 null sentinel. Errors if bitLength exceeds 16.
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

// readUInt8 reads an unsigned integer of up to 8 bits and returns it as *uint8.
// Returns nil for the NMEA 2000 null sentinel. Errors if bitLength exceeds 8.
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

// readInt64 reads a signed two's-complement integer of up to 64 bits and returns it as *int64.
// The sign extension is handled by getSignedNullableNumber. Returns nil for the NMEA 2000
// signed null sentinel (positive max value, i.e., 0x7F...F for the given bit width).
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

// readInt32 reads a signed two's-complement integer of up to 32 bits and returns it as *int32.
// Returns nil for the NMEA 2000 signed null sentinel.
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

// readInt16 reads a signed two's-complement integer of up to 16 bits and returns it as *int16.
// Returns nil for the NMEA 2000 signed null sentinel.
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

// readInt8 reads a signed two's-complement integer of up to 8 bits and returns it as *int8.
// Returns nil for the NMEA 2000 signed null sentinel.
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

// readFloat32 reads a 32-bit IEEE 754 single-precision float from the stream.
// It first reads 32 bits as a uint32, then reinterprets the bit pattern as float32
// using math.Float32frombits. Returns nil if the underlying uint32 is the null sentinel
// (0xFFFFFFFF), which means the data source did not provide a value for this field.
func (s *PGNDataStream) readFloat32() (*float32, error) {
	v, err := s.readUInt32(32)
	if err != nil {
		return nil, err
	}
	if v == nil {
		// The uint32 null sentinel (all-ones) means "not available".
		return nil, nil
	}
	vo := math.Float32frombits(*v)
	return &vo, nil
}

// readBinaryData reads bitLength bits from the stream and returns them as a byte slice.
// This is used for opaque binary fields (e.g., device serial numbers, proprietary data blobs)
// where no numeric interpretation is needed. The result is always ceil(bitLength/8) bytes;
// if bitLength is not a multiple of 8, the final byte's upper bits will be zero-padded.
//
// Internally it reads up to 64 bits at a time via getNumberRaw and splits the result
// into individual bytes in little-endian order, which matches the NMEA 2000 wire format.
func (s *PGNDataStream) readBinaryData(bitLength uint16) ([]uint8, error) {
	numBytes := uint16(math.Ceil(float64(bitLength) / 8))
	arr := make([]uint8, numBytes)

	idx := 0
	for i := uint16(0); i < bitLength; i += 64 {
		// Determine how many bits to grab in this iteration (up to 64).
		num := uint16(64)
		if bitLength-i < 64 {
			num = bitLength - i
		}
		v, err := s.getNumberRaw(num)
		if err != nil {
			return nil, err
		}
		// Split the 64-bit value into individual bytes, extracting each byte
		// by masking and shifting from least-significant to most-significant.
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

// readStringWithLengthAndControl reads a Canboat "STRING_LAU" encoded string.
// Wire format:
//   - Byte 0: total length in bytes (includes this byte, the control byte, the string chars, and a terminating zero)
//   - Byte 1: control/encoding byte (0 = UNICODE/UTF-16, 1 = ASCII) -- currently ignored
//   - Bytes 2..N: the string character data plus a trailing NUL
//
// The NMEA 2000 spec is ambiguous about encoding: one source says "0 = UNICODE, 1 = ASCII",
// another says "0 = ASCII, nonzero = UTF-8". In practice only ASCII has been observed on
// real networks. The control byte is read but not acted upon -- all bytes are returned as-is.
func (s *PGNDataStream) readStringWithLengthAndControl() (string, error) {
	// Read the 2-byte header: length byte and control byte.
	lc, err := s.readBinaryData(16)
	if err != nil {
		return "", err
	}
	// Subtract 2 from the length to exclude the length and control bytes themselves,
	// then convert to bits for readBinaryData. The remaining bytes include the string
	// characters and a terminating NUL.
	len := (uint16(lc[0]) - 2) * 8
	// control := lc[1]  // reserved for future encoding support
	arr, err := s.readBinaryData(len)
	if err != nil {
		return "", err
	}
	return string(arr), nil
}

// readStringWithLength reads a Canboat "STRING_LZ" encoded string.
// Wire format:
//   - Byte 0: length of the string data in bytes (does NOT include this length byte itself)
//   - Bytes 1..N: the string character data (may contain a trailing NUL)
//
// Returns an error if the length byte is the null sentinel (0xFF), which indicates
// that the string field has no data available.
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

// readFixedString reads a fixed-width string field of exactly bitLength bits.
// After reading the raw bytes, padding characters are stripped from the end.
// NMEA 2000 devices use three different padding conventions:
//   - NUL (0x00): standard C-string terminator
//   - 0xFF: "data not available" fill byte
//   - '@': legacy padding character used by some older devices
//
// The string is truncated at the first occurrence of any of these pad characters.
// This means a string cannot legitimately contain '@' or 0xFF as content -- an
// acceptable limitation given typical marine device naming conventions.
func (s *PGNDataStream) readFixedString(bitLength uint16) (string, error) {
	arr, err := s.readBinaryData(bitLength)
	if err != nil {
		return "", err
	}
	str := string(arr)

	// Truncate at the first padding character found (NUL, 0xFF, or '@').
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

// getNumberRaw is the lowest-level read primitive. It extracts up to 64 bits from the
// stream at the current cursor position, returning them as a uint64 in little-endian
// bit order. The cursor is advanced by exactly bitLength bits.
//
// Algorithm: the loop processes one "chunk" per iteration, where each chunk is the
// remaining bits within the current source byte (from bitOffset to end-of-byte).
// Steps per iteration:
//  1. Determine how many bits to grab: min(bits remaining in current byte, bits still needed).
//  2. Right-shift the current byte to discard already-consumed low bits (bitOffset).
//  3. Mask off the high bits we don't want (if grabbing fewer than 8 bits).
//  4. OR the extracted bits into the result at the correct output position (outBitOffset).
//  5. Advance both the stream cursor and the output position.
//
// Reference: loosely based on canboat/canboat pgn.c, but corrected for true LSB-first
// byte ordering as observed on real CAN bus traffic.
func (s *PGNDataStream) getNumberRaw(bitLength uint16) (uint64, error) {
	var ret uint64

	outBitOffset := 0 // tracks where in the output uint64 to place the next chunk

	for bitLength > 0 {
		if int(s.byteOffset) >= len(s.data) {
			return 0, fmt.Errorf("reading byte(%d) off end of pgn (len:%d)", s.byteOffset, len(s.data))
		}

		// How many bits remain in the current source byte from the current bit position.
		bitsToGrab := 8 - s.bitOffset
		if bitLength < uint16(bitsToGrab) {
			bitsToGrab = uint8(bitLength)
		}

		// Extract bits from the current byte:
		// 1. Shift right to drop the already-consumed low bits.
		b := s.data[s.byteOffset]
		b >>= s.bitOffset
		// 2. Mask to keep only the bits we want (when grabbing fewer than a full byte).
		if bitsToGrab < 8 {
			mask := uint8(0xFF >> uint8(8-bitsToGrab))
			b &= mask
		}
		// 3. Place the extracted bits at the correct position in the output value.
		ret |= uint64(b) << uint64(outBitOffset)
		outBitOffset += int(bitsToGrab)
		bitLength -= uint16(bitsToGrab)

		// Advance the stream cursor.
		s.bitOffset += bitsToGrab
		if s.bitOffset >= 8 {
			s.bitOffset -= 8
			s.byteOffset++
		}
	}

	return ret, nil
}

// getNullableNumberRaw reads bitLength bits and applies NMEA 2000 null detection.
// In the NMEA 2000 encoding, a field's maximum representable value means "data not available":
//   - For unsigned fields: all bits set (e.g., 0xFF for 8-bit, 0xFFFF for 16-bit)
//   - For signed fields: the positive maximum (e.g., 0x7F for 8-bit, 0x7FFF for 16-bit)
//
// When the raw value matches the null sentinel, this method returns (nil, nil) instead of
// a value pointer, allowing callers to distinguish "no data" from valid data.
func (s *PGNDataStream) getNullableNumberRaw(bitLength uint16, signed bool) (*uint64, error) {
	v, err := s.getNumberRaw(bitLength)
	if err != nil {
		return nil, err
	}

	// Compute the null sentinel: start with all 64 bits set, then shift right to keep
	// only the bits that fit in the field width. For signed fields, shift one more bit
	// to get the positive maximum (the sign bit is excluded from the sentinel).
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

// getUnsignedNullableNumber reads an unsigned integer with null detection.
// Returns nil when the field contains the unsigned null sentinel (all bits set).
func (s *PGNDataStream) getUnsignedNullableNumber(bitLength uint16) (*uint64, error) {
	return s.getNullableNumberRaw(bitLength, false)
}

// getSignedNullableNumber reads a signed two's-complement integer with null detection.
// Returns nil when the field contains the signed null sentinel (positive max value).
//
// Sign extension works by checking the sign bit (the highest bit of the field):
//   - If the sign bit is clear, the value is non-negative and returned as-is.
//   - If the sign bit is set, the value is negative. The sign bit is stripped via XOR,
//     and the result is computed as: -(2^(bitLength-1)) + remaining_bits.
//     This is equivalent to standard two's-complement interpretation but avoids
//     overflow issues with Go's uint64-to-int64 conversion for arbitrary bit widths.
func (s *PGNDataStream) getSignedNullableNumber(bitLength uint16) (*int64, error) {
	v, err := s.getNullableNumberRaw(bitLength, true)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}

	// Check the sign bit (most significant bit of the field).
	mask := uint64(1 << (bitLength - 1))
	if *v&mask > 0 {
		// Negative value: strip the sign bit, then compute the two's-complement result.
		// Example for 8-bit -44: raw=0xD4, mask=0x80, *v^mask=0x54=84, result=-128+84=-44.
		*v ^= mask
		vi := -int64(mask) + int64(*v)
		return &vi, nil
	}
	// Non-negative: safe to cast directly.
	vi := int64(*v)
	return &vi, nil
}

// readVariableData reads a field whose length or encoding depends on its type descriptor.
// It looks up the FieldDescriptor for the given PGN, manufacturer ID, and field index,
// then dispatches to the appropriate reader:
//   - For variable-length STRING_LAU fields: uses readStringWithLengthAndControl
//   - For all other fields: reads BitLength bits (rounded up to a byte boundary) as raw binary
//
// This method is used by generated decoders for "KeyValue" style PGNs where the field
// type is determined at runtime rather than at code-generation time.
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
		// Round BitLength up to the nearest byte boundary using bit-clear of the low 3 bits.
		len := (field.BitLength + 7) &^ 0x7
		return s.readBinaryData(len)
	} else {
		return nil, err
	}
}
