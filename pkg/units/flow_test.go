package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFlow(t *testing.T) {
	f := NewFlow(LitersPerHour, 100)
	assert.Equal(t, LitersPerHour, f.Unit)
	assert.Equal(t, float32(100), f.Value)

	f2 := NewFlow(GallonsPerMinute, 5)
	assert.Equal(t, GallonsPerMinute, f2.Unit)
	assert.Equal(t, float32(5), f2.Value)

	f3 := NewFlow(GallonsPerHour, 20)
	assert.Equal(t, GallonsPerHour, f3.Unit)
	assert.Equal(t, float32(20), f3.Value)
}

func TestFlowConvert(t *testing.T) {
	// Table: LitersPerHour=1, GallonsPerMinute=0.00440287, GallonsPerHour=0.264172

	t.Run("LitersPerHour to GallonsPerHour", func(t *testing.T) {
		f := NewFlow(LitersPerHour, 1)
		result := f.Convert(GallonsPerHour)
		// 1 * (0.264172 / 1) = 0.264172
		assert.InDelta(t, 0.264172, float64(result.Value), 0.001)
		assert.Equal(t, GallonsPerHour, result.Unit)
	})

	t.Run("LitersPerHour to GallonsPerMinute", func(t *testing.T) {
		f := NewFlow(LitersPerHour, 1)
		result := f.Convert(GallonsPerMinute)
		// 1 * (0.00440287 / 1) = 0.00440287
		assert.InDelta(t, 0.00440287, float64(result.Value), 0.0001)
		assert.Equal(t, GallonsPerMinute, result.Unit)
	})

	t.Run("GallonsPerHour to LitersPerHour", func(t *testing.T) {
		f := NewFlow(GallonsPerHour, 1)
		result := f.Convert(LitersPerHour)
		// 1 * (1 / 0.264172) = 3.7854
		assert.InDelta(t, 3.785, float64(result.Value), 0.01)
		assert.Equal(t, LitersPerHour, result.Unit)
	})

	t.Run("GallonsPerMinute to GallonsPerHour", func(t *testing.T) {
		f := NewFlow(GallonsPerMinute, 1)
		result := f.Convert(GallonsPerHour)
		// 1 * (0.264172 / 0.00440287) = 60.0
		assert.InDelta(t, 60.0, float64(result.Value), 0.1)
		assert.Equal(t, GallonsPerHour, result.Unit)
	})

	t.Run("GallonsPerHour to GallonsPerMinute", func(t *testing.T) {
		f := NewFlow(GallonsPerHour, 60)
		result := f.Convert(GallonsPerMinute)
		// 60 * (0.00440287 / 0.264172) = 1.0
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
		assert.Equal(t, GallonsPerMinute, result.Unit)
	})

	t.Run("GallonsPerMinute to LitersPerHour", func(t *testing.T) {
		f := NewFlow(GallonsPerMinute, 1)
		result := f.Convert(LitersPerHour)
		// 1 * (1 / 0.00440287) = 227.124
		assert.InDelta(t, 227.12, float64(result.Value), 0.1)
		assert.Equal(t, LitersPerHour, result.Unit)
	})

	t.Run("Identity conversion", func(t *testing.T) {
		f := NewFlow(LitersPerHour, 55.5)
		result := f.Convert(LitersPerHour)
		assert.Equal(t, float32(55.5), result.Value)
		assert.Equal(t, LitersPerHour, result.Unit)
	})
}

func TestFlowAddSameUnit(t *testing.T) {
	f1 := NewFlow(LitersPerHour, 100)
	f2 := NewFlow(LitersPerHour, 50)
	result := f1.Add(f2)
	assert.Equal(t, float32(150), result.Value)
	assert.Equal(t, LitersPerHour, result.Unit)
}

func TestFlowAddDifferentUnits(t *testing.T) {
	// 100 L/hr + 1 gal/min
	// 1 gal/min in L/hr = 1 * (1 / 0.00440287) = 227.12
	// total = 100 + 227.12 = 327.12 L/hr
	f1 := NewFlow(LitersPerHour, 100)
	f2 := NewFlow(GallonsPerMinute, 1)
	result := f1.Add(f2)
	assert.InDelta(t, 327.12, float64(result.Value), 0.5)
	assert.Equal(t, LitersPerHour, result.Unit)
}

func TestFlowSubSameUnit(t *testing.T) {
	f1 := NewFlow(GallonsPerHour, 100)
	f2 := NewFlow(GallonsPerHour, 30)
	result := f1.Sub(f2)
	assert.Equal(t, float32(70), result.Value)
	assert.Equal(t, GallonsPerHour, result.Unit)
}

func TestFlowSubDifferentUnits(t *testing.T) {
	// 500 L/hr - 1 gal/hr
	// 1 gal/hr in L/hr = 1 * (1 / 0.264172) = 3.785
	// total = 500 - 3.785 = 496.215
	f1 := NewFlow(LitersPerHour, 500)
	f2 := NewFlow(GallonsPerHour, 1)
	result := f1.Sub(f2)
	assert.InDelta(t, 496.21, float64(result.Value), 0.1)
	assert.Equal(t, LitersPerHour, result.Unit)
}

func TestFlowMarshalJSON(t *testing.T) {
	f := NewFlow(LitersPerHour, 100)
	data, err := json.Marshal(f)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.InDelta(t, 100.0, result["value"].(float64), 0.01)
	assert.Equal(t, float64(LitersPerHour), result["unit"].(float64))
	assert.Equal(t, float64(UnitTypeFlow), result["unitType"].(float64))
}
