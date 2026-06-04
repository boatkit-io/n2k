// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package pgn provides PGN (Parameter Group Number) parsing and encoding functionality for NMEA 2000 messages.
package pgn

// MaxPGNLength is the maximum length in bytes for a PGN data payload.
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

// IsProprietaryPGN reports whether pgn falls into a proprietary PGN range.
func IsProprietaryPGN(pgn uint32) bool {
	switch {
	case pgn >= 0x0EF00 && pgn <= 0x0EFFF:
		// proprietary PDU1 (addressed) single-frame range 0EF00 to 0xEFFF (61184 - 61439) messages.
		// Addressed means that you send it to specific node on the bus. This you can easily use for responding,
		// since you know the sender. For sender it is bit more complicate since your device address may change
		// due to address claiming. There is N2kDeviceList module for handling devices on bus and find them by
		// "NAME" (= 64 bit value set by SetDeviceInformation ).
		return true
	case pgn >= 0x0FF00 && pgn <= 0x0FFFF:
		// proprietary PDU2 (non addressed) single-frame range 0xFF00 to 0xFFFF (65280 - 65535).
		// Non addressed means that destination wil be 255 (=broadcast) so any cabable device can handle it.
		return true
	case pgn >= 0x1EF00 && pgn <= 0x1EFFF:
		// proprietary PDU1 (addressed) fast-packet PGN range 0x1EF00 to 0x1EFFF (126720 - 126975)
		return true
	case pgn >= 0x1FF00 && pgn <= 0x1FFFF:
		// proprietary PDU2 (non addressed) fast packet range 0x1FF00 to 0x1FFFF (130816 - 131071)
		return true
	default:
		return false
	}
}
