// Package canadapter implements the adapter interface for n2k endpoints.
package canadapter

import (
	"fmt"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"

	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/pkt"
)

// CANAdapter instances read canbus frames from its input and outputs complete Packets.
type CANAdapter struct {
	multi *MultiBuilder // combines multiple frames into a complete Packet.
	log   *logrus.Logger

	handler   PacketHandler
	canLogger func(string)
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

// SetCanLogger assigns a handler to output received frames as text ("RAW initially")
func (c *CANAdapter) SetCanLogger(cl func(string)) {
	c.canLogger = cl
}

// HandleMessage is how you tell CanAdapter to start processing a new message into a packet
func (c *CANAdapter) HandleMessage(message adapter.Message) {
	switch f := message.(type) {
	case *can.Frame:
		pInfo := NewPacketInfo(f)
		packet := pkt.NewPacket(pInfo, f.Data[:])

		c.logFrame(packet)

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
		c.log.Warnf("CanAdapter expected *can.Frame, received: %T", f)
	}
}

// packetReady is a helper for fanning out completed packets to the handler
func (c *CANAdapter) packetReady(packet *pkt.Packet) {
	if c.handler != nil {
		c.handler.HandlePacket(*packet)
	}
}

// logFrame is a helper for logging frames. Initially it outputs a
// simple RAW format string, but can be enriched...
func (c *CANAdapter) logFrame(p *pkt.Packet) {
	if c.canLogger != nil {
		c.canLogger(fmt.Sprintf("%s,%d,%d,%d,%d,%d,%02x,%02x,%02x,%02x,%02x,%02x,%02x,%02x", p.Info.Timestamp.Format("2006-01-02T15:04:05Z"), p.Info.Priority, p.Info.PGN, p.Info.SourceId, p.Info.TargetId, p.Info.Length, p.Data[0], p.Data[1], p.Data[2], p.Data[3], p.Data[4], p.Data[5], p.Data[6], p.Data[7]))
	}
}
