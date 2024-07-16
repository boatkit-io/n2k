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
