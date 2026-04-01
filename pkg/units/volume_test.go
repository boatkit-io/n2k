package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewVolume verifies that the constructor correctly stores the unit and value
// for all three volume units (Liter, MetersCubed, Gallon).
func TestNewVolume(t *testing.T) {
	v := NewVolume(Liter, 50)
	assert.Equal(t, Liter, v.Unit)
	assert.Equal(t, float32(50), v.Value)

	v2 := NewVolume(MetersCubed, 1)
	assert.Equal(t, MetersCubed, v2.Unit)
	assert.Equal(t, float32(1), v2.Value)

	v3 := NewVolume(Gallon, 10)
	assert.Equal(t, Gallon, v3.Unit)
	assert.Equal(t, float32(10), v3.Value)
}

// TestVolumeConvert tests the table-based volume conversion across all supported unit pairs.
// The conversion table uses MetersCubed as the reference unit (MetersCubed=1).
//
// Table values: MetersCubed=1, Liter=1000, Gallon=264.172
// Formula: value * (newConv / oldConv)
func TestVolumeConvert(t *testing.T) {
	// 1 m^3 -> Liter: 1 * (1000 / 1) = 1000 (1 cubic meter = 1000 liters)
	t.Run("MetersCubed to Liter", func(t *testing.T) {
		v := NewVolume(MetersCubed, 1)
		result := v.Convert(Liter)
		assert.InDelta(t, 1000.0, float64(result.Value), 0.1)
		assert.Equal(t, Liter, result.Unit)
	})

	// 1 m^3 -> Gallon: 1 * (264.172 / 1) = 264.172 US gallons
	t.Run("MetersCubed to Gallon", func(t *testing.T) {
		v := NewVolume(MetersCubed, 1)
		result := v.Convert(Gallon)
		assert.InDelta(t, 264.172, float64(result.Value), 0.1)
		assert.Equal(t, Gallon, result.Unit)
	})

	// 1000 Liters -> m^3: 1000 * (1 / 1000) = 1.0 (round-trip verification)
	t.Run("Liter to MetersCubed", func(t *testing.T) {
		v := NewVolume(Liter, 1000)
		result := v.Convert(MetersCubed)
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	})

	// 1 Liter -> Gallon: 1 * (264.172 / 1000) = 0.264172 (about a quarter gallon per liter)
	t.Run("Liter to Gallon", func(t *testing.T) {
		v := NewVolume(Liter, 1)
		result := v.Convert(Gallon)
		assert.InDelta(t, 0.264172, float64(result.Value), 0.001)
	})

	// 1 Gallon -> Liter: 1 * (1000 / 264.172) = 3.785 (about 3.785 liters per gallon)
	t.Run("Gallon to Liter", func(t *testing.T) {
		v := NewVolume(Gallon, 1)
		result := v.Convert(Liter)
		assert.InDelta(t, 3.785, float64(result.Value), 0.01)
	})

	// 264.172 Gallons -> m^3: 264.172 * (1 / 264.172) = 1.0 (round-trip verification)
	t.Run("Gallon to MetersCubed", func(t *testing.T) {
		v := NewVolume(Gallon, 264.172)
		result := v.Convert(MetersCubed)
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	})

	// Identity conversion: same unit should return the exact same value.
	t.Run("Identity conversion", func(t *testing.T) {
		v := NewVolume(Liter, 99.9)
		result := v.Convert(Liter)
		assert.Equal(t, float32(99.9), result.Value)
		assert.Equal(t, Liter, result.Unit)
	})
}

// TestVolumeAddSameUnit verifies same-unit addition produces a simple numeric sum.
func TestVolumeAddSameUnit(t *testing.T) {
	v1 := NewVolume(Liter, 50)
	v2 := NewVolume(Liter, 30)
	result := v1.Add(v2)
	assert.Equal(t, float32(80), result.Value)
	assert.Equal(t, Liter, result.Unit)
}

// TestVolumeAddDifferentUnits verifies cross-unit addition.
// 500 liters + 0.5 m^3 = 500 + 500 = 1000 liters (since 0.5 m^3 = 500 liters).
func TestVolumeAddDifferentUnits(t *testing.T) {
	v1 := NewVolume(Liter, 500)
	v2 := NewVolume(MetersCubed, 0.5)
	result := v1.Add(v2)
	assert.InDelta(t, 1000.0, float64(result.Value), 0.1)
	assert.Equal(t, Liter, result.Unit)
}

// TestVolumeSubSameUnit verifies same-unit subtraction produces a simple numeric difference.
func TestVolumeSubSameUnit(t *testing.T) {
	v1 := NewVolume(Gallon, 100)
	v2 := NewVolume(Gallon, 40)
	result := v1.Sub(v2)
	assert.Equal(t, float32(60), result.Value)
	assert.Equal(t, Gallon, result.Unit)
}

// TestVolumeSubDifferentUnits verifies cross-unit subtraction.
// 1 m^3 - 500 liters = 1.0 - 0.5 = 0.5 m^3 (since 500 liters = 0.5 m^3).
func TestVolumeSubDifferentUnits(t *testing.T) {
	v1 := NewVolume(MetersCubed, 1)
	v2 := NewVolume(Liter, 500)
	result := v1.Sub(v2)
	assert.InDelta(t, 0.5, float64(result.Value), 0.01)
	assert.Equal(t, MetersCubed, result.Unit)
}

// TestVolumeMarshalJSON verifies that the custom JSON marshaler produces the expected
// format with value, unit (as integer enum), and unitType (as UnitTypeVolume).
func TestVolumeMarshalJSON(t *testing.T) {
	v := NewVolume(Gallon, 10)
	data, err := json.Marshal(v)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.InDelta(t, 10.0, result["value"].(float64), 0.01)
	assert.Equal(t, float64(Gallon), result["unit"].(float64))
	assert.Equal(t, float64(UnitTypeVolume), result["unitType"].(float64))
}
