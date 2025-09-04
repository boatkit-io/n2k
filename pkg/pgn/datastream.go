package pgn

import "math"

// DataStream instances provide methods to read/write data types to/from a stream.
// byteOffset and bitOffset combine to act as the "read/write cursor".
// The low level read/write functions update the cursor.
type DataStream struct {
	data []uint8

	byteOffset uint16
	bitOffset  uint8
}

// GetData returns the DataStream's current contents
func (s *DataStream) GetData() []uint8 {
	return s.data[:s.byteOffset]
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

// resetToStart method resets the stream. Used for testing.
func (s *DataStream) resetToStart() {
	s.byteOffset = 0
	s.bitOffset = 0
}

// remainingLength returns the number of bits remaining in the stream
func (s *DataStream) remainingLength() uint16 {
	totalBits := len(s.data)*8 - (int(s.byteOffset)*8 + int(s.bitOffset))
	return uint16(totalBits)
}

// calcMaxPositiveValue calculates the maximum value that can be represented
// with a given length of signed or unsigned contents.
// reservedValuesCount is the number of reserved values for the field. Valid values are 0-2.
func calcMaxPositiveValue(bitLength uint16, signed bool, reservedValuesCount int) uint64 {
	// calculate maximum valid value
	maxVal := uint64(0xFFFFFFFFFFFFFFFF)

	maxVal >>= 64 - bitLength // the largest value representable in length of field
	if signed {               // high bit set means it's negative, so maximum positive value is 1 bit shorter
		maxVal >>= 1 // we know it's a positive value, so safe for us to check.
	}

	if reservedValuesCount > 0 {
		if reservedValuesCount > 2 {
			reservedValuesCount = 2
		}
		maxVal -= uint64(reservedValuesCount)
	}

	return maxVal
}

// missingValue calculates the value representing a missing (nil) wire value
func missingValue(bitLength uint16, signed bool, reservedValuesCount int) uint64 {
	if reservedValuesCount == 0 {
		// No reserved values means we can't represent missing - return 0 as a safe default
		return 0
	}

	missing := uint64(0xFFFFFFFFFFFFFFFF)
	missing >>= 64 - bitLength // the largest value representable in length of field if unsigned
	if signed {                // high bit set means it's negative, so maximum positive value is 1 bit shorter
		missing >>= 1 // missing flag is max positive value; negative value has high bit set
	}
	return missing
}

// calcPrecision calculates the resulting precision of applying a given resolution to a given value
func calcPrecision(resolution float64) uint8 {
	precision := resolution
	digits := uint8(0)
	for precision >= 0 && precision < 1.0 {
		precision *= 10
		digits++
	}
	return digits
}

// roundFloat rounds a float64 to the specified precision
func roundFloat(val float64, precision uint8) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

// roundFloat32 rounds a float32 to the specified precision
func roundFloat32(val float32, precision uint8) float32 {
	ratio := math.Pow(10, float64(precision))
	return float32(math.Round(float64(val)*ratio) / ratio)
}
