package units

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewFlow verifies that the constructor correctly stores the unit and value
// for all three flow units (LitersPerHour, GallonsPerMinute, GallonsPerHour).
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

// TestFlowConvert tests the table-based flow rate conversion across all supported unit pairs.
// The conversion table uses LitersPerHour as the reference unit (LitersPerHour=1).
//
// Table values: LitersPerHour=1, GallonsPerMinute=0.00440287, GallonsPerHour=0.264172
// Formula: value * (newConv / oldConv)
//
// Note: GallonsPerMinute = GallonsPerHour / 60, so the ratio between those two is always 60x,
// which serves as a useful sanity check.
func TestFlowConvert(t *testing.T) {
	// 1 L/hr -> Gal/hr: 1 * (0.264172 / 1) = 0.264172
	t.Run("LitersPerHour to GallonsPerHour", func(t *testing.T) {
		f := NewFlow(LitersPerHour, 1)
		result := f.Convert(GallonsPerHour)
		assert.InDelta(t, 0.264172, float64(result.Value), 0.001)
		assert.Equal(t, GallonsPerHour, result.Unit)
	})

	// 1 L/hr -> Gal/min: 1 * (0.00440287 / 1) = 0.00440287
	t.Run("LitersPerHour to GallonsPerMinute", func(t *testing.T) {
		f := NewFlow(LitersPerHour, 1)
		result := f.Convert(GallonsPerMinute)
		assert.InDelta(t, 0.00440287, float64(result.Value), 0.0001)
		assert.Equal(t, GallonsPerMinute, result.Unit)
	})

	// 1 Gal/hr -> L/hr: 1 * (1 / 0.264172) = 3.785 (about 3.785 liters per gallon)
	t.Run("GallonsPerHour to LitersPerHour", func(t *testing.T) {
		f := NewFlow(GallonsPerHour, 1)
		result := f.Convert(LitersPerHour)
		assert.InDelta(t, 3.785, float64(result.Value), 0.01)
		assert.Equal(t, LitersPerHour, result.Unit)
	})

	// 1 Gal/min -> Gal/hr: 1 * (0.264172 / 0.00440287) = 60.0
	// This makes physical sense: 1 gallon per minute = 60 gallons per hour.
	t.Run("GallonsPerMinute to GallonsPerHour", func(t *testing.T) {
		f := NewFlow(GallonsPerMinute, 1)
		result := f.Convert(GallonsPerHour)
		assert.InDelta(t, 60.0, float64(result.Value), 0.1)
		assert.Equal(t, GallonsPerHour, result.Unit)
	})

	// 60 Gal/hr -> Gal/min: 60 * (0.00440287 / 0.264172) = 1.0 (reverse of above)
	t.Run("GallonsPerHour to GallonsPerMinute", func(t *testing.T) {
		f := NewFlow(GallonsPerHour, 60)
		result := f.Convert(GallonsPerMinute)
		assert.InDelta(t, 1.0, float64(result.Value), 0.01)
		assert.Equal(t, GallonsPerMinute, result.Unit)
	})

	// 1 Gal/min -> L/hr: 1 * (1 / 0.00440287) = 227.12 liters per hour
	t.Run("GallonsPerMinute to LitersPerHour", func(t *testing.T) {
		f := NewFlow(GallonsPerMinute, 1)
		result := f.Convert(LitersPerHour)
		assert.InDelta(t, 227.12, float64(result.Value), 0.1)
		assert.Equal(t, LitersPerHour, result.Unit)
	})

	// Identity conversion: same unit should return the exact same value.
	t.Run("Identity conversion", func(t *testing.T) {
		f := NewFlow(LitersPerHour, 55.5)
		result := f.Convert(LitersPerHour)
		assert.Equal(t, float32(55.5), result.Value)
		assert.Equal(t, LitersPerHour, result.Unit)
	})
}

// TestFlowAddSameUnit verifies same-unit addition produces a simple numeric sum.
func TestFlowAddSameUnit(t *testing.T) {
	f1 := NewFlow(LitersPerHour, 100)
	f2 := NewFlow(LitersPerHour, 50)
	result := f1.Add(f2)
	assert.Equal(t, float32(150), result.Value)
	assert.Equal(t, LitersPerHour, result.Unit)
}

// TestFlowAddDifferentUnits verifies cross-unit addition.
// 100 L/hr + 1 Gal/min: 1 gal/min = ~227.12 L/hr, so total = ~327.12 L/hr.
func TestFlowAddDifferentUnits(t *testing.T) {
	f1 := NewFlow(LitersPerHour, 100)
	f2 := NewFlow(GallonsPerMinute, 1)
	result := f1.Add(f2)
	assert.InDelta(t, 327.12, float64(result.Value), 0.5)
	assert.Equal(t, LitersPerHour, result.Unit)
}

// TestFlowSubSameUnit verifies same-unit subtraction produces a simple numeric difference.
func TestFlowSubSameUnit(t *testing.T) {
	f1 := NewFlow(GallonsPerHour, 100)
	f2 := NewFlow(GallonsPerHour, 30)
	result := f1.Sub(f2)
	assert.Equal(t, float32(70), result.Value)
	assert.Equal(t, GallonsPerHour, result.Unit)
}

// TestFlowSubDifferentUnits verifies cross-unit subtraction.
// 500 L/hr - 1 Gal/hr: 1 gal/hr = ~3.785 L/hr, so total = ~496.215 L/hr.
func TestFlowSubDifferentUnits(t *testing.T) {
	f1 := NewFlow(LitersPerHour, 500)
	f2 := NewFlow(GallonsPerHour, 1)
	result := f1.Sub(f2)
	assert.InDelta(t, 496.21, float64(result.Value), 0.1)
	assert.Equal(t, LitersPerHour, result.Unit)
}

// TestFlowMarshalJSON verifies that the custom JSON marshaler produces the expected
// format with value, unit (as integer enum), and unitType (as UnitTypeFlow).
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
