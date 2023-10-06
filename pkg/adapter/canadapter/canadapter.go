// Package canadapter implements the adapter interface for n2k endpoints.
package canadapter

import (
	"github.com/sirupsen/logrus"

	"github.com/boatkit-io/goatutils/pkg/subscribableevent"
	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/pkt"
)

// CANAdapter instances read canbus frames from its input and outputs complete Packets.
type CANAdapter struct {
	multi *MultiBuilder // combines multiple frames into a complete Packet.
	log   *logrus.Logger

	packetReady subscribableevent.Event[func(pkt.Packet)]
}

// NewCANAdapter instantiates a new CanAdapter
func NewCANAdapter(log *logrus.Logger) *CANAdapter {
	return &CANAdapter{
		multi: NewMultiBuilder(log),
		log:   log,

		packetReady: subscribableevent.NewEvent[func(pkt.Packet)](),
	}
}

// SubscribeToPacketReady subscribes a callback function for whenever a packet is ready
func (c *CANAdapter) SubscribeToPacketReady(f func(pkt.Packet)) subscribableevent.SubscriptionId {
	return c.packetReady.Subscribe(f)
}

// UnsubscribeFromPacketReady unsubscribes a previous subscription for ready packets
func (c *CANAdapter) UnsubscribeFromPacketReady(t subscribableevent.SubscriptionId) error {
	return c.packetReady.Unsubscribe(t)
}

// ProcessMessage is how you tell CanAdapter to start processing a new message into a packet
func (c *CANAdapter) ProcessMessage(message adapter.Message) {
	switch f := message.(type) {
	case Frame:
		pInfo := NewPacketInfo(f)
		packet := pkt.NewPacket(pInfo, f.Data[:])

		// https://endige.com/2050/nmea-2000-pgns-deciphered/

		if len(packet.ParseErrors) > 0 {
			c.packetReady.Fire(*packet)
			return
		}

		if packet.Fast {
			// TODO: What does this go into?  Black hole? :D
			c.multi.Add(packet)
		} else {
			packet.Complete = true
		}
		if packet.Complete {
			packet.AddDecoders()
			c.packetReady.Fire(*packet)
		}
	default:
		c.log.Warnf("CanAdapter expected Frame, received: %T\n", f)
	}
}
