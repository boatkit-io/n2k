package pgn

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecoding(t *testing.T) {
	info := MessageInfo{}

	rotRaw, err := DecodeRateOfTurn(info, NewDataStream([]uint8{0xff, 0xd4, 0xee, 0xff, 0xff, 0xff, 0xff, 0xff}))
	assert.NoError(t, err)
	rot := rotRaw.(RateOfTurn)
	assert.Equal(t, float64(-0.000137375), *rot.Rate)

	// Test repeating fields
	satsRaw, err := DecodeGnssSatsInView(info, NewDataStream([]byte{0xe3, 0xff, 0xc, 0x1, 0x68, 0x3, 0xcd, 0xba, 0x4, 0x10, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0x3, 0x68, 0x12, 0xb5, 0xd4, 0xcc, 0x10, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0xa, 0x22, 0x6, 0x43, 0x75, 0xf8, 0x11, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0xc, 0xc5, 0x4, 0x39, 0x19, 0xf8, 0x11, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0x16, 0x38, 0x37, 0x50, 0x2c, 0x4, 0x10, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0x19, 0xa2, 0x1c, 0x2d, 0x26, 0xa0, 0xf, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0x1a, 0xd0, 0x15, 0xfc, 0x86, 0xf8, 0x11, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0x1d, 0xdc, 0x8, 0x95, 0x47, 0xcc, 0x10, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0x1f, 0x73, 0x32, 0xc1, 0xc7, 0x68, 0x10, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0x20, 0xdc, 0x26, 0xb8, 0x4d, 0x5c, 0x12, 0xff, 0xff, 0xff, 0x7f, 0xf5, 0x2e, 0xdc, 0x8, 0x13, 0xa9, 0x4, 0x10, 0xff, 0xff, 0xff, 0x7f, 0xf1, 0x30, 0xe8, 0xa, 0x59, 0xa6, 0xcc, 0x10, 0xff, 0xff, 0xff, 0x7f, 0xf1}))
	assert.NoError(t, err)
	sats := satsRaw.(GnssSatsInView)
	assert.Equal(t, uint8(0xc), *sats.SatsInView)
	assert.Equal(t, 0xc, len(sats.Repeating1))

	// Test not-full-length ("older") pgns
	shortDecodeRaw, err := DecodeDcDetailedStatus(info, NewDataStream([]uint8{213, 0, 0, 53, 255, 100, 4, 255, 255}))
	assert.NoError(t, err)
	shortDecode := shortDecodeRaw.(DcDetailedStatus)
	assert.NotNil(t, shortDecode.TimeRemaining)
	assert.Nil(t, shortDecode.RippleVoltage)
	assert.Nil(t, shortDecode.RemainingCapacity)

	longDecodeRaw, err := DecodeDcDetailedStatus(info, NewDataStream([]uint8{213, 0, 0, 53, 255, 100, 4, 255, 255, 10, 20}))
	assert.NoError(t, err)
	longDecode := longDecodeRaw.(DcDetailedStatus)
	assert.NotNil(t, longDecode.TimeRemaining)
	assert.Nil(t, longDecode.RippleVoltage)
	assert.NotNil(t, longDecode.RemainingCapacity)
}
func TestGenerated(t *testing.T) {

	for _, pgn := range pgnList {
		for _, field := range pgn.Fields {
			switch field.CanboatType {
			case "VARIABLE", "BINARY":
				assert.True(t, field.BitOffset%8 == 0)
			case "STRING_LAU":
				assert.True(t, field.BitLengthVariable)
			case "DECIMAL":
				assert.Failf(t, "field:%s", field.Id)
			case "STRING_FIX":
				assert.True(t, field.BitOffset%8 == 0)
				assert.True(t, field.BitLength%8 == 0)
				assert.True(t, field.BitLength != 0)
			case "STRING_LZ":
				assert.True(t, field.BitLength != 0)
				assert.False(t, field.BitLengthVariable)
			case "MMSI", "DATE":
				assert.True(t, field.BitLength != 0)
				assert.False(t, field.BitLengthVariable)
				assert.True(t, field.Resolution == 1)
				assert.False(t, field.Signed)
			case "TIME":
				assert.False(t, field.BitLengthVariable)
				assert.True(t, field.BitLength != 0)

			}
			switch {
			case field.Resolution != 1:
				assert.True(t, strings.HasPrefix(field.GolangType, "*"))
			}

		}
	}
}
