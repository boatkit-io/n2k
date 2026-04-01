package units

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPressure verifies the Pressure type's Add method with both same-unit and cross-unit
// addition. It checks:
//   - Same unit addition: 5 Hpa + 7 Hpa = 12 Hpa
//   - Cross-unit addition (Hpa + Pa): 5 Hpa + 1 Pa = 5.01 Hpa (since 1 Pa = 0.01 Hpa)
//   - Cross-unit addition (Pa + Hpa): 1 Pa + 5 Hpa = 501 Pa (since 5 Hpa = 500 Pa)
//     This verifies that the result unit matches the receiver (first operand), not the argument.
func TestPressure(t *testing.T) {
	// Same-unit addition: 5 Hpa + 7 Hpa = 12 Hpa
	p := NewPressure(Hpa, 5)
	p2 := NewPressure(Hpa, 7)
	p3 := p.Add(p2)
	assert.Equal(t, float32(12), p3.Value)
	assert.Equal(t, Hpa, p3.Unit)

	// Cross-unit addition (result in Hpa): 5 Hpa + 1 Pa.
	// 1 Pa converted to Hpa: 1 * (1/100) = 0.01 Hpa.
	// Total: 5 + 0.01 = 5.01 Hpa.
	p4 := NewPressure(Pa, 1)
	p5 := p.Add(p4)
	assert.Equal(t, float32(5.01), p5.Value)
	assert.Equal(t, Hpa, p5.Unit)

	// Cross-unit addition (result in Pa): 1 Pa + 5 Hpa.
	// 5 Hpa converted to Pa: 5 * (100/1) = 500 Pa.
	// Total: 1 + 500 = 501 Pa.
	// The result is in Pa because Pa is the receiver's unit.
	p6 := p4.Add(p)
	assert.Equal(t, float32(501), p6.Value)
	assert.Equal(t, Pa, p6.Unit)
}
