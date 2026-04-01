package units

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPressure(t *testing.T) {
	p := NewPressure(Hpa, 5)
	p2 := NewPressure(Hpa, 7)
	p3 := p.Add(p2)
	assert.Equal(t, float32(12), p3.Value)
	assert.Equal(t, Hpa, p3.Unit)
	p4 := NewPressure(Pa, 1)
	p5 := p.Add(p4)
	assert.Equal(t, float32(5.01), p5.Value)
	assert.Equal(t, Hpa, p5.Unit)
	p6 := p4.Add(p)
	assert.Equal(t, float32(501), p6.Value)
	assert.Equal(t, Pa, p6.Unit)
}
