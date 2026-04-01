package units

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testUnit int

const (
	C testUnit = 1
	D testUnit = 2
)

var testConversions = map[testUnit]float32{
	C: 1,
	D: 5.5,
}

func TestTableConvUnit(t *testing.T) {
	v := float32(10)
	v2d := convertTableUnit(testConversions, v, C, D)
	assert.Equal(t, float32(55), v2d)
	v2d2c := convertTableUnit(testConversions, v2d, D, C)
	assert.Equal(t, v, v2d2c)
	v2, u2 := addTableUnits(testConversions, v, C, v2d2c, C)
	assert.Equal(t, float32(20), v2)
	assert.Equal(t, C, u2)
	v2pd, u2pd := addTableUnits(testConversions, v, C, v2d, D)
	assert.Equal(t, float32(20), v2pd)
	assert.Equal(t, C, u2pd)
	v2do, u2do := addTableUnits(testConversions, v2d, D, v, C)
	assert.Equal(t, float32(110), v2do)
	assert.Equal(t, D, u2do)
}
