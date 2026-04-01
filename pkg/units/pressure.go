package units

import "encoding/json"

// PressureUnit is an enum for all pressure unit types
// ENUM(Pa, Psi, Hpa)
// Psi is PSI (pounds per square inch)
// Hpa is HectoPascals (100 Pascals)
// Pa is Pascals
//
//go:generate go run github.com/abice/go-enum@latest --noprefix --values
type PressureUnit int

// pressureConversions is a helper for doing unit conversions on PressureUnits
var pressureConversions = map[PressureUnit]float32{
	Psi: 0.0145038,
	Hpa: 1,
	Pa:  100,
}

// Pressure is a generic Unit structure that represents pressures
type Pressure Unit[PressureUnit]

// MarshalJSON is a custom marshaler for the unit type to add the UnitType string
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

// NewPressure creates a pressure unit of a given type and value
func NewPressure(u PressureUnit, value float32) Pressure {
	return Pressure{
		Unit:  u,
		Value: value,
	}
}

// Convert converts the unit+value into a new unit type, returning a new unit value of the requested type.
func (p Pressure) Convert(newUnit PressureUnit) Pressure {
	v2 := convertTableUnit(pressureConversions, p.Value, p.Unit, newUnit)
	return NewPressure(newUnit, v2)
}

// Add will add another unit to this one, returning a new unit with the added values
func (p Pressure) Add(o Pressure) Pressure {
	v2, u2 := addTableUnits(pressureConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewPressure(u2, v2)
}

// Sub will subtract another unit from this one, returning a new unit with the subtracted values
func (p Pressure) Sub(o Pressure) Pressure {
	v2, u2 := subTableUnits(pressureConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewPressure(u2, v2)
}
