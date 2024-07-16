package pgn

import (
	"fmt"
)

func (s *DataStream) writeReserved(bitLength uint16, bitOffset uint16) error {
	if s.getBitOffset() != uint32(bitOffset) {
		return fmt.Errorf("attempt to write field at wrong offset in writeReserved: %d, %d", s.getBitOffset(), bitOffset)
	}
	return s.writeuint8(0xFF, bitLength)
}

func (s *DataStream) writeuint8(value uint8, length uint16) error {
	return nil
}

// putNumberRaw method writes up to 64 bits to the stream from a uint64 argument.
// Cribbed the getNumberRaw function
func (s *DataStream) putNumberRaw(value uint64, bitLength uint16) error {

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
