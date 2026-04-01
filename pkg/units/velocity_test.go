package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewVelocity verifies that the constructor correctly stores the unit and value
// for Knots and MetersPerSecond velocity units.
func TestNewVelocity(t *testing.T) {
	v := NewVelocity(Knots, 10)
	assert.Equal(t, Knots, v.Unit)
	assert.Equal(t, float32(10), v.Value)

	v2 := NewVelocity(MetersPerSecond, 5.5)
	assert.Equal(t, MetersPerSecond, v2.Unit)
	assert.Equal(t, float32(5.5), v2.Value)
}

// TestVelocityConvert tests the table-based velocity conversion across all supported unit pairs.
// The conversion table uses Knots as the reference unit (Knots=1), with other units expressed
// as "how many of unit X per knot".
//
// Table values: MetersPerSecond=0.514444, Knots=1, Mph=1.15078, Kph=1.852
// Formula: value * (newConv / oldConv)
func TestVelocityConvert(t *testing.T) {
	// 1 knot -> mph: 1 * (1.15078 / 1) = 1.15078 (1 knot = 1.15078 mph)
	t.Run("Knots to Mph", func(t *testing.T) {
		v := NewVelocity(Knots, 1)
		result := v.Convert(Mph)
		assert.InDelta(t, 1.15078, float64(result.Value), 0.01)
		assert.Equal(t, Mph, result.Unit)
	})

	// 1 knot -> kph: 1 * (1.852 / 1) = 1.852 (1 knot = 1.852 kph)
	t.Run("Knots to Kph", func(t *testing.T) {
		v := NewVelocity(Knots, 1)
		result := v.Convert(Kph)
		assert.InDelta(t, 1.852, float64(result.Value), 0.01)
	})

	// 1 knot -> m/s: 1 * (0.514444 / 1) = 0.514444 (1 knot = ~0.5144 m/s)
	t.Run("Knots to MetersPerSecond", func(t *testing.T) {
		v := NewVelocity(Knots, 1)
		result := v.Convert(MetersPerSecond)
		assert.InDelta(t, 0.5144, float64(result.Value), 0.01)
	})

	// 1.15078 mph -> knots: 1.15078 * (1 / 1.15078) = 1.0 (round-trip verification)
	t.Run("Mph to Knots", func(t *testing.T) {
		v := NewVelocity(Mph, 1.15078)
		result := v.Convert(Knots)
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	})

	// 1.852 kph -> knots: 1.852 * (1 / 1.852) = 1.0 (round-trip verification)
	t.Run("Kph to Knots", func(t *testing.T) {
		v := NewVelocity(Kph, 1.852)
		result := v.Convert(Knots)
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	})

	// 1 m/s -> kph: 1 * (1.852 / 0.514444) = 3.6 (the well-known m/s to kph factor)
	t.Run("MetersPerSecond to Kph", func(t *testing.T) {
		v := NewVelocity(MetersPerSecond, 1)
		result := v.Convert(Kph)
		assert.InDelta(t, 3.6, float64(result.Value), 0.01)
	})

	// Identity conversion: same unit should return exact same value.
	t.Run("Identity conversion", func(t *testing.T) {
		v := NewVelocity(Knots, 42)
		result := v.Convert(Knots)
		assert.Equal(t, float32(42), result.Value)
		assert.Equal(t, Knots, result.Unit)
	})
}

// TestVelocityAddSameUnit verifies that adding two velocities in the same unit
// produces a simple numeric sum with the unit preserved.
func TestVelocityAddSameUnit(t *testing.T) {
	v1 := NewVelocity(Knots, 5)
	v2 := NewVelocity(Knots, 3)
	result := v1.Add(v2)
	assert.Equal(t, float32(8), result.Value)
	assert.Equal(t, Knots, result.Unit)
}

// TestVelocityAddDifferentUnits verifies cross-unit addition.
// 1 knot + 1.15078 mph = 2 knots (since 1.15078 mph exactly equals 1 knot).
func TestVelocityAddDifferentUnits(t *testing.T) {
	v1 := NewVelocity(Knots, 1)
	v2 := NewVelocity(Mph, 1.15078)
	result := v1.Add(v2)
	assert.InDelta(t, 2.0, float64(result.Value), 0.01)
	assert.Equal(t, Knots, result.Unit)
}

// TestVelocitySubSameUnit verifies that subtracting two velocities in the same unit
// produces a simple numeric difference with the unit preserved.
func TestVelocitySubSameUnit(t *testing.T) {
	v1 := NewVelocity(Knots, 10)
	v2 := NewVelocity(Knots, 3)
	result := v1.Sub(v2)
	assert.Equal(t, float32(7), result.Value)
	assert.Equal(t, Knots, result.Unit)
}

// TestVelocitySubDifferentUnits verifies cross-unit subtraction.
// 2 knots - 1.15078 mph (= 1 knot) = 1 knot.
func TestVelocitySubDifferentUnits(t *testing.T) {
	v1 := NewVelocity(Knots, 2)
	v2 := NewVelocity(Mph, 1.15078)
	result := v1.Sub(v2)
	assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	assert.Equal(t, Knots, result.Unit)
}

// TestVelocityTimesTime verifies the velocity*time=distance calculation (d = v * t).
// The method converts to knots first, then multiplies by hours to get nautical miles.
//
// Test cases:
//   - 1 knot * 3600s (1 hour) = 1 NM (by definition of a knot)
//   - 10 knots * 1800s (30 min) = 5 NM
//   - 1 m/s * 3600s: first converts to ~1.9438 knots, then * 1 hour = 1.9438 NM
func TestVelocityTimesTime(t *testing.T) {
	// 1 knot * 3600 seconds = 1 nautical mile (this is the definition of a knot)
	v := NewVelocity(Knots, 1)
	d := v.TimesTime(3600)
	assert.InDelta(t, 1.0, float64(d.Value), 0.01)
	assert.Equal(t, NauticalMile, d.Unit)

	// 10 knots * 1800 seconds (30 min = 0.5 hours) = 5 nautical miles
	v2 := NewVelocity(Knots, 10)
	d2 := v2.TimesTime(1800)
	assert.InDelta(t, 5.0, float64(d2.Value), 0.01)
	assert.Equal(t, NauticalMile, d2.Unit)

	// Test with non-knot unit: 1 m/s * 3600s.
	// Step 1: Convert 1 m/s to knots: 1 * (1 / 0.514444) = 1.9438 knots
	// Step 2: 1.9438 knots * 1 hour = 1.9438 NM
	v3 := NewVelocity(MetersPerSecond, 1)
	d3 := v3.TimesTime(3600)
	assert.InDelta(t, 1.9438, float64(d3.Value), 0.01)
	assert.Equal(t, NauticalMile, d3.Unit)
}

// TestVelocityMarshalJSON verifies that the custom JSON marshaler produces the expected
// format with value, unit (as integer enum), and unitType (as UnitTypeVelocity).
func TestVelocityMarshalJSON(t *testing.T) {
	v := NewVelocity(Knots, 15)
	data, err := json.Marshal(v)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.InDelta(t, 15.0, result["value"].(float64), 0.01)
	assert.Equal(t, float64(Knots), result["unit"].(float64))
	assert.Equal(t, float64(UnitTypeVelocity), result["unitType"].(float64))
}
