package canbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCanInterfaceNameForSpiDevice(t *testing.T) {
	_, err := GetCanInterfaceNameForSpiDevice("fake")
	assert.Error(t, err, "expected getCanInterfaceNameForSpiDevice() to fail")
}
