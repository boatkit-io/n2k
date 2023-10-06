package pkt

import (
	"fmt"

	"github.com/boatkit-io/goatutils/pkg/subscribableevent"
	"github.com/boatkit-io/n2k/pkg/pgn"
)

// PacketStruct methods convert Packets to golang structs and sends them on.
type PacketStruct struct {
	pgnReady subscribableevent.Event[func(any)]
}

// NewPacketStruct initializes and returns a new PacketStruct instance.
func NewPacketStruct() *PacketStruct {
	return &PacketStruct{
		pgnReady: subscribableevent.NewEvent[func(any)](),
	}
}

// SubscribeToPGNReady subscribes a callback function for whenever a PGN is ready
func (ps *PacketStruct) SubscribeToPGNReady(f func(any)) subscribableevent.SubscriptionId {
	return ps.pgnReady.Subscribe(f)
}

// UnsubscribeFromPGNReady unsubscribes a previous subscription for ready PGNs
func (ps *PacketStruct) UnsubscribeFromPGNReady(t subscribableevent.SubscriptionId) error {
	return ps.pgnReady.Unsubscribe(t)
}

// ProcessPacket is how you tell PacketStruct to start processing a new packet into a PGN
func (ps *PacketStruct) ProcessPacket(pkt Packet) {
	if len(pkt.Decoders) > 0 {
		// call frame decoders, send valid return on.
		for _, decoder := range pkt.Decoders {
			stream := pgn.NewPgnDataStream(pkt.Data)
			ret, err := decoder(pkt.Info, stream)
			if err != nil {
				pkt.ParseErrors = append(pkt.ParseErrors, err)
				continue
			} else {
				ps.pgnReady.Fire(ret)
				return
			}
		}

		// no decoder succeeded
		ps.pgnReady.Fire(pkt.UnknownPGN())
	} else {
		// No valid decoder, so send on an UnknownPGN.
		pkt.ParseErrors = append(pkt.ParseErrors, fmt.Errorf("no matching decoder"))
		ps.pgnReady.Fire(pkt.UnknownPGN())
	}
}
