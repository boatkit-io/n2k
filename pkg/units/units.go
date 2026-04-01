// Package units provides type-safe unit conversion for physical quantities commonly encountered
// in marine (NMEA 2000) and general-purpose sensor systems.
//
// Architecture overview:
//
// Each physical quantity (distance, velocity, pressure, etc.) is represented by its own struct type
// (e.g., Distance, Velocity, Pressure) that wraps a numeric value and a unit enum. This ensures
// type safety at compile time -- you cannot accidentally add a Distance to a Velocity.
//
// Most unit types use a "table-based conversion" approach (see tablebasedconversion.go) where
// each unit has a numeric ratio relative to a common reference unit. Conversion between any two
// units is done by multiplying: value * (newConv / oldConv). This works for all linear unit
// relationships. Temperature is the notable exception, as Fahrenheit/Celsius conversions involve
// offsets (not just ratios), so Temperature uses custom conversion logic.
//
// Each unit type provides:
//   - A constructor (e.g., NewDistance)
//   - Convert() to change units
//   - Add() and Sub() for arithmetic that handles mixed units
//   - MarshalJSON() for serialization with unit type metadata
//
// The UnitType enum is used in JSON serialization to identify which physical quantity a value
// represents, enabling generic handling in frontends like boatweb.
//
// These units also have to interact cleanly with units transferred from boatweb.
package units

// UnitType is an enum that identifies the category of physical quantity (distance, flow, etc.).
// It is included in JSON output so consumers can determine what kind of unit a value represents
// without needing to know the specific unit variant.
//
// ENUM(Distance,Flow,Pressure,Temperature,Velocity,Volume)
//
//go:generate go run github.com/abice/go-enum@latest --values
type UnitType int

// Unit is the generic base type that all specific unit structs (Distance, Velocity, etc.) are
// built from. It pairs a numeric value with a unit enum.
//
// The type parameter T is constrained to ~int, which means it must be an int-based enum type
// (like DistanceUnit, VelocityUnit, etc.). This enables compile-time type safety: each
// physical quantity uses its own enum type, preventing accidental mixing.
type Unit[T ~int] struct {
	// Value is the numeric magnitude of the measurement in whatever unit Unit specifies.
	Value float32

	// Unit identifies which unit the Value is expressed in (e.g., Meter, Foot, Knots, etc.).
	// The concrete type depends on the physical quantity (DistanceUnit, VelocityUnit, etc.).
	Unit T
}
