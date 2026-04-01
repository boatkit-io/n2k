package units

import "encoding/json"

// FlowUnit is an enum for all supported flow rate unit types.
// ENUM(LitersPerHour, GallonsPerMinute, GallonsPerHour)
// LitersPerHour is L/hr
// GallonsPerMinute is Gal/min
// GallonsPerHour is Gal/hr
//
//go:generate go run github.com/abice/go-enum@latest --noprefix --values
type FlowUnit int

// flowConversions maps each FlowUnit to its ratio relative to a common reference.
// The reference unit is LitersPerHour (LitersPerHour=1). All other entries represent
// "how many of this unit are equivalent to 1 liter per hour".
//
// Conversion math: value_in_new_unit = value_in_old_unit * (newConv / oldConv)
//
// Examples:
//   - 1 L/hr -> Gal/hr:  1 * (0.264172 / 1) = 0.264172 gal/hr
//   - 1 L/hr -> Gal/min: 1 * (0.00440287 / 1) = 0.00440287 gal/min
//   - 1 Gal/min -> Gal/hr: 1 * (0.264172 / 0.00440287) = 60.0 gal/hr
//   - 1 Gal/min -> L/hr: 1 * (1 / 0.00440287) = 227.12 L/hr
//
// Note: GallonsPerMinute's conversion factor (0.00440287) equals GallonsPerHour (0.264172) / 60,
// which makes sense since there are 60 minutes in an hour.
var flowConversions = map[FlowUnit]float32{
	LitersPerHour:    1,          // Reference unit: liters per hour
	GallonsPerMinute: 0.00440287, // 1 L/hr = 0.00440287 gal/min
	GallonsPerHour:   0.264172,   // 1 L/hr = 0.264172 gal/hr
}

// Flow is a type-safe unit structure that represents a flow rate measurement (volume over time).
// It combines a numeric value with a FlowUnit to ensure correct unit handling.
// This is commonly used for fuel flow rates in marine applications.
type Flow Unit[FlowUnit]

// MarshalJSON implements a custom JSON marshaler that includes the UnitType discriminator.
// The output includes "value", "unit" (as the integer enum value), and "unitType" (as the
// UnitType enum value for Flow). This allows JSON consumers to identify this as a
// flow rate measurement and determine the specific unit.
func (u Flow) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Value    float32  `json:"value"`
		Unit     int      `json:"unit"`
		UnitType UnitType `json:"unitType"`
	}{
		Value:    u.Value,
		Unit:     int(u.Unit),
		UnitType: UnitTypeFlow,
	})
}

// NewFlow creates a new Flow value with the given unit and magnitude.
//
// Parameters:
//   - u: the flow unit (LitersPerHour, GallonsPerMinute, or GallonsPerHour)
//   - value: the numeric magnitude in the specified unit
func NewFlow(u FlowUnit, value float32) Flow {
	return Flow{
		Unit:  u,
		Value: value,
	}
}

// Convert converts this flow value to a different flow unit, returning a new Flow.
// Uses the table-based conversion system (flowConversions) for the math.
//
// Parameters:
//   - newUnit: the target flow unit to convert to
//
// Returns a new Flow in the requested unit.
func (p Flow) Convert(newUnit FlowUnit) Flow {
	v2 := convertTableUnit(flowConversions, p.Value, p.Unit, newUnit)
	return NewFlow(newUnit, v2)
}

// Add adds another Flow to this one, returning a new Flow in this value's unit.
// If the other Flow is in a different unit, it is automatically converted before adding.
// The result preserves the receiver's unit.
func (p Flow) Add(o Flow) Flow {
	v2, u2 := addTableUnits(flowConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewFlow(u2, v2)
}

// Sub subtracts another Flow from this one, returning a new Flow in this value's unit.
// If the other Flow is in a different unit, it is automatically converted before subtracting.
// The result preserves the receiver's unit.
func (p Flow) Sub(o Flow) Flow {
	v2, u2 := subTableUnits(flowConversions, p.Value, p.Unit, o.Value, o.Unit)
	return NewFlow(u2, v2)
}
