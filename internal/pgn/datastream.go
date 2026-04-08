// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package pgn

import (
	"math"
)

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

// Position returns the current bit position in the stream
func (s *DataStream) Position() uint16 {
	return uint16(s.getBitOffset())
}

// SetPosition sets the stream position to the specified bit offset
func (s *DataStream) SetPosition(bitOffset uint16) {
	s.byteOffset = bitOffset / 8
	s.bitOffset = uint8(bitOffset % 8)
}

// remainingLength returns the number of bits remaining in the stream
/* func (s *DataStream) remainingLength() uint16 {
	totalBits := len(s.data)*8 - (int(s.byteOffset)*8 + int(s.bitOffset))
	return uint16(totalBits)
}
*/

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
