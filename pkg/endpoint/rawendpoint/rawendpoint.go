package rawendpoint

import (
	"context"
	"fmt"
	"os"

	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
)

// RawEndpoint writes a raw log file from canbus frames sent through the write pipeline.
// Initially through stdout
type RawEndpoint struct {
	log     *logrus.Logger
	file    *os.File
	handler endpoint.MessageHandler
}

// NewRawEndpoint creates a new RAW endpoint
func NewRawEndpoint(outFilePath string, log *logrus.Logger) *RawEndpoint {
	file, err := os.Create(outFilePath)
	if err != nil {
		log.Infof("RAW output file failed to open: %s", err)
	}
	return &RawEndpoint{
		log:  log,
		file: file,
	}
}

// Run method opens the specified log file and kicks off a goroutine that sends frames to the handler
func (r *RawEndpoint) Run(ctx context.Context) error {

	defer r.file.Close()

	go func() {
		<-ctx.Done()
	}()

	return nil
}

// SetOutput sets the output struct for handling when a message is ready
func (r *RawEndpoint) SetOutput(mh endpoint.MessageHandler) {
	r.handler = mh
}

func (r *RawEndpoint) WriteFrame(frame can.Frame) {
	f := canadapter.NewPacketInfo(&frame)
	outStr := fmt.Sprintf("%s,%d,%d,%d,%d,%d,%02x,%02x,%02x,%02x,%02x,%02x,%02x,%02x\n", f.Timestamp.Format("2006-01-02T15:04:05Z"), f.Priority, f.PGN, f.SourceId, f.TargetId, frame.Length, frame.Data[0], frame.Data[1], frame.Data[2], frame.Data[3], frame.Data[4], frame.Data[5], frame.Data[6], frame.Data[7])
	if r.file != nil {
		r.file.WriteString(outStr)
	} else {
		r.log.Infof(outStr)

	}

}
