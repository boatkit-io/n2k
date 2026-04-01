package units

import "encoding/json"

// PressureUnit is an enum for all supported pressure unit types.
// ENUM(Pa, Psi, Hpa)
// Psi is PSI (pounds per square inch)
// Hpa is HectoPascals (100 Pascals)
// Pa is Pascals
//
//go:generate go run github.com/abice/go-enum@latest --noprefix --values
type PressureUnit int

// pressureConversions maps each PressureUnit to its ratio relative to a common reference.
// The reference unit is effectively HectoPascals (Hpa=1). All other units are expressed as
// "how many of this unit equal 1 HectoPascal".
//
// Conversion math: value_in_new_unit = value_in_old_unit * (newConv / oldConv)
//
// Examples:
//   - 1 Hpa -> Pa: 1 * (100 / 1) = 100 Pa  (1 hectopascal = 100 pascals)
//   - 1 Hpa -> Psi: 1 * (0.0145038 / 1) = 0.0145038 Psi
//   - 1 Psi -> Hpa: 1 * (1 / 0.0145038) = 68.95 Hpa
var pressureConversions = map[PressureUnit]float32{
	Psi: 0.0145038, // 1 hectopascal = 0.0145038 PSI
	Hpa: 1,         // Reference unit: hectopascals
	Pa:  100,       // 1 hectopascal = 100 pascals
}

// Pressure is a type-safe unit structure that represents a pressure measurement.
// It combines a numeric value with a PressureUnit to ensure correct unit handling.
type Pressure Unit[PressureUnit]

// MarshalJSON implements a custom JSON marshaler that includes the UnitType discriminator.
// The output includes "value", "unit" (as the integer enum value), and "unitType" (as the
// UnitType enum value for Pressure). This allows JSON consumers to identify this as a
// pressure measurement and determine the specific unit.
func (u Pressure) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Value    float32  `json:"value"`
		Unit     int      `json:"unit"`
		UnitType UnitType `json:"unitType"`
	}{
		Value:    u.Value,
		Unit:     int(u.Unit),
		UnitType: UnitTypePressure,
	})
}

// NewPressure creates a new Pressure value with the given unit and magnitude.
//
// Parameters:
//   - u: the pressure unit (Pa, Psi, or Hpa)
//   - value: the numeric magnitude in the specified unit
func NewPressure(u PressureUnit, value float32) Pressure {
	return Pressure{
		Unit:  u,
		Value: value,
	}
}

// Convert converts this pressure value to a different pressure unit, returning a new Pressure.
// Uses the table-based conversion system (pressureConversions) for the math.
//
// Parameters:
//   - newUnit: the target pressure unit to convert to
//
// Returns a new Pressure in the requested unit.
func (p Pressure) Convert(newUnit PressureUnit) Pressure {
	v2 := convertTableUnit(pressureConversions, p.Value, p.Unit, newUnit)
	return NewPressure(newUnit, v2)
}

// Add adds another Pressure to this one, returning a new Pressure in this value's unit.
// If the other Pressure is in a different unit, it is automatically converted before adding.
// The result preserves the receiver's unit.
func (p Pressure) Add(o Pressure) Pressure {
	v2, u2 := addTableUnits(pressureConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewPressure(u2, v2)
}

// Sub subtracts another Pressure from this one, returning a new Pressure in this value's unit.
// If the other Pressure is in a different unit, it is automatically converted before subtracting.
// The result preserves the receiver's unit.
func (p Pressure) Sub(o Pressure) Pressure {
	v2, u2 := subTableUnits(pressureConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewPressure(u2, v2)
}
