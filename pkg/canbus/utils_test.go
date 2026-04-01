package canbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetCanInterfaceNameForSpiDevice verifies that looking up a non-existent SPI device
// returns an error. This is a basic negative test -- on a real Linux system with actual
// SPI-based CAN hardware, a positive test would verify the returned interface name.
// Since unit tests typically run without CAN hardware, we only verify the error path.
func TestGetCanInterfaceNameForSpiDevice(t *testing.T) {
	_, err := GetCanInterfaceNameForSpiDevice("fake")
	assert.Error(t, err, "expected getCanInterfaceNameForSpiDevice() to fail")
}
