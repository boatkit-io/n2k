// Copyright (C) 2026 Boatkit
// SPDX-License-Identifier: MIT

package pgn

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeStringLAU(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected []byte
	}{
		{name: "empty", value: "", expected: []byte{0x02, 0x01}},
		{
			name:  "Actisense installation description",
			value: "Port engine fuel test",
			expected: []byte{
				0x17, 0x01, 'P', 'o', 'r', 't', ' ', 'e', 'n', 'g', 'i', 'n', 'e',
				' ', 'f', 'u', 'e', 'l', ' ', 't', 'e', 's', 't',
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := EncodeStringLAU(test.value)
			require.NoError(t, err)
			assert.Equal(t, test.expected, encoded)
		})
	}
}

func TestEncodeStringLAULengthLimit(t *testing.T) {
	encoded, err := EncodeStringLAU(strings.Repeat("a", 253))
	require.NoError(t, err)
	assert.Len(t, encoded, 255)

	_, err = EncodeStringLAU(strings.Repeat("a", 254))
	require.ErrorContains(t, err, "254 bytes")
}
