package n2k

import (
	"time"

	"github.com/brutella/can"
)

// PacketInfo contains info from the packet that will later go into the PGN structure as context
// /around/ the PGN struct's sending.
type PacketInfo struct {
	// when did we get the message
	Timestamp time.Time

	// 3-bit
	Priority uint8

	// 19-bit number
	PGN uint32

	// actually 8-bit
	SourceId uint8

	// target address, when relevant (PGNs with PF < 240)
	TargetId uint8
}

func newPacketInfo(message can.Frame) PacketInfo {
	p := PacketInfo{
		Timestamp: time.Now(),
		SourceId:  uint8(message.ID & 0xFF),
		PGN:       (message.ID & 0x3FFFF00) >> 8,
		Priority:  uint8((message.ID & 0x1C000000) >> 26),
	}

	pduFormat := uint8((p.PGN & 0xFF00) >> 8)
	if pduFormat < 240 {
		// This is a targeted packet, and the lower PS has the address
		p.TargetId = uint8(p.PGN & 0xFF)
		p.PGN &= 0xFFF00
	}
	return p
}
