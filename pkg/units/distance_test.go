package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewDistance verifies that the constructor correctly stores the unit and value
// for Meter and Foot distance units.
func TestNewDistance(t *testing.T) {
	d := NewDistance(Meter, 100)
	assert.Equal(t, Meter, d.Unit)
	assert.Equal(t, float32(100), d.Value)

	d2 := NewDistance(Foot, 5280)
	assert.Equal(t, Foot, d2.Unit)
	assert.Equal(t, float32(5280), d2.Value)
}

// TestDistanceConvert tests the table-based distance conversion across various unit pairs.
// The conversion table uses Miles as the reference unit (Mile=1), with other units
// expressed as "how many of unit X per mile".
//
// The conversion formula is: value * (newConv / oldConv)
// Table values: Mile=1, Meter=1609.34, Foot=5280, NauticalMile=1.15078, Fathom=880
func TestDistanceConvert(t *testing.T) {
	// Mile -> Meter: 1 * (1609.34 / 1) = 1609.34
	t.Run("Mile to Meter", func(t *testing.T) {
		d := NewDistance(Mile, 1)
		result := d.Convert(Meter)
		assert.InDelta(t, 1609.34, float64(result.Value), 0.1)
		assert.Equal(t, Meter, result.Unit)
	})

	// Mile -> Foot: 1 * (5280 / 1) = 5280
	t.Run("Mile to Foot", func(t *testing.T) {
		d := NewDistance(Mile, 1)
		result := d.Convert(Foot)
		assert.InDelta(t, 5280.0, float64(result.Value), 0.1)
	})

	// Mile -> NauticalMile: 1 * (1.15078 / 1) = 1.15078
	// Note: The table stores NauticalMile=1.15078 which is the ratio of statute miles
	// per nautical mile (1 NM = 1.15078 statute miles). The conversion math produces
	// 1.15078 for 1 mile -> NM, which is the table's ratio value. See the extensive
	// comments in the original test for the mathematical reasoning.
	t.Run("Mile to NauticalMile", func(t *testing.T) {
		d := NewDistance(Mile, 1)
		result := d.Convert(NauticalMile)
		assert.InDelta(t, 1.15078, float64(result.Value), 0.01)
	})

	// Mile -> Fathom: 1 * (880 / 1) = 880 (1 mile = 880 fathoms, since 1 fathom = 6 feet)
	t.Run("Mile to Fathom", func(t *testing.T) {
		d := NewDistance(Mile, 1)
		result := d.Convert(Fathom)
		assert.InDelta(t, 880.0, float64(result.Value), 0.1)
	})

	// Meter -> Foot: 1 * (5280 / 1609.34) = 3.2808 feet per meter
	t.Run("Meter to Foot", func(t *testing.T) {
		d := NewDistance(Meter, 1)
		result := d.Convert(Foot)
		assert.InDelta(t, 3.28, float64(result.Value), 0.01)
	})

	// Foot -> Meter: 1 * (1609.34 / 5280) = 0.3048 meters per foot
	t.Run("Foot to Meter", func(t *testing.T) {
		d := NewDistance(Foot, 1)
		result := d.Convert(Meter)
		assert.InDelta(t, 0.3048, float64(result.Value), 0.001)
	})

	// Identity conversion: converting to the same unit should return the exact same value
	// (exercises the early-return shortcut in convertTableUnit).
	t.Run("Identity conversion", func(t *testing.T) {
		d := NewDistance(Meter, 42.5)
		result := d.Convert(Meter)
		assert.Equal(t, float32(42.5), result.Value)
		assert.Equal(t, Meter, result.Unit)
	})
}

// TestDistanceAddSameUnit verifies that adding two distances in the same unit
// produces a simple numeric sum with the unit preserved.
func TestDistanceAddSameUnit(t *testing.T) {
	d1 := NewDistance(Meter, 100)
	d2 := NewDistance(Meter, 200)
	result := d1.Add(d2)
	assert.Equal(t, float32(300), result.Value)
	assert.Equal(t, Meter, result.Unit)
}

// TestDistanceAddDifferentUnits verifies cross-unit addition where the second operand
// is converted to the first operand's unit before adding.
// - 1 Mile + 5280 Feet: 5280 feet = 1 mile, so result = 2 miles
// - 5280 Feet + 1 Mile: 1 mile = 5280 feet, so result = 10560 feet
// This demonstrates that the result unit always matches the receiver (first operand).
func TestDistanceAddDifferentUnits(t *testing.T) {
	// 1 mile + 5280 feet = 2 miles (result in miles, the unit of the receiver)
	d1 := NewDistance(Mile, 1)
	d2 := NewDistance(Foot, 5280)
	result := d1.Add(d2)
	assert.InDelta(t, 2.0, float64(result.Value), 0.01)
	assert.Equal(t, Mile, result.Unit)

	// Reverse: 5280 feet + 1 mile = 10560 feet (result in feet)
	result2 := d2.Add(d1)
	assert.InDelta(t, 10560.0, float64(result2.Value), 0.1)
	assert.Equal(t, Foot, result2.Unit)
}

// TestDistanceSubSameUnit verifies that subtracting two distances in the same unit
// produces a simple numeric difference with the unit preserved.
func TestDistanceSubSameUnit(t *testing.T) {
	d1 := NewDistance(Meter, 300)
	d2 := NewDistance(Meter, 100)
	result := d1.Sub(d2)
	assert.Equal(t, float32(200), result.Value)
	assert.Equal(t, Meter, result.Unit)
}

// TestDistanceSubDifferentUnits verifies cross-unit subtraction.
// 2 miles - 5280 feet (= 1 mile) = 1 mile.
func TestDistanceSubDifferentUnits(t *testing.T) {
	d1 := NewDistance(Mile, 2)
	d2 := NewDistance(Foot, 5280)
	result := d1.Sub(d2)
	assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	assert.Equal(t, Mile, result.Unit)
}

// TestDistanceMultiply verifies the scalar multiplication of a distance value.
// The unit is preserved and only the magnitude changes.
func TestDistanceMultiply(t *testing.T) {
	d := NewDistance(Meter, 10)
	result := d.Multiply(3)
	assert.Equal(t, float32(30), result.Value)
	assert.Equal(t, Meter, result.Unit)

	// Multiply by a fraction to verify non-integer scaling works.
	result2 := d.Multiply(0.5)
	assert.Equal(t, float32(5), result2.Value)
}

// TestDistanceMarshalJSON verifies that the custom JSON marshaler produces the expected
// format with value, unit (as integer enum), and unitType (as UnitTypeDistance).
func TestDistanceMarshalJSON(t *testing.T) {
	d := NewDistance(Foot, 100)
	data, err := json.Marshal(d)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.InDelta(t, 100.0, result["value"].(float64), 0.01)
	assert.Equal(t, float64(Foot), result["unit"].(float64))
	assert.Equal(t, float64(UnitTypeDistance), result["unitType"].(float64))
}
