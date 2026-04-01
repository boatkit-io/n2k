// Package units has helpers for conversion between different units (meters into feet, etc.), with specific structs
// for different types of units (distance, volume, velocity, etc.)  These units also have to interact cleanly with
// units transferred from boatweb.
package units

// UnitType is an enum for all unit types
// ENUM(Distance,Flow,Pressure,Temperature,Velocity,Volume)
//
//go:generate go run github.com/abice/go-enum@latest --values
type UnitType int

// Unit is a base type for all other unit structs
type Unit[T ~int] struct {
	Value float32
	Unit  T
}
