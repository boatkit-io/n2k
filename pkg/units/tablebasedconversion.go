package units

import (
	"fmt"
)

// addTableUnits is a helper function to add two table-based conversion units
func addTableUnits[U ~int](table map[U]float32, v1 float32, u1 U, v2 float32, u2 U) (float32, U) {
	otherValConv := convertTableUnit(table, v2, u2, u1)
	return v1 + otherValConv, u1
}

// subTableUnits is a helper function to subtract two table-based conversion units
func subTableUnits[U ~int](table map[U]float32, v1 float32, u1 U, v2 float32, u2 U) (float32, U) {
	otherValConv := convertTableUnit(table, v2, u2, u1)
	return v1 - otherValConv, u1
}

// convertTableUnit is a helper function to convert a table-based conversion unit into another
func convertTableUnit[U ~int](conversionTable map[U]float32, value float32, oldUnit U, newUnit U) float32 {
	// Shortcut (ez optimization)
	if oldUnit == newUnit {
		return value
	}

	curConv, curExists := conversionTable[oldUnit]
	if !curExists {
		panic(fmt.Sprintf("No unit conversion for old unit %+v", oldUnit))
	}
	newConv, newExists := conversionTable[newUnit]
	if !newExists {
		panic(fmt.Sprintf("No unit conversion for new unit %+v", newUnit))
	}
	return value * (newConv / curConv)
}
