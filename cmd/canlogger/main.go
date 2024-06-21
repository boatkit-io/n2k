package main

import (
	"flag"
	"os"
)

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing
	var format string // initally RAW but allow for others
	flag.StringVar(&format, "format", "", "One of: 'RAW', 'n2k', ?")

	var extended bool // if true, record fast packets as single frame (RAW only)
	flag.BoolVar(&extended, "extended", false, "for 'RAW', return fast packets as single frame")

	flag.Parse()

}
