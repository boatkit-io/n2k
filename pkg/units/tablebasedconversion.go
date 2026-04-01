package units

import (
	"fmt"
)

// Table-Based Conversion System
//
// Most unit types in this package use a ratio-based conversion table. Each table maps unit enum
// values to a numeric ratio relative to an implicit "reference unit" (the unit whose table value
// represents 1x of the reference quantity).
//
// For example, the distance conversion table uses "miles" as the reference:
//   Meter=1609.34  (1 mile = 1609.34 meters)
//   Foot=5280      (1 mile = 5280 feet)
//   Mile=1         (1 mile = 1 mile)
//
// To convert between any two units, we use: value * (newConv / oldConv)
//
// Example: Convert 1 mile to meters:
//   1 * (1609.34 / 1) = 1609.34 meters
//
// Example: Convert 1 meter to feet:
//   1 * (5280 / 1609.34) = 3.2808 feet
//
// This approach is elegant because:
//   1. Adding a new unit only requires adding one table entry (not N^2 pairwise conversions)
//   2. The conversion formula is the same for all unit pairs
//   3. It naturally handles chains: A->B->C collapses to a single multiply+divide
//
// IMPORTANT: This approach only works for linear unit relationships where unit_A = k * unit_B.
// Temperature (Fahrenheit/Celsius) has offsets and requires custom conversion logic.

// addTableUnits adds two values that may be in different units, returning the result in the
// first value's unit. The second value is automatically converted to match the first value's
// unit before adding.
//
// Parameters:
//   - table: the conversion ratio table for this unit type
//   - v1: the first value's magnitude
//   - u1: the first value's unit (the result will be in this unit)
//   - v2: the second value's magnitude
//   - u2: the second value's unit (will be converted to u1 before adding)
//
// Returns the sum and the result unit (always u1).
func addTableUnits[U ~int](table map[U]float32, v1 float32, u1 U, v2 float32, u2 U) (float32, U) {
	// Convert the second operand into the first operand's unit, then add.
	otherValConv := convertTableUnit(table, v2, u2, u1)
	return v1 + otherValConv, u1
}

// subTableUnits subtracts two values that may be in different units, returning the result in the
// first value's unit. The second value is automatically converted to match the first value's
// unit before subtracting.
//
// Parameters:
//   - table: the conversion ratio table for this unit type
//   - v1: the first value's magnitude (minuend)
//   - u1: the first value's unit (the result will be in this unit)
//   - v2: the second value's magnitude (subtrahend)
//   - u2: the second value's unit (will be converted to u1 before subtracting)
//
// Returns the difference and the result unit (always u1).
func subTableUnits[U ~int](table map[U]float32, v1 float32, u1 U, v2 float32, u2 U) (float32, U) {
	// Convert the second operand into the first operand's unit, then subtract.
	otherValConv := convertTableUnit(table, v2, u2, u1)
	return v1 - otherValConv, u1
}

// convertTableUnit converts a value from one unit to another using the ratio-based conversion table.
//
// The conversion formula is: value * (newConv / oldConv)
//
// This works because each table entry represents "how many of this unit equal 1 reference unit".
// Dividing newConv by oldConv gives the ratio between the two units, and multiplying by the
// value scales it accordingly.
//
// Parameters:
//   - conversionTable: maps each unit enum to its ratio relative to the reference unit
//   - value: the numeric magnitude to convert
//   - oldUnit: the unit that value is currently expressed in
//   - newUnit: the desired target unit
//
// Returns the converted value in the new unit.
// Panics if either oldUnit or newUnit is not found in the conversion table (indicates a bug
// in the unit definition, not a runtime data error).
func convertTableUnit[U ~int](conversionTable map[U]float32, value float32, oldUnit U, newUnit U) float32 {
	// Shortcut: if converting to the same unit, no math needed. This avoids unnecessary
	// floating-point operations and potential precision loss.
	if oldUnit == newUnit {
		return value
	}

	// Look up the conversion ratios for both the old and new units.
	// Both must exist in the table -- a missing entry means the developer forgot to add it.
	curConv, curExists := conversionTable[oldUnit]
	if !curExists {
		panic(fmt.Sprintf("No unit conversion for old unit %+v", oldUnit))
	}
	newConv, newExists := conversionTable[newUnit]
	if !newExists {
		panic(fmt.Sprintf("No unit conversion for new unit %+v", newUnit))
	}

	// Apply the conversion: value * (newConv / curConv)
	// This is equivalent to: (value / curConv) * newConv, which conceptually converts to the
	// reference unit first, then to the target unit.
	return value * (newConv / curConv)
}
