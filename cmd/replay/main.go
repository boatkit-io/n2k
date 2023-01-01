package main

import (
	//	"context"
	"flag"
	"os"
	"strings"
	"sync"

	//	"time"

	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kendpoint"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/boatkit-io/n2k/pkg/subscribe"

	//	"github.com/boatkit-io/tugboat/pkg/service"
	"github.com/sirupsen/logrus"
)

func main() {
	var exitCode int
	var activities = new(sync.WaitGroup)
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing, largely for local testing
	var replayFile string
	flag.StringVar(&replayFile, "replayFile", "", "An optional replay file to run")
	var dumpPgns bool
	flag.BoolVar(&dumpPgns, "dumpPgns", false, "Debug spew all PGNs coming down the pipe")
	flag.Parse()

	log := logrus.StandardLogger()
	packetChannel := make(chan pkt.Packet, 10)
	structChannel := make(chan interface{}, 10)
	subs := subscribe.New()
	s := pkt.NewPacketStruct(packetChannel, structChannel)
	h := pgn.NewStructHandler(structChannel, subs)

	//	ctx, cancel := context.WithCancel(context.Background())
	//	defer cancel()
	if len(replayFile) > 0 && strings.HasSuffix(replayFile, ".n2k") {
		frameChannel := make(chan adapter.Frame, 100)
		n := n2kendpoint.NewN2kEndpoint(frameChannel, replayFile, log)
		p := canadapter.NewCanAdapter(log, frameChannel, packetChannel)
		activities.Add(5)
		h.Run(activities)
		s.Run(activities)
		p.Run(activities)
		err := n.Run(activities)
		if err != nil {
			exitCode = 1
			return
		}
	}

	//	runner := service.NewRunner(log, time.Second*15, time.Second*5)

	//	n2kSvc, err := n2k.NewService(ctx, log, "")
	//	if err != nil {
	//		log.WithError(err).Error("create n2k service")
	//		exitCode = 1
	//		return
	//	}

	// For now, dump all unknown PGNs as warnings until we decide how much to filter
	//	_, _ = n2kSvc.SubscribeToPgn(n2k.UnknownPGN{}, func(p n2k.UnknownPGN) {
	//		log.Warnf("%s", n2k.DebugDumpPGN(p))
	//	})

	go func() {
		// Command-line parsing, largely for local testing

		defer activities.Done()

		if dumpPgns {
			_, _ = subs.SubscribeToAllStructs(func(p interface{}) {
				log.Infof("Handling PGN: %s", pgn.DebugDumpPGN(p))
			})
		}
	}()

	activities.Wait()

}
