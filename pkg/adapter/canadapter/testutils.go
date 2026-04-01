package canadapter

import (
	"strconv"
	"strings"

	"github.com/brutella/can"
)

// CanFrameFromRaw parses a CSV-formatted raw CAN bus log line into a can.Frame suitable
// for testing and replay. The expected input format (matching common N2K replay tools) is:
//
//	timestamp,priority,pgn,source,destination,length,byte0,byte1,...,byteN
//
// For example:
//
//	"2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
//
// The function reconstructs the 29-bit CAN extended ID from the parsed fields using
// CanIdFromData, and always sets the frame Length to 8 (standard CAN data frame size).
// Data bytes beyond the declared length are left at their zero default value.
//
// Note: This function ignores parse errors on individual fields for simplicity in test
// code. In production, a more robust parser would be needed.
//
// Parameters:
//   - in: A comma-separated string in the format described above.
//
// Returns a can.Frame with the reconstructed CAN ID and parsed data bytes.
func CanFrameFromRaw(in string) can.Frame {
	// Split the CSV line into its component fields.
	elems := strings.Split(in, ",")

	// Parse the individual NMEA 2000 fields from the CSV columns.
	// Column 0: timestamp (ignored for frame construction)
	// Column 1: priority (3-bit value, 0-7)
	priority, _ := strconv.ParseUint(elems[1], 10, 8)
	// Column 2: PGN (Parameter Group Number, up to 18 bits)
	pgn, _ := strconv.ParseUint(elems[2], 10, 32)
	// Column 3: source address (8-bit NMEA 2000 bus address)
	source, _ := strconv.ParseUint(elems[3], 10, 8)
	// Column 4: destination address (8-bit, 255 = broadcast)
	destination, _ := strconv.ParseUint(elems[4], 10, 8)
	// Column 5: data length (number of data bytes following)
	length, _ := strconv.ParseUint(elems[5], 10, 8)

	// Reconstruct the 29-bit CAN extended ID by encoding PGN, source, priority, and
	// destination into the appropriate bit positions.
	id := CanIdFromData(uint32(pgn), uint8(source), uint8(priority), uint8(destination))
	retval := can.Frame{
		ID:     id,
		Length: 8, // CAN data frames always carry 8 bytes on NMEA 2000
	}

	// Parse each hex-encoded data byte from columns 6 onward.
	for i := 0; i < int(length); i++ {
		b, _ := strconv.ParseUint(elems[i+6], 16, 8)
		retval.Data[i] = uint8(b)
	}

	return retval
}

// CanIdFromData constructs a 29-bit CAN extended identifier from its component NMEA 2000
// fields. The resulting ID follows the NMEA 2000 / ISO 11783 bit layout:
//
//	Bits 28-26: Priority (3 bits)
//	Bits 25-8:  PGN (18 bits, includes Data Page, PDU Format, and PDU Specific)
//	Bits 7-0:   Source Address (8 bits)
//
// Note: The destination parameter is OR'd into the low byte alongside the source address.
// This works correctly for broadcast PGNs (destination=0), but for addressed PGNs the
// destination should be encoded in the PS (PDU Specific) field within the PGN instead.
// The OR-based approach can cause bit collisions when both source and destination are
// non-zero -- see TestCanFrameFromRaw_DestinationBitCollision for documentation of this
// behavior. This is acceptable for test utilities since replay data typically uses the
// correct encoding.
//
// Parameters:
//   - pgn: The 18-bit Parameter Group Number
//   - sourceId: The 8-bit source address of the sender
//   - priority: The 3-bit message priority (0 = highest, 7 = lowest)
//   - destination: The 8-bit destination address (0 for broadcast PGNs, OR'd into low byte)
//
// Returns the assembled 29-bit CAN ID as a uint32.
func CanIdFromData(pgn uint32, sourceId uint8, priority uint8, destination uint8) uint32 {
	// Assemble the CAN ID by placing each field in its bit position:
	// - Source address in bits 0-7
	// - PGN shifted left by 8 into bits 8-25
	// - Priority shifted left by 26 into bits 26-28
	// - Destination OR'd into the low byte (overlaps with source -- see note above)
	return uint32(sourceId) | (pgn << 8) | (uint32(priority) << 26) | uint32(destination)
}
