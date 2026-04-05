// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package main implements the replay CLI for replaying PGNs.
package main

import (
	"context"
	"flag"
	"os"
	"strings"

	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/boatkit-io/n2k/pkg/subscribe"

	"github.com/sirupsen/logrus"
)

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing
	var replayFile string
	flag.StringVar(&replayFile, "replayFile", "", "An optional replay file to run")
	var dumpPgns bool
	flag.BoolVar(&dumpPgns, "dumpPgns", false, "Debug spew all PGNs coming down the pipe")
	flag.Parse()

	log := logrus.StandardLogger()
	log.Infof("in replayfile, dump:%t, file:%s\n", dumpPgns, replayFile)

	subs := subscribe.New()
	go func() {
		if dumpPgns {
			var err error
			_, err = subs.SubscribeToAllStructs(func(p any) {
				log.Infof("Handling PGN: %s", pgn.DebugDumpPGN(p))
			})
			if err != nil {
				log.WithError(err).Error("failed")
			}
		}
	}()

	ps := pkt.NewPacketStruct()
	ps.SetOutput(subs)

	if replayFile != "" && strings.HasSuffix(replayFile, ".n2k") {
		ca := canadapter.NewCANAdapter(log)
		ca.SetOutput(ps)

		ep := n2kfileendpoint.NewN2kFileEndpoint(replayFile, log)
		ep.SetOutput(ca)

		ctx := context.Background()
		err := ep.Run(ctx)
		if err != nil {
			exitCode = 1
			return
		}
	}
}
