// Copyright (C) 2026 Boatkit
// SPDX-License-Identifier: MIT

package pgn

import "fmt"

const maxStringLAUUTF8Bytes = 253

// EncodeStringLAU encodes a UTF-8 STRING_LAU value for use in dynamic group-function fields.
func EncodeStringLAU(value string) ([]byte, error) {
	if len(value) > maxStringLAUUTF8Bytes {
		return nil, fmt.Errorf("STRING_LAU value is too long: %d bytes", len(value))
	}

	encoded := make([]byte, len(value)+2)
	encoded[0] = uint8(len(encoded)) //nolint:gosec // Length is bounded above.
	encoded[1] = 1
	copy(encoded[2:], value)
	return encoded, nil
}
