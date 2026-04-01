package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewTemperature verifies that the constructor correctly stores the unit and value
// for each supported temperature unit (Kelvin, Celsius, Fahrenheit).
func TestNewTemperature(t *testing.T) {
	temp := NewTemperature(Kelvin, 300)
	assert.Equal(t, Kelvin, temp.Unit)
	assert.Equal(t, float32(300), temp.Value)

	temp2 := NewTemperature(Celsius, 25.5)
	assert.Equal(t, Celsius, temp2.Unit)
	assert.Equal(t, float32(25.5), temp2.Value)

	temp3 := NewTemperature(Fahrenheit, 72)
	assert.Equal(t, Fahrenheit, temp3.Unit)
	assert.Equal(t, float32(72), temp3.Value)
}

// TestTemperatureIdentityConversion verifies that converting a temperature to its own unit
// returns the exact same value. This exercises the early-return optimization in Convert()
// and ensures no floating-point drift from unnecessary calculations.
func TestTemperatureIdentityConversion(t *testing.T) {
	temp := NewTemperature(Kelvin, 300)
	result := temp.Convert(Kelvin)
	assert.Equal(t, float32(300), result.Value)
	assert.Equal(t, Kelvin, result.Unit)

	temp2 := NewTemperature(Celsius, 50)
	result2 := temp2.Convert(Celsius)
	assert.Equal(t, float32(50), result2.Value)

	temp3 := NewTemperature(Fahrenheit, 100)
	result3 := temp3.Convert(Fahrenheit)
	assert.Equal(t, float32(100), result3.Value)
}

// TestTemperatureConvertKnownValues tests temperature conversions using two well-known
// reference points:
//   - Water freezing point: 0C = 273.15K = 32F
//   - Water boiling point: 100C = 373.15K = 212F
//
// These are universally known physical constants, making them ideal for verifying
// the correctness of the conversion formulas in all six directions.
func TestTemperatureConvertKnownValues(t *testing.T) {
	// === Water freezing point: 0C = 273.15K = 32F ===

	// Celsius -> Kelvin: 0 + 273.15 = 273.15
	t.Run("0C to K", func(t *testing.T) {
		temp := NewTemperature(Celsius, 0)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 273.15, float64(result.Value), 0.01)
		assert.Equal(t, Kelvin, result.Unit)
	})

	// Celsius -> Fahrenheit: (0 + 273.15) * 9/5 - 459.67 = 32
	t.Run("0C to F", func(t *testing.T) {
		temp := NewTemperature(Celsius, 0)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 32.0, float64(result.Value), 0.01)
		assert.Equal(t, Fahrenheit, result.Unit)
	})

	// Kelvin -> Celsius: 273.15 - 273.15 = 0
	t.Run("273.15K to C", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 273.15)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 0.0, float64(result.Value), 0.01)
	})

	// Kelvin -> Fahrenheit: 273.15 * 9/5 - 459.67 = 32
	t.Run("273.15K to F", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 273.15)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 32.0, float64(result.Value), 0.01)
	})

	// Fahrenheit -> Celsius: via Kelvin: (32 + 459.67) * 5/9 - 273.15 = 0
	t.Run("32F to C", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 32)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 0.0, float64(result.Value), 0.01)
	})

	// Fahrenheit -> Kelvin: (32 + 459.67) * 5/9 = 273.15
	t.Run("32F to K", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 32)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 273.15, float64(result.Value), 0.01)
	})

	// === Water boiling point: 100C = 373.15K = 212F ===

	t.Run("100C to K", func(t *testing.T) {
		temp := NewTemperature(Celsius, 100)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 373.15, float64(result.Value), 0.01)
	})

	t.Run("100C to F", func(t *testing.T) {
		temp := NewTemperature(Celsius, 100)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 212.0, float64(result.Value), 0.01)
	})

	t.Run("373.15K to C", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 373.15)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 100.0, float64(result.Value), 0.01)
	})

	t.Run("373.15K to F", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 373.15)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 212.0, float64(result.Value), 0.01)
	})

	t.Run("212F to C", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 212)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 100.0, float64(result.Value), 0.01)
	})

	t.Run("212F to K", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 212)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 373.15, float64(result.Value), 0.01)
	})
}

// TestTemperatureConvertAllDirections tests all six conversion directions using 300K as
// the starting point. 300K is approximately room temperature (26.85C / 80.33F), making
// it a practical real-world test value.
//
// This complements TestTemperatureConvertKnownValues by testing at a non-special temperature
// (not a freezing/boiling reference point) to ensure the formulas work generally.
func TestTemperatureConvertAllDirections(t *testing.T) {
	// K -> F: 300 * 9/5 - 459.67 = 540 - 459.67 = 80.33
	t.Run("K to F", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 300)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 80.33, float64(result.Value), 0.01)
	})

	// K -> C: 300 - 273.15 = 26.85
	t.Run("K to C", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 300)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 26.85, float64(result.Value), 0.01)
	})

	// F -> K: (80.33 + 459.67) * 5/9 = 540 * 5/9 = 300
	t.Run("F to K", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 80.33)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 300.0, float64(result.Value), 0.01)
	})

	// F -> C: via K: (80.33 + 459.67) * 5/9 - 273.15 = 300 - 273.15 = 26.85
	t.Run("F to C", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 80.33)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 26.85, float64(result.Value), 0.01)
	})

	// C -> K: 26.85 + 273.15 = 300
	t.Run("C to K", func(t *testing.T) {
		temp := NewTemperature(Celsius, 26.85)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 300.0, float64(result.Value), 0.01)
	})

	// C -> F: via K: (26.85 + 273.15) * 9/5 - 459.67 = 300 * 1.8 - 459.67 = 80.33
	t.Run("C to F", func(t *testing.T) {
		temp := NewTemperature(Celsius, 26.85)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 80.33, float64(result.Value), 0.01)
	})
}

// TestTemperatureMarshalJSON verifies that the custom JSON marshaler produces the expected
// format with value, unit (as integer enum), and unitType fields. The unitType field allows
// JSON consumers to identify this as a temperature measurement without knowing the unit enum.
func TestTemperatureMarshalJSON(t *testing.T) {
	temp := NewTemperature(Celsius, 25)
	data, err := json.Marshal(temp)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.InDelta(t, 25.0, result["value"].(float64), 0.01)
	assert.Equal(t, float64(Celsius), result["unit"].(float64))
	assert.Equal(t, float64(UnitTypeTemperature), result["unitType"].(float64))
}
