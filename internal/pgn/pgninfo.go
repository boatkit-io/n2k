package pgn

const MaxPGNLength = 223

// fastPgnBits is generated in fastbits_generated.go

// IsFast returns true if the specified PGN is a Fast packet
func IsFast(pgn uint32) bool {
	if pgn < 126208 {
		return false // All PGNs < 126208 are single frame
	}

	// Check bit array for PGNs >= 126208
	bitIndex := pgn - 126208
	byteIndex := bitIndex / 8
	bitOffset := bitIndex % 8

	if byteIndex >= uint32(len(fastPgnBits)) {
		return false // PGN out of range, assume single frame
	}

	return (fastPgnBits[byteIndex] & (1 << bitOffset)) != 0
}

// IsProprietaryPGN returns true if its argument is in one of the proprietary ranges.
func IsProprietaryPGN(pgn uint32) bool {
	// Proprietary PGN ranges: 0xEF00-0xEFFF and 0x10000-0x1FFFF
	return (pgn >= 0xEF00 && pgn <= 0xEFFF) || (pgn >= 0x10000 && pgn <= 0x1FFFF)
}
