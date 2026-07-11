package n2kinternal

import (
	"bytes"
	"context"
	"testing"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type queueTestEndpoint struct{}

func (queueTestEndpoint) Run(context.Context) error         { return nil }
func (queueTestEndpoint) Close() error                      { return nil }
func (queueTestEndpoint) SetOutput(endpoint.MessageHandler) {}
func (queueTestEndpoint) WriteFrame(can.Frame)              {}

func TestHandleMessageLogsWhenHandlerQueueDrops(t *testing.T) {
	var buf bytes.Buffer
	log := logrus.New()
	log.SetOutput(&buf)
	log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	service := NewN2kService(queueTestEndpoint{}, log)
	service.messageQueue = make(chan endpoint.Message, 1)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.processorCancel = cancel

	frame := &can.Frame{
		ID:     converter.CanIDFromData(publicpgn.PositionRapidUpdatePGN, 42, 3, 255),
		Length: 8,
	}

	service.messageQueueWG.Add(1)
	service.messageQueue <- frame
	service.HandleMessage(frame)
	service.discardQueuedMessages()

	output := buf.String()
	assert.Contains(t, output, "N2K handler queue is falling behind")
	assert.Contains(t, output, "droppedTotal=1")
	assert.Contains(t, output, "stage=n2k-listener-handler-queue")
}
