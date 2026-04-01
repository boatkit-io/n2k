package canbus

import (
	"context"

	"github.com/brutella/can"
)

// Interface is a basic interface for a CANbus implementation
type Interface interface {
	Run(ctx context.Context) error
	Close() error
	WriteFrame(frame can.Frame) error
}
