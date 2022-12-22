package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/boatkit-io/n2k/pkg/n2k"

	"github.com/boatkit-io/tugboat/pkg/service"
	"github.com/sirupsen/logrus"
)

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logrus.StandardLogger()

	runner := service.NewRunner(log, time.Second*15, time.Second*5)

	n2kSvc, err := n2k.NewService(ctx, log, "")
	if err != nil {
		log.WithError(err).Error("create n2k service")
		exitCode = 1
		return
	}

	// For now, dump all unknown PGNs as warnings until we decide how much to filter
	//	_, _ = n2kSvc.SubscribeToPgn(n2k.UnknownPGN{}, func(p n2k.UnknownPGN) {
	//		log.Warnf("%s", n2k.DebugDumpPGN(p))
	//	})

	go func() {
		// Command-line parsing, largely for local testing
		var replayFile string
		flag.StringVar(&replayFile, "replayFile", "", "An optional replay file to run")
		var dumpPgns bool
		flag.BoolVar(&dumpPgns, "dumpPgns", false, "Debug spew all PGNs coming down the pipe")
		flag.Parse()

		if replayFile != "" {
			_ = n2kSvc.ReplayFile(replayFile)
		}

		if dumpPgns {
			_, _ = n2kSvc.SubscribeToAllPgns(func(p interface{}) {
				log.Infof("Handling PGN: %s", n2k.DebugDumpPGN(p))
			})
		}
	}()

	acts := []service.Activity{service.NewShutdown(cancel), n2kSvc}
	runner.RegisterActivities(acts...)
	exitCode = runner.Run(ctx)
}
