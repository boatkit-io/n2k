package main

import (
	//	"context"
	"context"
	"flag"
	"os"
	"strings"
	"time"

	//	"time"

	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/boatkit-io/n2k/pkg/endpoint/rawendpoint"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/boatkit-io/n2k/pkg/subscribe"

	//	"github.com/boatkit-io/n2k/pkg/subscribe"

	"github.com/sirupsen/logrus"
)

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing
	var replayFile string
	var rawReplayFile string
	flag.StringVar(&replayFile, "replayFile", "", "An optional n2k replay file to run")
	flag.StringVar(&rawReplayFile, "rawReplayFile", "", "An optional raw replay file to run")
	var dumpPgns bool
	var checkUnseen bool
	var checkMissingOrInvalid bool
	var writeRaw bool
	var rawOutFile string
	flag.StringVar(&rawOutFile, "rawOutFile", "", "if writePgns, optionally dump into this file")
	flag.BoolVar(&dumpPgns, "dumpPgns", false, "Debug spew all PGNs coming down the pipe")
	flag.BoolVar(&checkUnseen, "checkUnseen", false, "Check if any of the messages are pgns not yet seen")
	flag.BoolVar(&checkMissingOrInvalid, "checkMissingOrInvalid", false, "Check if any numeric values are missing or invalid")
	flag.BoolVar(&writeRaw, "writeRaw", false, "write out PGN structs as RAW canbus frames")
	flag.Parse()

	log := logrus.StandardLogger()
	log.Infof("in replayfile, dump:%t, checkUnseen:%t writeRaw:%t file:%s rawFile:%s\n",
		dumpPgns, checkUnseen, writeRaw, replayFile, rawReplayFile)

	subs := subscribe.New()
	ca := canadapter.NewCANAdapter(log)
	pub := pgn.NewPublisher(ca)
	ps := pkt.NewPacketStruct()
	ps.SetOutput(subs)
	ca.SetOutput(ps)

	// N2K file endpoint setup
	ep := n2kfileendpoint.NewN2kFileEndpoint(replayFile, log)
	ep.SetOutput(ca)

	// Raw file endpoint setup
	rep := rawendpoint.NewRawFileEndpoint(rawReplayFile, log)
	rep.SetOutput(ca)

	// Raw output endpoint setup
	wep := rawendpoint.NewRawEndpoint(rawOutFile, log)
	ca.SetWriter(wep)

	go func() {
		if dumpPgns {
			index := 0
			_, _ = subs.SubscribeToAllStructs(func(p any) {
				log.Infof("Handling PGN: %s", pgn.DebugDumpPGN(p))
				index++
			})
		}
		if writeRaw {
			_, _ = subs.SubscribeToAllStructs(func(p any) {
				err := pub.Write(p)
				if err != nil {
					log.Debugf("Handling PGN: %s", err)
				}
			})
		}

	}()

	//	ctx, cancel := context.WithCancel(context.Background())
	//	defer cancel()
	if len(replayFile) > 0 && strings.HasSuffix(replayFile, ".n2k") {

		//		sp := pgn.NewPublisher(ca)

		ctx := context.Background()
		err := ep.Run(ctx)
		if err != nil {
			exitCode = 1
			return
		}
		// Give subscription goroutines time to process messages
		if dumpPgns || writeRaw {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if len(rawReplayFile) > 0 {
		ctx := context.Background()
		err := rep.Run(ctx)
		if err != nil {
			exitCode = 1
			return
		}
		// Give subscription goroutines time to process messages
		if dumpPgns || writeRaw {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if writeRaw {
		ctx := context.Background()
		err := wep.Run(ctx)
		if err != nil {
			exitCode = 1
			return
		}
	}
}
