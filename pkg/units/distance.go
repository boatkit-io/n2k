package units

import "encoding/json"

// DistanceUnit is an enum for all supported distance/length unit types.
// ENUM(Meter, Foot, Mile, NauticalMile, Fathom)
// Meter is a meter
// Foot is a foot
// Mile is a mile
// NauticalMile is a nautical mile
// Fathom is a fathom
//
//go:generate go run github.com/abice/go-enum@latest --noprefix --values
type DistanceUnit int

// distanceConversions maps each DistanceUnit to its ratio relative to a common reference.
// The reference unit is Miles (Mile=1). All other entries represent "how many of this unit
// are in 1 mile".
//
// Conversion math: value_in_new_unit = value_in_old_unit * (newConv / oldConv)
//
// Examples:
//   - 1 Mile -> Meter: 1 * (1609.34 / 1) = 1609.34 meters
//   - 1 Foot -> Meter: 1 * (1609.34 / 5280) = 0.3048 meters
//   - 1 Mile -> Fathom: 1 * (880 / 1) = 880 fathoms
//
// Note on NauticalMile: The value 1.15078 means "1 mile = 1.15078 in the NauticalMile column".
// Since 1 NM = 1.15078 statute miles, this means converting 1 mile gives 1.15078 "NM units"
// in this table's math. The table encodes ratios, not physical equivalences directly.
var distanceConversions = map[DistanceUnit]float32{
	Meter:        1609.34, // 1 mile = 1609.34 meters
	Foot:         5280,    // 1 mile = 5280 feet
	Mile:         1,       // Reference unit: miles
	NauticalMile: 1.15078, // Ratio relative to miles (1 NM = 1.15078 statute miles)
	Fathom:       880,     // 1 mile = 880 fathoms (1 fathom = 6 feet)
}

// Distance is a type-safe unit structure that represents a distance/length measurement.
// It combines a numeric value with a DistanceUnit to ensure correct unit handling.
type Distance Unit[DistanceUnit]

// MarshalJSON implements a custom JSON marshaler that includes the UnitType discriminator.
// The output includes "value", "unit" (as the integer enum value), and "unitType" (as the
// UnitType enum value for Distance). This allows JSON consumers to identify this as a
// distance measurement and determine the specific unit.
func (u Distance) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Value    float32  `json:"value"`
		Unit     int      `json:"unit"`
		UnitType UnitType `json:"unitType"`
	}{
		Value:    u.Value,
		Unit:     int(u.Unit),
		UnitType: UnitTypeDistance,
	})
}

// NewDistance creates a new Distance value with the given unit and magnitude.
//
// Parameters:
//   - u: the distance unit (Meter, Foot, Mile, NauticalMile, or Fathom)
//   - value: the numeric magnitude in the specified unit
func NewDistance(u DistanceUnit, value float32) Distance {
	return Distance{
		Unit:  u,
		Value: value,
	}
}

// Convert converts this distance value to a different distance unit, returning a new Distance.
// Uses the table-based conversion system (distanceConversions) for the math.
//
// Parameters:
//   - newUnit: the target distance unit to convert to
//
// Returns a new Distance in the requested unit.
func (p Distance) Convert(newUnit DistanceUnit) Distance {
	v2 := convertTableUnit(distanceConversions, p.Value, p.Unit, newUnit)
	return NewDistance(newUnit, v2)
}

// Add adds another Distance to this one, returning a new Distance in this value's unit.
// If the other Distance is in a different unit, it is automatically converted before adding.
// The result preserves the receiver's unit.
func (p Distance) Add(o Distance) Distance {
	v2, u2 := addTableUnits(distanceConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewDistance(u2, v2)
}

// Sub subtracts another Distance from this one, returning a new Distance in this value's unit.
// If the other Distance is in a different unit, it is automatically converted before subtracting.
// The result preserves the receiver's unit.
func (p Distance) Sub(o Distance) Distance {
	v2, u2 := subTableUnits(distanceConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewDistance(u2, v2)
}

// Multiply scales this distance by a scalar factor, returning a new Distance in the same unit.
// This is useful for operations like "double the distance" or "take half the distance".
//
// Parameters:
//   - by: the scalar multiplier (e.g., 2.0 to double, 0.5 to halve)
func (p Distance) Multiply(by float32) Distance {
	return NewDistance(p.Unit, p.Value*by)
}
