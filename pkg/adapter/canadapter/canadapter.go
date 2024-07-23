// Package canadapter implements the adapter interface for n2k endpoints.
package canadapter

import (
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"

	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
)

// CANAdapter instances on input read canbus frames from its input and outputs complete Packets.
// On output it
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
	case *can.Frame:
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
		c.log.Warnf("CanAdapter expected *can.Frame, received: %T", f)
	}
}

// packetReady is a helper for fanning out completed packets to the handler
func (c *CANAdapter) packetReady(packet *pkt.Packet) {
	if c.handler != nil {
		c.handler.HandlePacket(*packet)
	}
}

// WritePgn generates one or more frames from its input and writes them to its configured endpoint.
func (c *CANAdapter) WritePgn(info pgn.MessageInfo, data []uint8) error {
	var err error
	/* 	canId := CanIdFromData(info.PGN, info.SourceId, info.Priority, info.TargetId)
	   	if pgn.IsFast((info.PGN)) {
	   		err = c.sendFast(canId, data)
	   	} else {
	   		err = c.sendSingle(canId, data)
	   	} */
	return err
}

// CanIdFromData returns an encoded ID from its inputs.
func CanIdFromData(pgn uint32, sourceId uint8, priority uint8, target uint8) uint32 {
	return uint32(sourceId) | (pgn << 8) | (uint32(priority) << 26) | uint32(target)
}

/* func (c *CANAdapter) sendFast(canId uint32, data []uint8) error {
	return nil
}

func (c *CANAdapter) sendSingle(canId uint32, data []uint8) error {
	frame := can.Frame{
		ID:     canId,
		Length: uint8(len(data)),
	}
	for i := range data {
		frame.Data[i] = data[i]
	}
	i := frame.Length
	for i < 8 {
		frame.Data[i] = 0xFF
	}
	// invoke endpoint handler
	if c.outHandler != nil {
		c.outHandler.WriteFrame(frame)
	}
	return nil
}
*/
/*
for fast packet sending:
need to choose sequence# for sourceID|Pgn
need to send 1st packet (seq#, length) + 6.
need to send subsequent pkts (seq#) + 7
need to FF out the extra bytes in the final package (since frame.Length will still be 8)
*/
