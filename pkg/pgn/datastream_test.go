// tests for datastream.go
package pgn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataStream_GetData(t *testing.T) {
	data := []uint8{1, 2, 3, 4, 5}
	stream := NewDataStream(data)

	assert.Equal(t, []uint8{}, stream.GetData())
}
func TestDataStream_getBitOffset(t *testing.T) {
	stream := NewDataStream([]uint8{1, 2, 3})

	// Initial offset should be 0
	assert.Equal(t, uint32(0), stream.getBitOffset())

	// Set some offsets and verify calculation
	stream.byteOffset = 1
	stream.bitOffset = 3
	assert.Equal(t, uint32(11), stream.getBitOffset()) // 1 byte (8 bits) + 3 bits = 11
}

func TestDataStream_resetToStart(t *testing.T) {
	stream := NewDataStream([]uint8{1, 2, 3})

	// Set some non-zero offsets
	stream.byteOffset = 2
	stream.bitOffset = 4

	// Reset and verify both are 0
	stream.resetToStart()
	assert.Equal(t, uint16(0), stream.byteOffset)
	assert.Equal(t, uint8(0), stream.bitOffset)
}

func TestCalcMaxPositiveValue(t *testing.T) {
	tests := []struct {
		name                string
		bitLength           uint16
		reservedValuesCount int
		signed              bool
		want                uint64
	}{
		{"1 bit unsigned", 1, 0, false, 1},
		{"1 bit signed", 1, 0, true, 0},
		{"2 bit unsigned", 2, 1, false, 0x2},
		{"2 bit signed", 2, 1, true, 0x0},
		{"3 bit unsigned", 3, 1, false, 0x6},
		{"3 bit signed", 3, 1, true, 0x2},
		{"4 bit unsigned", 4, 2, false, 0xD},
		{"4 bit signed", 4, 2, true, 0x5},
		{"8 bits unsigned", 8, 2, false, 0xFD},
		{"8 bits signed", 8, 2, true, 0x7D},
		{"16 bits unsigned", 16, 2, false, 0xFFFD},
		{"16 bits signed", 16, 2, true, 0x7FFD},
		{"32 bits unsigned", 32, 2, false, 0xFFFFFFFD},
		{"32 bits signed", 32, 2, true, 0x7FFFFFFD},
		{"64 bits unsigned", 64, 2, false, 0xFFFFFFFFFFFFFFFD},
		{"64 bits signed", 64, 2, true, 0x7FFFFFFFFFFFFFFD},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcMaxPositiveValue(tt.bitLength, tt.signed, tt.reservedValuesCount)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewDataStream(t *testing.T) {
	data := []uint8{1, 2, 3, 4, 5}
	stream := NewDataStream(data)

	assert.Equal(t, data, stream.data)
	assert.Equal(t, uint16(0), stream.byteOffset)
	assert.Equal(t, uint8(0), stream.bitOffset)
}

func TestMissingValue(t *testing.T) {
	tests := []struct {
		name      string
		length    int
		bitLength uint16
		signed    bool
		want      uint64
	}{
		{"1 bit unsigned", 1, 1, false, 1},
		{"1 bit signed", 1, 1, true, 0},
		{"2 bit unsigned", 2, 2, false, 3},
		{"2 bit signed", 2, 2, true, 1},
		{"3 bit unsigned", 3, 3, false, 7},
		{"3 bit signed", 3, 3, true, 3},
		{"4 bit unsigned", 4, 4, false, 15},
		{"4 bit signed", 4, 4, true, 7},
		{"8 bits unsigned", 8, 8, false, 255},
		{"8 bits signed", 8, 8, true, 127},
		{"16 bits unsigned", 16, 16, false, 65535},
		{"16 bits signed", 16, 16, true, 32767},
		{"32 bits unsigned", 32, 32, false, 4294967295},
		{"32 bits signed", 32, 32, true, 2147483647},
		{"64 bits unsigned", 64, 64, false, 18446744073709551615},
		{"64 bits signed", 64, 64, true, 9223372036854775807},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := missingValue(tt.bitLength, tt.signed, tt.length)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDataStream_readFixedString(t *testing.T) {
	tests := []struct {
		name    string
		data    []uint8
		length  uint16 // length in bits
		want    string
		wantErr bool
	}{
		{
			name:    "exact length string (40 bits = 5 bytes)",
			data:    []uint8{'H', 'e', 'l', 'l', 'o'},
			length:  40, // 5 bytes * 8 bits
			want:    "Hello",
			wantErr: false,
		},
		{
			name:    "string with null padding (64 bits = 8 bytes)",
			data:    []uint8{'T', 'e', 's', 't', 0, 0, 0, 0},
			length:  64, // 8 bytes * 8 bits
			want:    "Test",
			wantErr: false,
		},
		{
			name:    "string with 0xFF padding (48 bits = 6 bytes)",
			data:    []uint8{'A', 'B', 'C', 0xFF, 0xFF, 0xFF},
			length:  48, // 6 bytes * 8 bits
			want:    "ABC",
			wantErr: false,
		},
		{
			name:    "string with @ padding (40 bits = 5 bytes)",
			data:    []uint8{'X', 'Y', '@', '@', '@'},
			length:  40, // 5 bytes * 8 bits
			want:    "XY",
			wantErr: false,
		},
		{
			name:    "empty string - all null (32 bits = 4 bytes)",
			data:    []uint8{0, 0, 0, 0},
			length:  32, // 4 bytes * 8 bits
			want:    "",
			wantErr: false,
		},
		{
			name:    "empty string - all 0xFF (24 bits = 3 bytes)",
			data:    []uint8{0xFF, 0xFF, 0xFF},
			length:  24, // 3 bytes * 8 bits
			want:    "",
			wantErr: false,
		},
		{
			name:    "empty string - all @ (24 bits = 3 bytes)",
			data:    []uint8{'@', '@', '@'},
			length:  24, // 3 bytes * 8 bits
			want:    "",
			wantErr: false,
		},
		{
			name:    "mixed padding (40 bits = 5 bytes)",
			data:    []uint8{'H', 'i', 0, '@', 0xFF},
			length:  40, // 5 bytes * 8 bits
			want:    "Hi",
			wantErr: false,
		},
		{
			name:    "insufficient data (40 bits requested, 24 bits available)",
			data:    []uint8{'A', 'B', 'C'},
			length:  40, // requesting 5 bytes but only 3 available
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := NewDataStream(tt.data)

			got, err := stream.readFixedString(tt.length)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
