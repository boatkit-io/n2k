package canbus

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetCanInterfaceNameForSpiDevice returns the can interface name (i.e. can0/can1) for
// a given spi device (i.e. "spi0.0").
func GetCanInterfaceNameForSpiDevice(spiName string) (string, error) {
	fp := filepath.Join(string(os.PathSeparator), "sys", "bus", "spi", "devices", spiName, "net")

	files, err := os.ReadDir(fp)
	if err != nil {
		return "", err
	}

	if len(files) != 1 {
		return "", fmt.Errorf("non-single directory for %s", fp)
	}

	return files[0].Name(), nil
}
