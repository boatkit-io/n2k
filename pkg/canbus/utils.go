package canbus

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetCanInterfaceNameForSpiDevice returns the can interface name (i.e. "can0" or "can1") for
// a given SPI device (i.e. "spi0.0").
//
// On Linux, SPI-based CAN controllers (such as the MCP2515) register themselves under
// /sys/bus/spi/devices/<spiName>/net/. This directory contains a single subdirectory
// whose name is the corresponding CAN network interface (e.g., "can0").
//
// This function reads that directory to discover the interface name dynamically,
// which is useful because the kernel may assign different canN numbers depending on
// probe order, device tree overlays, or hot-plug timing.
//
// Parameters:
//   - spiName: the SPI device identifier, e.g. "spi0.0"
//
// Returns:
//   - The CAN interface name (e.g., "can0"), or an error if the sysfs path does not
//     exist or contains an unexpected number of entries (should always be exactly one).
func GetCanInterfaceNameForSpiDevice(spiName string) (string, error) {
	// Build the sysfs path: /sys/bus/spi/devices/<spiName>/net/
	fp := filepath.Join(string(os.PathSeparator), "sys", "bus", "spi", "devices", spiName, "net")

	files, err := os.ReadDir(fp)
	if err != nil {
		return "", err
	}

	// There should be exactly one network interface directory under the SPI device's net/ folder.
	// If there are zero or multiple, something is wrong with the hardware configuration.
	if len(files) != 1 {
		return "", fmt.Errorf("non-single directory for %s", fp)
	}

	return files[0].Name(), nil
}
