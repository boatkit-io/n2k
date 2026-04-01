package units

import "encoding/json"

// VolumeUnit is an enum for all supported volume/capacity unit types.
// ENUM(Liter, MetersCubed, Gallon)
//
//go:generate go run github.com/abice/go-enum@latest --noprefix --values
type VolumeUnit int

// volumeConversions maps each VolumeUnit to its ratio relative to a common reference.
// The reference unit is MetersCubed (MetersCubed=1). All other entries represent
// "how many of this unit are in 1 cubic meter".
//
// Conversion math: value_in_new_unit = value_in_old_unit * (newConv / oldConv)
//
// Examples:
//   - 1 m^3 -> Liter:  1 * (1000 / 1) = 1000 liters
//   - 1 m^3 -> Gallon: 1 * (264.172 / 1) = 264.172 gallons
//   - 1 Liter -> Gallon: 1 * (264.172 / 1000) = 0.264172 gallons
//   - 1 Gallon -> Liter: 1 * (1000 / 264.172) = 3.785 liters
var volumeConversions = map[VolumeUnit]float32{
	MetersCubed: 1,       // Reference unit: cubic meters
	Liter:       1000,    // 1 cubic meter = 1000 liters
	Gallon:      264.172, // 1 cubic meter = 264.172 US gallons
}

// Volume is a type-safe unit structure that represents a volume/capacity measurement.
// It combines a numeric value with a VolumeUnit to ensure correct unit handling.
type Volume Unit[VolumeUnit]

// MarshalJSON implements a custom JSON marshaler that includes the UnitType discriminator.
// The output includes "value", "unit" (as the integer enum value), and "unitType" (as the
// UnitType enum value for Volume). This allows JSON consumers to identify this as a
// volume measurement and determine the specific unit.
func (u Volume) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Value    float32  `json:"value"`
		Unit     int      `json:"unit"`
		UnitType UnitType `json:"unitType"`
	}{
		Value:    u.Value,
		Unit:     int(u.Unit),
		UnitType: UnitTypeVolume,
	})
}

// NewVolume creates a new Volume value with the given unit and magnitude.
//
// Parameters:
//   - u: the volume unit (Liter, MetersCubed, or Gallon)
//   - value: the numeric magnitude in the specified unit
func NewVolume(u VolumeUnit, value float32) Volume {
	return Volume{
		Unit:  u,
		Value: value,
	}
}

// Convert converts this volume value to a different volume unit, returning a new Volume.
// Uses the table-based conversion system (volumeConversions) for the math.
//
// Parameters:
//   - newUnit: the target volume unit to convert to
//
// Returns a new Volume in the requested unit.
func (p Volume) Convert(newUnit VolumeUnit) Volume {
	v2 := convertTableUnit(volumeConversions, p.Value, p.Unit, newUnit)
	return NewVolume(newUnit, v2)
}

// Add adds another Volume to this one, returning a new Volume in this value's unit.
// If the other Volume is in a different unit, it is automatically converted before adding.
// The result preserves the receiver's unit.
func (p Volume) Add(o Volume) Volume {
	v2, u2 := addTableUnits(volumeConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewVolume(u2, v2)
}

// Sub subtracts another Volume from this one, returning a new Volume in this value's unit.
// If the other Volume is in a different unit, it is automatically converted before subtracting.
// The result preserves the receiver's unit.
func (p Volume) Sub(o Volume) Volume {
	v2, u2 := subTableUnits(volumeConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewVolume(u2, v2)
}
