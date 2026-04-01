package units

import "encoding/json"

// VelocityUnit is an enum for all velocity unit types
// ENUM(MetersPerSecond, Knots, Mph, Kph)
//
//go:generate go run github.com/abice/go-enum@latest --noprefix --values
type VelocityUnit int

// velocityConversions is a helper for doing unit conversions on VelocityUnits
var velocityConversions = map[VelocityUnit]float32{
	MetersPerSecond: 0.514444444444,
	Knots:           1,
	Mph:             1.15078,
	Kph:             1.852,
}

// Velocity is a generic Unit structure that represents velocities (speeds)
type Velocity Unit[VelocityUnit]

// MarshalJSON is a custom marshaler for the unit type to add the UnitType string
func (u Velocity) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Value    float32  `json:"value"`
		Unit     int      `json:"unit"`
		UnitType UnitType `json:"unitType"`
	}{
		Value:    u.Value,
		Unit:     int(u.Unit),
		UnitType: UnitTypeVelocity,
	})
}

// NewVelocity creates a velocity unit of a given type and value
func NewVelocity(u VelocityUnit, value float32) Velocity {
	return Velocity{
		Unit:  u,
		Value: value,
	}
}

// Convert converts the unit+value into a new unit type, returning a new unit value of the requested type.
func (p Velocity) Convert(newUnit VelocityUnit) Velocity {
	v2 := convertTableUnit(velocityConversions, p.Value, p.Unit, newUnit)
	return NewVelocity(newUnit, v2)
}

// Add will add another unit to this one, returning a new unit with the added values
func (p Velocity) Add(o Velocity) Velocity {
	v2, u2 := addTableUnits(velocityConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewVelocity(u2, v2)
}

// Sub will subtract another unit from this one, returning a new unit with the subtracted values
func (p Velocity) Sub(o Velocity) Velocity {
	v2, u2 := subTableUnits(velocityConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewVelocity(u2, v2)
}

// TimesTime calculates a distance from this Velocity unit by multiplying it by an amount of time
func (v Velocity) TimesTime(seconds float32) Distance {
	vKt := v.Convert(Knots)
	return NewDistance(NauticalMile, vKt.Value*(seconds/3600.0))
}
