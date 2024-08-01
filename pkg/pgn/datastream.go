package pgn

// DataStream instances provide methods to read data types from a stream.
// byteOffset and bitOffset combine to act as the read "cursor".
// The low level read functions update the cursor.
type DataStream struct {
	data []uint8

	byteOffset uint16
	bitOffset  uint8
}

// NewDataStream returns a new DataStream. Call it with the data from a complete Packet.
func NewDataStream(data []uint8) *DataStream {
	return &DataStream{
		data:       data,
		byteOffset: 0,
		bitOffset:  0,
	}
}

// getBitOffset method returns the cursor in bits.
func (s *DataStream) getBitOffset() uint32 {
	return uint32(s.byteOffset)*8 + uint32(s.bitOffset)
}

// resetToStart method resets the stream. Commented out since its currently unused.
func (s *DataStream) resetToStart() {
	s.byteOffset = 0
	s.bitOffset = 0
}

func calcMaxPositiveValue(bitLength uint16, signed bool) uint64 {
	// calculate maximum valid value
	maxVal := uint64(0xFFFFFFFFFFFFFFFF)

	maxVal >>= 64 - bitLength // the largest value representable in length of field
	if signed {               // high bit set means it's negative, so maximum positive value is 1 bit shorter
		maxVal >>= 1 // we know it's a positive value, so safe for us to check.
	}
	switch bitLength {
	case 1: // leave alone
	case 2, 3: // for fields < 4 bits long, largest possible positive value indicates the field is missing
		maxVal -= 1
	default: // for larger fields, largest positive value means missing, that value minus 1 means invalid
		maxVal -= 2
	}
	return maxVal
}

func missingValue(bitLength uint16, signed bool) uint64 {
	var plus uint64
	switch bitLength {
	case 1:
	case 2, 3:
		plus = 1
	default:
		plus = 2
	}
	return calcMaxPositiveValue(bitLength, signed) + plus
}
