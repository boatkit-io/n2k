package units

import "encoding/json"

// VelocityUnit is an enum for all supported velocity/speed unit types.
// ENUM(MetersPerSecond, Knots, Mph, Kph)
//
//go:generate go run github.com/abice/go-enum@latest --noprefix --values
type VelocityUnit int

// velocityConversions maps each VelocityUnit to its ratio relative to a common reference.
// The reference unit is Knots (Knots=1). All other entries represent "how many of this unit
// are equivalent to 1 knot".
//
// Conversion math: value_in_new_unit = value_in_old_unit * (newConv / oldConv)
//
// Examples:
//   - 1 Knot -> Mph: 1 * (1.15078 / 1) = 1.15078 mph
//   - 1 Knot -> Kph: 1 * (1.852 / 1) = 1.852 kph
//   - 1 Knot -> m/s: 1 * (0.514444 / 1) = 0.514444 m/s
//   - 1 m/s -> Kph:  1 * (1.852 / 0.514444) = 3.6 kph
//
// Knots are the natural reference unit for marine applications (NMEA 2000).
var velocityConversions = map[VelocityUnit]float32{
	MetersPerSecond: 0.514444444444, // 1 knot = 0.514444 m/s
	Knots:           1,              // Reference unit: knots
	Mph:             1.15078,        // 1 knot = 1.15078 mph
	Kph:             1.852,          // 1 knot = 1.852 kph
}

// Velocity is a type-safe unit structure that represents a speed/velocity measurement.
// It combines a numeric value with a VelocityUnit to ensure correct unit handling.
type Velocity Unit[VelocityUnit]

// MarshalJSON implements a custom JSON marshaler that includes the UnitType discriminator.
// The output includes "value", "unit" (as the integer enum value), and "unitType" (as the
// UnitType enum value for Velocity). This allows JSON consumers to identify this as a
// velocity measurement and determine the specific unit.
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

// NewVelocity creates a new Velocity value with the given unit and magnitude.
//
// Parameters:
//   - u: the velocity unit (MetersPerSecond, Knots, Mph, or Kph)
//   - value: the numeric magnitude in the specified unit
func NewVelocity(u VelocityUnit, value float32) Velocity {
	return Velocity{
		Unit:  u,
		Value: value,
	}
}

// Convert converts this velocity value to a different velocity unit, returning a new Velocity.
// Uses the table-based conversion system (velocityConversions) for the math.
//
// Parameters:
//   - newUnit: the target velocity unit to convert to
//
// Returns a new Velocity in the requested unit.
func (p Velocity) Convert(newUnit VelocityUnit) Velocity {
	v2 := convertTableUnit(velocityConversions, p.Value, p.Unit, newUnit)
	return NewVelocity(newUnit, v2)
}

// Add adds another Velocity to this one, returning a new Velocity in this value's unit.
// If the other Velocity is in a different unit, it is automatically converted before adding.
// The result preserves the receiver's unit.
func (p Velocity) Add(o Velocity) Velocity {
	v2, u2 := addTableUnits(velocityConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewVelocity(u2, v2)
}

// Sub subtracts another Velocity from this one, returning a new Velocity in this value's unit.
// If the other Velocity is in a different unit, it is automatically converted before subtracting.
// The result preserves the receiver's unit.
func (p Velocity) Sub(o Velocity) Velocity {
	v2, u2 := subTableUnits(velocityConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewVelocity(u2, v2)
}

// TimesTime computes a distance by multiplying this velocity by a duration in seconds.
// This implements the physics formula: distance = velocity * time.
//
// The calculation works by first converting the velocity to knots, then multiplying by
// the time in hours (seconds / 3600) to get a distance in nautical miles. This is the
// natural unit pairing for marine applications: knots * hours = nautical miles.
//
// Parameters:
//   - seconds: the duration in seconds to multiply by
//
// Returns a Distance in NauticalMile units.
//
// Example: 10 knots * 1800 seconds (30 minutes) = 5 nautical miles
func (v Velocity) TimesTime(seconds float32) Distance {
	// Convert to knots first, since knots are defined as nautical miles per hour.
	vKt := v.Convert(Knots)
	// Divide seconds by 3600 to get hours, then multiply: knots * hours = nautical miles.
	return NewDistance(NauticalMile, vKt.Value*(seconds/3600.0))
}
