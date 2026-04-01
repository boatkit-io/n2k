package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestTemperatureConvertKnownValues(t *testing.T) {
	// 0°C = 273.15K = 32°F
	t.Run("0C to K", func(t *testing.T) {
		temp := NewTemperature(Celsius, 0)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 273.15, float64(result.Value), 0.01)
		assert.Equal(t, Kelvin, result.Unit)
	})

	t.Run("0C to F", func(t *testing.T) {
		temp := NewTemperature(Celsius, 0)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 32.0, float64(result.Value), 0.01)
		assert.Equal(t, Fahrenheit, result.Unit)
	})

	t.Run("273.15K to C", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 273.15)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 0.0, float64(result.Value), 0.01)
	})

	t.Run("273.15K to F", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 273.15)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 32.0, float64(result.Value), 0.01)
	})

	t.Run("32F to C", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 32)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 0.0, float64(result.Value), 0.01)
	})

	t.Run("32F to K", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 32)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 273.15, float64(result.Value), 0.01)
	})

	// 100°C = 373.15K = 212°F
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

func TestTemperatureConvertAllDirections(t *testing.T) {
	// K -> F
	t.Run("K to F", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 300)
		result := temp.Convert(Fahrenheit)
		// 300 * 9/5 - 459.67 = 540 - 459.67 = 80.33
		assert.InDelta(t, 80.33, float64(result.Value), 0.01)
	})

	// K -> C
	t.Run("K to C", func(t *testing.T) {
		temp := NewTemperature(Kelvin, 300)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 26.85, float64(result.Value), 0.01)
	})

	// F -> K
	t.Run("F to K", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 80.33)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 300.0, float64(result.Value), 0.01)
	})

	// F -> C
	t.Run("F to C", func(t *testing.T) {
		temp := NewTemperature(Fahrenheit, 80.33)
		result := temp.Convert(Celsius)
		assert.InDelta(t, 26.85, float64(result.Value), 0.01)
	})

	// C -> K
	t.Run("C to K", func(t *testing.T) {
		temp := NewTemperature(Celsius, 26.85)
		result := temp.Convert(Kelvin)
		assert.InDelta(t, 300.0, float64(result.Value), 0.01)
	})

	// C -> F
	t.Run("C to F", func(t *testing.T) {
		temp := NewTemperature(Celsius, 26.85)
		result := temp.Convert(Fahrenheit)
		assert.InDelta(t, 80.33, float64(result.Value), 0.01)
	})
}

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
