package main

import (
	"context"
	"flag"
	"os"

	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint/usbcanendpoint"

	"github.com/sirupsen/logrus"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := logrus.StandardLogger()
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing
	var format string // initally RAW but allow for others
	flag.StringVar(&format, "format", "", "One of: 'RAW', 'n2k', ?")

	//	var extended bool // if true, record fast packets as single frame (RAW only)
	//	flag.BoolVar(&extended, "extended", false, "for 'RAW', return fast packets as single frame")

	var outputPath string
	flag.StringVar(&outputPath, "outputPath", "", "output path")

	var canInterfaceName string
	flag.StringVar(&canInterfaceName, "usbCanName", "", "name of Can interface")

	flag.Parse()

	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// set up can endpoint, connect to canadapter
	canEndpoint := usbcanendpoint.NewUSBCANEndpoint(logger, canInterfaceName)
	canAdapter := canadapter.NewCANAdapter((logger))
	canEndpoint.SetOutput(canAdapter)

	go func() {
		canAdapter.SetCanLogger(func(out string) {
			if _, err := f.WriteString(out + "\n"); err != nil {
				panic(err)
			}
		})

	}()

	// and off we go
	err = canEndpoint.Run(ctx)
	if err != nil {
		exitCode = 1
		return
	}

}
