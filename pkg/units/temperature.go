package units

import (
	"encoding/json"
	"fmt"
)

// TemperatureUnit is an enum for all supported temperature unit types.
// ENUM(Kelvin, Fahrenheit, Celsius)
//
//go:generate go run github.com/abice/go-enum@latest --noprefix --values
type TemperatureUnit int

// Temperature is a type-safe unit structure that represents a temperature measurement.
// Unlike other unit types in this package, Temperature does NOT use the table-based conversion
// system because temperature conversions involve offsets (not just ratios). For example,
// Celsius = Kelvin - 273.15 and Fahrenheit = Kelvin * 9/5 - 459.67.
// These cannot be expressed as simple ratio tables.
type Temperature Unit[TemperatureUnit]

// MarshalJSON implements a custom JSON marshaler that includes the UnitType discriminator.
// The output includes "value", "unit" (as the integer enum value), and "unitType" (as the
// UnitType enum value for Temperature). This allows JSON consumers to identify this as a
// temperature measurement and determine the specific unit.
func (u Temperature) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Value    float32  `json:"value"`
		Unit     int      `json:"unit"`
		UnitType UnitType `json:"unitType"`
	}{
		Value:    u.Value,
		Unit:     int(u.Unit),
		UnitType: UnitTypeTemperature,
	})
}

// NewTemperature creates a new Temperature value with the given unit and magnitude.
//
// Parameters:
//   - u: the temperature unit (Kelvin, Fahrenheit, or Celsius)
//   - value: the numeric magnitude in the specified unit
func NewTemperature(u TemperatureUnit, value float32) Temperature {
	return Temperature{
		Unit:  u,
		Value: value,
	}
}

// Convert converts this temperature value to a different temperature unit, returning a new Temperature.
//
// Unlike other unit types that use table-based conversion (simple ratio multiplication), temperature
// conversion requires a two-step process using Kelvin as the intermediate/base unit:
//   1. Convert the current value to Kelvin (the SI base unit for temperature)
//   2. Convert from Kelvin to the target unit
//
// Conversion formulas:
//   - Celsius to Kelvin:    K = C + 273.15
//   - Fahrenheit to Kelvin: K = (F + 459.67) * 5/9
//   - Kelvin to Celsius:    C = K - 273.15
//   - Kelvin to Fahrenheit: F = K * 9/5 - 459.67
//
// Parameters:
//   - newUnit: the target temperature unit to convert to
//
// Returns a new Temperature in the requested unit.
// Panics if either the current or target unit is an unknown TemperatureUnit value.
func (p Temperature) Convert(newUnit TemperatureUnit) Temperature {
	// Shortcut: if already in the target unit, return as-is to avoid floating-point precision loss.
	if p.Unit == newUnit {
		return p
	}

	// Step 1: Convert the current value to Kelvin (the universal intermediate).
	var inKelvin float32
	switch p.Unit {
	case Kelvin:
		inKelvin = p.Value
	case Fahrenheit:
		// Fahrenheit to Kelvin: add the Fahrenheit absolute zero offset (459.67),
		// then scale by 5/9 to convert the degree size from Fahrenheit to Kelvin.
		inKelvin = (p.Value + 459.67) * (5.0 / 9.0)
	case Celsius:
		// Celsius to Kelvin: simply add the offset (273.15) since both scales
		// have the same degree size, just different zero points.
		inKelvin = p.Value + 273.15
	default:
		panic(fmt.Sprintf("Unknown old temperature unit %+v", p.Unit))
	}

	// Step 2: Convert from Kelvin to the target unit.
	switch newUnit {
	case Kelvin:
		return NewTemperature(newUnit, inKelvin)
	case Fahrenheit:
		// Kelvin to Fahrenheit: scale by 9/5 (convert degree size), then subtract
		// the absolute zero offset in Fahrenheit.
		return NewTemperature(newUnit, inKelvin*(9.0/5.0)-459.67)
	case Celsius:
		// Kelvin to Celsius: subtract the offset (273.15) since both scales have
		// the same degree size.
		return NewTemperature(newUnit, inKelvin-273.15)
	default:
		panic(fmt.Sprintf("Unknown new temperature unit %+v", newUnit))
	}
}
