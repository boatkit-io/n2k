package units

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testUnit is a minimal unit enum used only for testing the table-based conversion functions.
// It has two units (C and D) with a 1:5.5 ratio, making it easy to verify conversion math.
type testUnit int

const (
	C testUnit = 1 // Test unit C: the "base" unit (conversion factor = 1)
	D testUnit = 2 // Test unit D: 5.5x larger than C (1 reference unit = 5.5 D)
)

// testConversions is the conversion table for the test units.
// With C=1 and D=5.5, converting 10 C -> D gives 10 * (5.5/1) = 55.
var testConversions = map[testUnit]float32{
	C: 1,
	D: 5.5,
}

// TestTableConvUnit exercises the core table-based conversion functions (convertTableUnit,
// addTableUnits) with the simple test units above. It verifies:
//   - Converting C to D multiplies by (5.5/1) = 5.5
//   - Round-trip conversion (C -> D -> C) returns the original value
//   - Adding two values in the same unit (C + C) works correctly
//   - Adding values in different units (C + D) converts D to C before adding
//   - Adding values in different units (D + C) converts C to D before adding,
//     demonstrating that the result unit always matches the first operand
func TestTableConvUnit(t *testing.T) {
	v := float32(10)

	// Convert 10 C -> D: 10 * (5.5 / 1) = 55
	v2d := convertTableUnit(testConversions, v, C, D)
	assert.Equal(t, float32(55), v2d)

	// Round-trip: convert 55 D -> C: 55 * (1 / 5.5) = 10 (back to original)
	v2d2c := convertTableUnit(testConversions, v2d, D, C)
	assert.Equal(t, v, v2d2c)

	// Add same units: 10 C + 10 C = 20 C
	v2, u2 := addTableUnits(testConversions, v, C, v2d2c, C)
	assert.Equal(t, float32(20), v2)
	assert.Equal(t, C, u2)

	// Add different units (C + D): 10 C + 55 D. First converts 55 D to C (=10), then 10+10=20 C.
	v2pd, u2pd := addTableUnits(testConversions, v, C, v2d, D)
	assert.Equal(t, float32(20), v2pd)
	assert.Equal(t, C, u2pd)

	// Add different units (D + C): 55 D + 10 C. First converts 10 C to D (=55), then 55+55=110 D.
	// This demonstrates the result is always in the first operand's unit (D in this case).
	v2do, u2do := addTableUnits(testConversions, v2d, D, v, C)
	assert.Equal(t, float32(110), v2do)
	assert.Equal(t, D, u2do)
}
