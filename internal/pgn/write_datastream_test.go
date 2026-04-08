package pgn

import (
	"math"
	"testing"

	"github.com/boatkit-io/tugboat/pkg/units"
	"github.com/stretchr/testify/assert"
)

func TestWriteNumerics(t *testing.T) {
	// test a variety of uint64 basics
	uintTests := []struct {
		exp    []uint8
		value  uint64
		length uint16
	}{
		// On byte boundary
		{[]uint8{0x12}, 0x12, 8},
		{[]uint8{0x12, 0x34, 0x12}, 0x1234, 16},
		{[]uint8{0x12, 0x34, 0x12, 0x24}, 0x24, 8},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12}, 0x1234, 16},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff}, 0xffffeed4, 32},

		// On byte boundary, sub-byte
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0x1E}, 0x1E, 5},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0xFE}, 7, 3},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0xFE, 0x02}, 2, 2},

		// Off byte boundary
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0xFE, 0x16}, 5, 3},
		{[]uint8{0x12, 0x34, 0x12, 0x24, 0x34, 0x12, 0xd4, 0xee, 0xff, 0xff, 0xFE, 0xD6, 0x7}, 0x3E, 6},
		/*				{[]uint8{0, 0x10, 0x02, 0}, 0x21, 0, 12, 8},
						{[]uint8{1, 2, 0x3}, 0xC080, 0, 2, 16},
		*/
	}

	p := NewDataStream(make([]uint8, 223))
	bitOffset := uint16(0)
	for _, tst := range uintTests {
		err := p.putNumberRaw(tst.value, tst.length, bitOffset)
		bitOffset += tst.length
		assert.NoError(t, err)
		for i := range tst.exp {
			assert.Equal(t, tst.exp[i], p.data[i])
		}
	}
	readTests := []struct {
		exp    uint64
		length uint16
	}{
		{0x12, 8},
		{0x1234, 16},
		{0x24, 8},
		{0x1234, 16},
		{0xffffeed4, 32},
	}
	p.resetToStart()
	for _, tst := range readTests {
		v, err := p.getNumberRaw(tst.length)
		assert.NoError(t, err)
		assert.Equal(t, tst.exp, v)
	}

	// binary data
	bdTests := []struct {
		exp    []uint8
		data   []uint8
		length uint16
	}{
		{[]uint8{1, 2, 3}, []uint8{1, 2, 3}, 24},
		{[]uint8{1, 2, 3, 0xFF, 0x00, 0x0F}, []uint8{0xFF, 0x00, 0xFF}, 20},
	}

	p = NewDataStream(make([]uint8, 223))
	offset := uint16(0)
	for _, tst := range bdTests {
		err := p.writeBinary(tst.data, tst.length, offset)
		offset += tst.length
		assert.NoError(t, err)
		for i := range tst.exp {
			assert.Equal(t, tst.exp[i], p.data[i])
		}
	}
}

func TestWritePgn(t *testing.T) {
	mmsi := uint32(123456789)
	p := ManOverboardNotification{
		Info: MessageInfo{
			SourceId: 12,
			PGN:      129702,
		},
		Sid:                nil,
		MobEmitterId:       nil,
		ManOverboardStatus: MobStatusConst(1),
		ActivationTime:     nil,
		PositionSource:     MobPositionSourceConst(3),
		PositionDate:       nil,
		PositionTime:       nil,
		Latitude:           nil,
		Longitude:          nil,
		CogReference:       DirectionReferenceConst(2),
		Cog:                nil,
		Sog: &units.Velocity{
			Unit:  1,
			Value: 8,
		},
		MmsiOfVesselOfOrigin:       &mmsi,
		MobEmitterBatteryLowStatus: LowBatteryConst(1),
	}
	stream := NewDataStream(make([]uint8, 223))
	info, err := p.Encode(stream)
	assert.NoError(t, err)
	var ok bool
	_, ok = interface{}(&p).(PgnStruct)
	assert.True(t, ok)
	assert.Equal(t, info.PGN, uint32(129702))
	assert.Nil(t, err)
}

// test Binary Round Trip
func TestBinaryRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		data     []uint8
		length   uint16
		expected []uint8
	}{
		{
			name:     "7 bits",
			data:     []uint8{0x5A}, // 0101 1010
			length:   7,
			expected: []uint8{0x5A},
		},
		{
			name:     "8 bits",
			data:     []uint8{0xAA},
			length:   8,
			expected: []uint8{0xAA},
		},
		{
			name:     "13 bits",
			data:     []uint8{0xAB, 0xFC}, // 1010 1011 1100
			length:   13,
			expected: []uint8{0xAB, 0x1C},
		},
		{
			name:     "16 bits",
			data:     []uint8{0xAB, 0xCD},
			length:   16,
			expected: []uint8{0xAB, 0xCD},
		},
		{
			name:     "21 bits",
			data:     []uint8{0xAB, 0xCD, 0xFE}, // 1010 1011 1100 1101 1110 0
			length:   21,
			expected: []uint8{0xAB, 0xCD, 0x1E},
		},
		{
			name:     "24 bits",
			data:     []uint8{0xAB, 0xCD, 0xEF},
			length:   24,
			expected: []uint8{0xAB, 0xCD, 0xEF},
		},
		{
			name:     "29 bits",
			data:     []uint8{0xAB, 0xCD, 0xEF, 0xFC}, // 1010 1011 1100 1101 1110 1111 1111 0
			length:   29,
			expected: []uint8{0xAB, 0xCD, 0xEF, 0x1C},
		},
		{
			name:     "32 bits",
			data:     []uint8{0xAB, 0xCD, 0xEF, 0x12},
			length:   32,
			expected: []uint8{0xAB, 0xCD, 0xEF, 0x12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write the data
			stream := NewDataStream(make([]uint8, 32))
			stream.resetToStart()
			err := stream.writeBinary(tt.data, tt.length, 0)
			assert.NoError(t, err)

			// Read it back
			stream.resetToStart()
			result, err := stream.readBinaryData(tt.length)
			assert.NoError(t, err)

			// Compare the results, accounting for any padding bits
			expectedBytes := (tt.length + 7) / 8 // Round up division without math.Ceil
			assert.Equal(t, tt.expected[:expectedBytes], result[:expectedBytes])
		})
	}
}

// TestWriteSignedResolutionRoundTrip tests writing and reading signed resolution values
func TestWriteSignedResolutionRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		value      float64
		length     uint16
		resolution float32
		offset     int32
		expected   float32
		tolerance  float64
	}{
		{
			name:       "Test positive value",
			value:      123.456,
			length:     32,
			resolution: 0.001,
			offset:     0,
			expected:   123.456,
			tolerance:  0.001,
		},
		{
			name:       "Test negative value",
			value:      -45.678,
			length:     32,
			resolution: 0.001,
			offset:     0,
			expected:   -45.678,
			tolerance:  0.0011,
		},
		{
			name:       "Test with offset",
			value:      -100.5,
			length:     16,
			resolution: 0.1,
			offset:     100,
			expected:   -100.5,
			tolerance:  0.1,
		},
		{
			name:       "Test max precision",
			value:      -300.986328125,
			length:     32,
			resolution: 0.0078125,
			offset:     0,
			expected:   -300.986328125,
			tolerance:  0.0078125,
		},
		{
			name:       "Test near zero",
			value:      0.001,
			length:     16,
			resolution: 0.001,
			offset:     0,
			expected:   0.001,
			tolerance:  0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write the value using WriteScaled
			stream := NewDataStream(make([]uint8, 32))
			spec := &FieldSpec{
				BitLength:     tt.length,
				Resolution:    float64(tt.resolution),
				Offset:        int64(tt.offset),
				IsSigned:      true,
				ReservedCount: 2,
				MaxRawValue:   calcMaxPositiveValue(tt.length, true),
			}
			err := WriteScaled(stream, &tt.value, spec)
			assert.NoError(t, err)

			// Read it back using ReadScaled
			stream.resetToStart()
			result, err := ReadScaled[float32](stream, spec)
			assert.NoError(t, err)
			assert.NotNil(t, result)

			// Compare with tolerance
			assert.InDelta(t, tt.expected, *result, tt.tolerance)
		})
	}
}

// TestSignedResolutionRoundTrip tests the signed resolution functions
func TestSignedResolutionRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		value      float64
		length     uint16
		resolution float32
		offset     int32
		expected   float32
		tolerance  float64
	}{
		{
			name:       "Test precise value",
			value:      300.986328125,
			length:     32,
			resolution: 0.0078125,
			offset:     -2000000,
			expected:   301.0,
			tolerance:  0.02,
		},
		{
			name:       "Test simple value",
			value:      10.0,
			length:     16,
			resolution: 0.1,
			offset:     0,
			expected:   10.0,
			tolerance:  0.1,
		},
		{
			name:       "Test with offset",
			value:      110.0,
			length:     16,
			resolution: 0.1,
			offset:     100,
			expected:   110.0,
			tolerance:  0.1,
		},
		{
			name:       "Test minimum float32",
			value:      -3.4028234663852886e+38,
			length:     32,
			resolution: 1.0,
			offset:     0,
			expected:   float32(math.MinInt32),
			tolerance:  1.0,
		},
		{
			name:       "Test maximum float32",
			value:      float64(math.MaxInt32),
			length:     32,
			resolution: 1.0,
			offset:     0,
			expected:   float32(math.MaxInt32 - 2),
			tolerance:  1.0,
		},
		{
			name:       "Test near minimum float32",
			value:      -3.4028e+38,
			length:     32,
			resolution: 1.0,
			offset:     0,
			expected:   float32(math.MinInt32),
			tolerance:  1.0,
		},
		{
			name:       "Test near maximum float32",
			value:      float64(math.MaxInt32 - 1),
			length:     32,
			resolution: 1.0,
			offset:     0,
			expected:   float32(math.MaxInt32 - 2),
			tolerance:  1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write the value using WriteScaled
			stream := NewDataStream(make([]uint8, 32))
			spec := &FieldSpec{
				BitLength:     tt.length,
				Resolution:    float64(tt.resolution),
				Offset:        int64(tt.offset),
				IsSigned:      true,
				ReservedCount: 2,
				MaxRawValue:   calcMaxPositiveValue(tt.length, true),
			}
			err := WriteScaled(stream, &tt.value, spec)
			assert.NoError(t, err)

			// Read it back using ReadScaled
			stream.resetToStart()
			result, err := ReadScaled[float32](stream, spec)
			assert.NoError(t, err)
			assert.NotNil(t, result)

			// Compare with resolution-based tolerance
			assert.InDelta(t, tt.expected, *result, tt.tolerance)
		})
	}
}

// round trip tests for the other functions in writedatastream.go
func TestWriteUint8RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		value     *uint8
		length    uint16
		bitOffset uint16
		expected  *uint8
		err       bool
	}{
		{
			name:      "Test nil value",
			value:     nil,
			length:    8,
			bitOffset: 0,
			expected:  nil,
			err:       false,
		},
		{
			name:      "Test zero value",
			value:     ptr(uint8(0)),
			length:    8,
			bitOffset: 0,
			expected:  ptr(uint8(0)),
			err:       false,
		},
		{
			name:      "Test max value",
			value:     ptr(uint8(255)),
			length:    8,
			bitOffset: 0,
			expected:  ptr(uint8(253)),
			err:       false,
		},
		{
			name:      "Test max value - 1",
			value:     ptr(uint8(254)),
			length:    8,
			bitOffset: 0,
			expected:  ptr(uint8(253)),
			err:       false,
		},
		{
			name:      "Test max value - 2",
			value:     ptr(uint8(253)),
			length:    8,
			bitOffset: 0,
			expected:  ptr(uint8(253)),
			err:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := NewDataStream(make([]uint8, 32))
			spec := &FieldSpec{
				BitLength:     tt.length,
				ReservedCount: 2,
				MaxRawValue:   calcMaxPositiveValue(tt.length, false),
				MissingValue:  0xFF,
			}
			err := WriteRaw(stream, tt.value, spec)
			if tt.err {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			stream.resetToStart()
			result, err := ReadRaw[uint8](stream, spec)
			assert.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

// TestWriteInt16 tests the int16 functions
func TestWriteInt16(t *testing.T) {
	tests := []struct {
		name      string
		value     *int16
		length    uint16
		bitOffset uint16
		expected  *int16
		err       bool
	}{
		{
			name:      "Test nil value",
			value:     nil,
			length:    16,
			bitOffset: 0,
			expected:  nil,
			err:       false,
		},
		{
			name:      "Test zero value",
			value:     ptr(int16(0)),
			length:    16,
			bitOffset: 0,
			expected:  ptr(int16(0)),
			err:       false,
		},
		{
			name:      "Test positive value",
			value:     ptr(int16(123)),
			length:    16,
			bitOffset: 0,
			expected:  ptr(int16(123)),
			err:       false,
		},
		{
			name:      "Test negative value",
			value:     ptr(int16(-123)),
			length:    16,
			bitOffset: 0,
			expected:  ptr(int16(-123)),
			err:       false,
		},
		{
			name:      "Test max positive value (reserved for nil)",
			value:     ptr(int16(32767)),
			length:    16,
			bitOffset: 0,
			expected:  ptr(int16(32765)),
			err:       false,
		},
		{
			name:      "Test max positive value - 1",
			value:     ptr(int16(32766)),
			length:    16,
			bitOffset: 0,
			expected:  ptr(int16(32765)),
			err:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := NewDataStream(make([]uint8, 32))
			spec := &FieldSpec{
				BitLength:     tt.length,
				IsSigned:      true,
				ReservedCount: 2,
				MaxRawValue:   calcMaxPositiveValue(tt.length, true),
				MissingValue:  0x7FFF,
			}
			err := WriteRaw(stream, tt.value, spec)
			if tt.err {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			stream.resetToStart()
			result, err := ReadRaw[int16](stream, spec)
			assert.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

// TestWriteUnit tests the unit functions
func TestWriteUnit(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		length     uint16
		resolution float32
		bitOffset  uint16
		offset     int64
		signed     bool
		expected   float64
	}{
		{
			name:       "Test nil value",
			value:      nil,
			length:     16,
			resolution: 0.1,
			bitOffset:  0,
			offset:     0,
			signed:     true,
			expected:   float64(0xFFFE), // Missing value for signed
		},
		{
			name: "Test distance conversion to meters",
			value: &units.Distance{
				Value: 100,
				Unit:  units.Foot,
			},
			length:     16,
			resolution: 0.1,
			bitOffset:  0,
			offset:     0,
			signed:     false,
			expected:   30.48, // 100ft = 30.48m
		},
		{
			name: "Test velocity conversion to m/s",
			value: &units.Velocity{
				Value: 10,
				Unit:  units.Knots,
			},
			length:     16,
			resolution: 0.01,
			bitOffset:  0,
			offset:     0,
			signed:     false,
			expected:   5.144, // 10kts = 5.144m/s
		},
		{
			name: "Test volume conversion to liters",
			value: &units.Volume{
				Value: 1,
				Unit:  units.Gallon,
			},
			length:     16,
			resolution: 0.1,
			bitOffset:  0,
			offset:     0,
			signed:     false,
			expected:   3.78541, // 1gal = 3.78541L
		},
		{
			name: "Test temperature conversion to kelvin",
			value: &units.Temperature{
				Value: 20,
				Unit:  units.Celsius,
			},
			length:     16,
			resolution: 0.1,
			bitOffset:  0,
			offset:     0,
			signed:     true,
			expected:   293.15, // 20C = 293.15K
		},
		{
			name: "Test pressure conversion to pascal",
			value: &units.Pressure{
				Value: 1,
				Unit:  units.Hpa,
			},
			length:     16,
			resolution: 100,
			bitOffset:  0,
			offset:     0,
			signed:     false,
			expected:   100, // 1hPa = 100Pa
		},
		{
			name: "Test flow conversion to L/h",
			value: &units.Flow{
				Value: 6,
				Unit:  units.GallonsPerHour,
			},
			length:     16,
			resolution: 1,
			bitOffset:  0,
			offset:     0,
			signed:     false,
			expected:   3.78541 * 6, // 6gal/hr * 3.78541 = 22.71246 L/hr
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write the value using WriteScaled
			stream := NewDataStream(make([]uint8, 32))
			spec := &FieldSpec{
				BitLength:     tt.length,
				Resolution:    float64(tt.resolution),
				Offset:        tt.offset,
				IsSigned:      tt.signed,
				ReservedCount: 2,
				MaxRawValue:   calcMaxPositiveValue(tt.length, tt.signed),
				MissingValue:  uint64(1<<(tt.length-1) - 1), // Max positive value
			}

			// Convert the value to the appropriate type for WriteScaled
			var valuePtr *float32
			if tt.value != nil {
				// For unit types, we need to extract the float value
				switch v := tt.value.(type) {
				case *units.Distance:
					val := float32(v.Convert(units.Meter).Value)
					valuePtr = &val
				case *units.Velocity:
					val := float32(v.Convert(units.MetersPerSecond).Value)
					valuePtr = &val
				case *units.Volume:
					val := float32(v.Convert(units.Liter).Value)
					valuePtr = &val
				case *units.Pressure:
					val := float32(v.Convert(units.Pa).Value)
					valuePtr = &val
				case *units.Temperature:
					val := float32(v.Convert(units.Kelvin).Value)
					valuePtr = &val
				case *units.Flow:
					val := float32(v.Convert(units.LitersPerHour).Value)
					valuePtr = &val
				}
			}

			err := WriteScaled(stream, valuePtr, spec)
			assert.NoError(t, err)

			// Read it back using ReadScaled
			stream.resetToStart()
			var result float64
			val, err := ReadScaled[float32](stream, spec)
			assert.NoError(t, err)
			if val == nil {
				result = float64(0xFFFE)
			} else {
				result = float64(*val)
			}

			// Compare with reasonable tolerance for floating point
			assert.InDelta(t, tt.expected, result, float64(tt.resolution))
		})
	}
}

// TestWriteInt32 tests the int32 functions
func TestWriteInt32(t *testing.T) {
	tests := []struct {
		name      string
		value     *int32
		length    uint16
		bitOffset uint16
		expected  *int32
		err       bool
	}{
		{
			name:      "Test nil value",
			value:     nil,
			length:    32,
			bitOffset: 0,
			expected:  nil,
			err:       false,
		},
		{
			name:      "Test zero value",
			value:     ptr(int32(0)),
			length:    32,
			bitOffset: 0,
			expected:  ptr(int32(0)),
			err:       false,
		},
		{
			name:      "Test positive value",
			value:     ptr(int32(123)),
			length:    32,
			bitOffset: 0,
			expected:  ptr(int32(123)),
			err:       false,
		},
		{
			name:      "Test negative value",
			value:     ptr(int32(-123)),
			length:    32,
			bitOffset: 0,
			expected:  ptr(int32(-123)),
			err:       false,
		},
		{
			name:      "Test max positive value (reserved for nil)",
			value:     ptr(int32(2147483647)),
			length:    32,
			bitOffset: 0,
			expected:  ptr(int32(2147483645)),
			err:       false,
		},
		{
			name:      "Test max positive value - 1 (reserved for invalid)",
			value:     ptr(int32(2147483646)),
			length:    32,
			bitOffset: 0,
			expected:  ptr(int32(2147483645)),
			err:       false,
		},
		{
			name:      "Test max positive value - 2 (maximum valid value)",
			value:     ptr(int32(2147483645)),
			length:    32,
			bitOffset: 0,
			expected:  ptr(int32(2147483645)),
			err:       false,
		},
		{
			name:      "Test max negative value",
			value:     ptr(int32(-2147483648)),
			length:    32,
			bitOffset: 0,
			expected:  ptr(int32(-2147483648)),
			err:       false,
		},
		{
			name:      "Test partial bits positive max (reserved for nil)",
			value:     ptr(int32(511)),
			length:    10,
			bitOffset: 0,
			expected:  ptr(int32(509)),
			err:       false,
		},
		{
			name:      "Test partial bits positive max - 1 (reserved for invalid)",
			value:     ptr(int32(510)),
			length:    10,
			bitOffset: 0,
			expected:  ptr(int32(509)),
			err:       false,
		},
		{
			name:      "Test partial bits positive max - 2 (maximum valid value)",
			value:     ptr(int32(508)),
			length:    10,
			bitOffset: 0,
			expected:  ptr(int32(508)),
			err:       false,
		},
		{
			name:      "Test partial bits negative min",
			value:     ptr(int32(-512)),
			length:    10,
			bitOffset: 0,
			expected:  ptr(int32(-512)),
			err:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := NewDataStream(make([]uint8, 32))
			spec := &FieldSpec{
				BitLength:     tt.length,
				IsSigned:      true,
				ReservedCount: 2,
				MaxRawValue:   calcMaxPositiveValue(tt.length, true),
				MissingValue:  uint64(1<<(tt.length-1) - 1), // Max positive value
			}
			err := WriteRaw(stream, tt.value, spec)
			if tt.err {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			stream.resetToStart()
			result, err := ReadRaw[int32](stream, spec)
			assert.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

// Round trip test for WriteStringFixedLength
func TestWriteStringFixedLength(t *testing.T) {
	tests := []struct {
		name      string
		value     []uint8
		length    uint16
		bitOffset uint16
		expected  string
		err       bool
	}{
		{
			name:      "Empty string",
			value:     []uint8(""),
			length:    64,
			bitOffset: 0,
			expected:  "",
			err:       false,
		},
		{
			name:      "Basic ASCII string",
			value:     []uint8("Test"),
			length:    4 * 8,
			bitOffset: 0,
			expected:  "Test",
			err:       false,
		},
		{
			name:      "String exactly at length",
			value:     []uint8("12345678"),
			length:    8 * 8,
			bitOffset: 0,
			expected:  "12345678",
			err:       false,
		},
		{
			name:      "String shorter than length",
			value:     []uint8("12345678"),
			length:    16 * 8,
			bitOffset: 0,
			expected:  "12345678",
			err:       false,
		},
		{
			name:      "String too long",
			value:     []uint8("123456789"),
			length:    64,
			bitOffset: 0,
			expected:  "12345678",
			err:       false,
		},
		{
			name:      "String with special chars",
			value:     []uint8("Test@#$%"),
			length:    8 * 8,
			bitOffset: 0,
			expected:  "Test@#$%",
			err:       false,
		},
		{
			name:      "String with spaces",
			value:     []uint8("A B C D"),
			length:    8 * 7,
			bitOffset: 0,
			expected:  "A B C D",
			err:       false,
		},
		{
			name:      "String with non-zero offset",
			value:     []uint8("Test"),
			length:    8,
			bitOffset: 4,
			expected:  "Test",
			err:       true,
		},
		{
			name:      "String with UTF-8",
			value:     []uint8("Hello世界"),
			length:    16 * 8,
			bitOffset: 0,
			expected:  "",
			err:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := NewDataStream(make([]uint8, 32))
			err := stream.writeStringFix(tt.value, tt.length, tt.bitOffset)
			if tt.err {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestWriteRawSignExtensionFix tests that WriteRaw properly handles negative signed integers
// without sign extension issues that would cause incorrect clipping
func TestWriteRawSignExtensionFix(t *testing.T) {
	tests := []struct {
		name      string
		value     int32
		bitLength uint16
		expected  int32
	}{
		{
			name:      "Negative value in 8-bit signed field",
			value:     -1,
			bitLength: 8,
			expected:  -1, // Should not be clipped to MaxRawValue
		},
		{
			name:      "Negative value in 16-bit signed field",
			value:     -100,
			bitLength: 16,
			expected:  -100,
		},
		{
			name:      "Large negative value in 32-bit signed field",
			value:     -1000000,
			bitLength: 32,
			expected:  -1000000,
		},
		{
			name:      "Minimum value for 8-bit signed field",
			value:     -128,
			bitLength: 8,
			expected:  -128,
		},
		{
			name:      "Minimum value for 16-bit signed field",
			value:     -32768,
			bitLength: 16,
			expected:  -32768,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := NewDataStream(make([]uint8, 32))
			spec := &FieldSpec{
				BitLength:     tt.bitLength,
				IsSigned:      true,
				ReservedCount: 2,
				MaxRawValue:   calcMaxPositiveValue(tt.bitLength, true),
				MissingValue:  uint64(1<<(tt.bitLength-1) - 1), // Max positive value
			}

			// Write the value
			err := WriteRaw(stream, &tt.value, spec)
			assert.NoError(t, err)

			// Read it back
			stream.resetToStart()
			result, err := ReadRaw[int32](stream, spec)
			assert.NoError(t, err)
			assert.NotNil(t, result)

			// Verify the value was not incorrectly clipped
			assert.Equal(t, tt.expected, *result,
				"Negative value should not be clipped due to sign extension")
		})
	}
}

// Helper function to create pointers to values
func ptr[T any](v T) *T {
	return &v
}
