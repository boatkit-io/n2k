package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestVolumeConvert(t *testing.T) {
	// Table: MetersCubed=1, Liter=1000, Gallon=264.172

	t.Run("MetersCubed to Liter", func(t *testing.T) {
		v := NewVolume(MetersCubed, 1)
		result := v.Convert(Liter)
		// 1 * (1000 / 1) = 1000
		assert.InDelta(t, 1000.0, float64(result.Value), 0.1)
		assert.Equal(t, Liter, result.Unit)
	})

	t.Run("MetersCubed to Gallon", func(t *testing.T) {
		v := NewVolume(MetersCubed, 1)
		result := v.Convert(Gallon)
		// 1 * (264.172 / 1) = 264.172
		assert.InDelta(t, 264.172, float64(result.Value), 0.1)
		assert.Equal(t, Gallon, result.Unit)
	})

	t.Run("Liter to MetersCubed", func(t *testing.T) {
		v := NewVolume(Liter, 1000)
		result := v.Convert(MetersCubed)
		// 1000 * (1 / 1000) = 1
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	})

	t.Run("Liter to Gallon", func(t *testing.T) {
		v := NewVolume(Liter, 1)
		result := v.Convert(Gallon)
		// 1 * (264.172 / 1000) = 0.264172
		assert.InDelta(t, 0.264172, float64(result.Value), 0.001)
	})

	t.Run("Gallon to Liter", func(t *testing.T) {
		v := NewVolume(Gallon, 1)
		result := v.Convert(Liter)
		// 1 * (1000 / 264.172) = 3.7854
		assert.InDelta(t, 3.785, float64(result.Value), 0.01)
	})

	t.Run("Gallon to MetersCubed", func(t *testing.T) {
		v := NewVolume(Gallon, 264.172)
		result := v.Convert(MetersCubed)
		// 264.172 * (1 / 264.172) = 1
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
	})

	t.Run("Identity conversion", func(t *testing.T) {
		v := NewVolume(Liter, 99.9)
		result := v.Convert(Liter)
		assert.Equal(t, float32(99.9), result.Value)
		assert.Equal(t, Liter, result.Unit)
	})
}

func TestVolumeAddSameUnit(t *testing.T) {
	v1 := NewVolume(Liter, 50)
	v2 := NewVolume(Liter, 30)
	result := v1.Add(v2)
	assert.Equal(t, float32(80), result.Value)
	assert.Equal(t, Liter, result.Unit)
}

func TestVolumeAddDifferentUnits(t *testing.T) {
	// 500 liters + 0.5 m^3 = 500 + 500 = 1000 liters
	v1 := NewVolume(Liter, 500)
	v2 := NewVolume(MetersCubed, 0.5)
	result := v1.Add(v2)
	assert.InDelta(t, 1000.0, float64(result.Value), 0.1)
	assert.Equal(t, Liter, result.Unit)
}

func TestVolumeSubSameUnit(t *testing.T) {
	v1 := NewVolume(Gallon, 100)
	v2 := NewVolume(Gallon, 40)
	result := v1.Sub(v2)
	assert.Equal(t, float32(60), result.Value)
	assert.Equal(t, Gallon, result.Unit)
}

func TestVolumeSubDifferentUnits(t *testing.T) {
	// 1 m^3 - 500 liters = 1 - 0.5 = 0.5 m^3
	v1 := NewVolume(MetersCubed, 1)
	v2 := NewVolume(Liter, 500)
	result := v1.Sub(v2)
	assert.InDelta(t, 0.5, float64(result.Value), 0.01)
	assert.Equal(t, MetersCubed, result.Unit)
}

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
