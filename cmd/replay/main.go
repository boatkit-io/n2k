package main

import (
	//	"context"
	"context"
	"flag"
	"os"
	"strings"

	//	"time"

	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/boatkit-io/n2k/pkg/subscribe"

	//	"github.com/boatkit-io/n2k/pkg/subscribe"

	"github.com/sirupsen/logrus"
)

func IsMissing(value any) bool {

	switch v := value.(type) {
	case int8:
		return v>>6&0x01 == 1
	case int16:
		return v>>14&0x01 == 1
	case int32:
		return v>>30&0x01 == 1
	case int64:
		return v>>62&0x01 == 1
	case uint8:
		return v>>7&0x01 == 1
	case uint16:
		return v>>15&0x01 == 1
	case uint32:
		return v>>31&0x01 == 1
	case uint64:
		return v>>63&0x01 == 1
	default:
		// Unsupported type
		return false
	}
}

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing
	var replayFile string
	flag.StringVar(&replayFile, "replayFile", "", "An optional replay file to run")
	var dumpPgns bool
	var checkUnseen bool
	var checkMissingOrInvalid bool
	flag.BoolVar(&dumpPgns, "dumpPgns", false, "Debug spew all PGNs coming down the pipe")
	flag.BoolVar(&checkUnseen, "checkUnseen", false, "Check if any of the messages are pgns not yet seen")
	flag.BoolVar(&checkMissingOrInvalid, "checkMissingOrInvalid", false, "Check if any numeric values are missing or invalid")
	flag.Parse()

	log := logrus.StandardLogger()
	log.Infof("in replayfile, dump:%t, checkUnseen:%t file:%s\n", dumpPgns, checkUnseen, replayFile)

	subs := subscribe.New()
	go func() {
		if dumpPgns {
			_, _ = subs.SubscribeToAllStructs(func(p any) {
				log.Infof("Handling PGN: %s", pgn.DebugDumpPGN(p))
			})
		}
		if checkUnseen {
			_, _ = subs.SubscribeToAllStructs(func(p pgn.PgnStruct) {
				info, err := p.Encode(nil)
				if err == nil {
					if pgn.SearchUnseenList(info.PGN) {
						log.Infof("Unseen PGN encountered: %s", pgn.DebugDumpPGN(p))

					}
				}
			})
		}
		if checkMissingOrInvalid {
			_, _ = subs.SubscribeToAllStructs(func(p pgn.PgnStruct) {

			})
		}

	}()

	ps := pkt.NewPacketStruct()
	ps.SetOutput(subs)

	//	ctx, cancel := context.WithCancel(context.Background())
	//	defer cancel()
	if len(replayFile) > 0 && strings.HasSuffix(replayFile, ".n2k") {
		ca := canadapter.NewCANAdapter(log)
		ca.SetOutput(ps)

		ep := n2kfileendpoint.NewN2kFileEndpoint(replayFile, log)
		ep.SetOutput(ca)

		//		sp := pgn.NewPublisher(ca)

		ctx := context.Background()
		err := ep.Run(ctx)
		if err != nil {
			exitCode = 1
			return
		}
	}
}
