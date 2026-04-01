package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVelocity(t *testing.T) {
	v := NewVelocity(Knots, 10)
	assert.Equal(t, Knots, v.Unit)
	assert.Equal(t, float32(10), v.Value)

	v2 := NewVelocity(MetersPerSecond, 5.5)
	assert.Equal(t, MetersPerSecond, v2.Unit)
	assert.Equal(t, float32(5.5), v2.Value)
}

func TestVelocityConvert(t *testing.T) {
	// Table: MetersPerSecond=0.514444, Knots=1, Mph=1.15078, Kph=1.852
	// Formula: value * (newConv / oldConv)

	t.Run("Knots to Mph", func(t *testing.T) {
		v := NewVelocity(Knots, 1)
		result := v.Convert(Mph)
		// 1 * (1.15078 / 1) = 1.15078
		assert.InDelta(t, 1.15078, float64(result.Value), 0.01)
		assert.Equal(t, Mph, result.Unit)
	})

	t.Run("Knots to Kph", func(t *testing.T) {
		v := NewVelocity(Knots, 1)
		result := v.Convert(Kph)
		// 1 * (1.852 / 1) = 1.852
		assert.InDelta(t, 1.852, float64(result.Value), 0.01)
	})

	t.Run("Knots to MetersPerSecond", func(t *testing.T) {
		v := NewVelocity(Knots, 1)
		result := v.Convert(MetersPerSecond)
		// 1 * (0.514444 / 1) = 0.514444
		assert.InDelta(t, 0.5144, float64(result.Value), 0.01)
	})

	t.Run("Mph to Knots", func(t *testing.T) {
		v := NewVelocity(Mph, 1.15078)
		result := v.Convert(Knots)
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	})

	t.Run("Kph to Knots", func(t *testing.T) {
		v := NewVelocity(Kph, 1.852)
		result := v.Convert(Knots)
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	})

	t.Run("MetersPerSecond to Kph", func(t *testing.T) {
		v := NewVelocity(MetersPerSecond, 1)
		result := v.Convert(Kph)
		// 1 * (1.852 / 0.514444) = 3.6
		assert.InDelta(t, 3.6, float64(result.Value), 0.01)
	})

	t.Run("Identity conversion", func(t *testing.T) {
		v := NewVelocity(Knots, 42)
		result := v.Convert(Knots)
		assert.Equal(t, float32(42), result.Value)
		assert.Equal(t, Knots, result.Unit)
	})
}

func TestVelocityAddSameUnit(t *testing.T) {
	v1 := NewVelocity(Knots, 5)
	v2 := NewVelocity(Knots, 3)
	result := v1.Add(v2)
	assert.Equal(t, float32(8), result.Value)
	assert.Equal(t, Knots, result.Unit)
}

func TestVelocityAddDifferentUnits(t *testing.T) {
	// 1 knot + 1.15078 mph = 2 knots (since 1.15078 mph = 1 knot)
	v1 := NewVelocity(Knots, 1)
	v2 := NewVelocity(Mph, 1.15078)
	result := v1.Add(v2)
	assert.InDelta(t, 2.0, float64(result.Value), 0.01)
	assert.Equal(t, Knots, result.Unit)
}

func TestVelocitySubSameUnit(t *testing.T) {
	v1 := NewVelocity(Knots, 10)
	v2 := NewVelocity(Knots, 3)
	result := v1.Sub(v2)
	assert.Equal(t, float32(7), result.Value)
	assert.Equal(t, Knots, result.Unit)
}

func TestVelocitySubDifferentUnits(t *testing.T) {
	v1 := NewVelocity(Knots, 2)
	v2 := NewVelocity(Mph, 1.15078)
	result := v1.Sub(v2)
	assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	assert.Equal(t, Knots, result.Unit)
}

func TestVelocityTimesTime(t *testing.T) {
	// 1 knot * 3600 seconds = 1 nautical mile
	v := NewVelocity(Knots, 1)
	d := v.TimesTime(3600)
	assert.InDelta(t, 1.0, float64(d.Value), 0.01)
	assert.Equal(t, NauticalMile, d.Unit)

	// 10 knots * 1800 seconds (30 min) = 5 nautical miles
	v2 := NewVelocity(Knots, 10)
	d2 := v2.TimesTime(1800)
	assert.InDelta(t, 5.0, float64(d2.Value), 0.01)
	assert.Equal(t, NauticalMile, d2.Unit)

	// Test with non-knot unit: 1 m/s * 3600s
	// 1 m/s = 1/0.514444 * 0.514444... actually it converts to knots first
	// 1 m/s = 1 * (1 / 0.514444) = 1.9438 knots
	// 1.9438 knots * 1 hour = 1.9438 NM
	v3 := NewVelocity(MetersPerSecond, 1)
	d3 := v3.TimesTime(3600)
	assert.InDelta(t, 1.9438, float64(d3.Value), 0.01)
	assert.Equal(t, NauticalMile, d3.Unit)
}

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
