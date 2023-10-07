// Package canadapter implements the adapter interface for n2k endpoints.
package canadapter

import (
	"github.com/sirupsen/logrus"

	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/pkt"
)

// CANAdapter instances read canbus frames from its input and outputs complete Packets.
type CANAdapter struct {
	multi *MultiBuilder // combines multiple frames into a complete Packet.
	log   *logrus.Logger

	handler PacketHandler
}

// PacketHandler is an interface for the output handler for a CANAdapter
type PacketHandler interface {
	HandlePacket(pkt.Packet)
}

// NewCANAdapter instantiates a new CanAdapter
func NewCANAdapter(log *logrus.Logger) *CANAdapter {
	return &CANAdapter{
		multi: NewMultiBuilder(log),
		log:   log,
	}
}

// SetOutput assigns a handler for any ready packets
func (c *CANAdapter) SetOutput(ph PacketHandler) {
	c.handler = ph
}

// HandleMessage is how you tell CanAdapter to start processing a new message into a packet
func (c *CANAdapter) HandleMessage(message adapter.Message) {
	switch f := message.(type) {
	case *Frame:
		pInfo := NewPacketInfo(f)
		packet := pkt.NewPacket(pInfo, f.Data[:])

		// https://endige.com/2050/nmea-2000-pgns-deciphered/

		if len(packet.ParseErrors) > 0 {
			c.packetReady(packet)
			return
		}

		if packet.Fast {
			c.multi.Add(packet)
		} else {
			packet.Complete = true
		}

		if packet.Complete {
			packet.AddDecoders()
			c.packetReady(packet)
		}
	default:
		c.log.Warnf("CanAdapter expected Frame, received: %T\n", f)
	}
}

// packetReady is a helper for fanning out completed packets to the handler
func (c *CANAdapter) packetReady(packet *pkt.Packet) {
	if c.handler != nil {
		c.handler.HandlePacket(*packet)
	}
}
