// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package canadapter

import (
	"strconv"
	"strings"

	"github.com/brutella/can"
)

// CanFrameFromRaw parses an input string into a [can.Frame].
//
//nolint:errcheck // Why: Needs error return later.
func CanFrameFromRaw(in string) can.Frame {
	elems := strings.Split(in, ",")
	priority, _ := strconv.ParseUint(elems[1], 10, 8)
	pgn, _ := strconv.ParseUint(elems[2], 10, 32)
	source, _ := strconv.ParseUint(elems[3], 10, 8)
	destination, _ := strconv.ParseUint(elems[4], 10, 8)
	length, _ := strconv.ParseUint(elems[5], 10, 8)

	id := CanIdFromData(uint32(pgn), uint8(source), uint8(priority), uint8(destination))
	retval := can.Frame{
		ID:     id,
		Length: 8,
	}
	for i := 0; i < int(length); i++ {
		b, _ := strconv.ParseUint(elems[i+6], 16, 8)
		retval.Data[i] = uint8(b)
	}

	return retval
}

// CanIdFromData returns an encoded ID from its inputs.
//
// Deprecated: Use [CanIDFromData] instead.
//
//nolint:revive // Why: Deprecated.
func CanIdFromData(pgn uint32, sourceId, priority, destination uint8) uint32 {
	return CanIDFromData(pgn, sourceId, priority, destination)
}

// CanIDFromData returns an encoded ID from its inputs.
func CanIDFromData(pgn uint32, sourceID, priority, destination uint8) uint32 {
	return uint32(sourceID) | (pgn << 8) | (uint32(priority) << 26) | uint32(destination)
}
