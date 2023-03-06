package pkt

import (
	"fmt"
	"sync"

	"github.com/boatkit-io/n2k/pkg/pgn"
)

// PacketStruct methods convert Packets to golang structs and sends them on.
type PacketStruct struct {
	packetC chan Packet
	structC chan interface{}
}

// NewPacketStruct initializes and returns a new PacketStruct instance.
func NewPacketStruct() *PacketStruct {

	return &PacketStruct{
		packetC: make(chan Packet, 10),
		structC: make(chan any, 10),
	}
}

// InChannel method returns the PacketStruct's input channel.
func (p *PacketStruct) InChannel() chan Packet {
	return p.packetC
}

// OutChannel method returns the PacketStruct's output channel.
func (p *PacketStruct) OutChannel() chan any {
	return p.structC
}

// Run method kicks off a goroutine that converts incoming Packets to structs send to its output channel.
func (p *PacketStruct) Run(wg *sync.WaitGroup) {
	go func() {
		defer wg.Done()
		for {
			pkt, more := <-p.packetC
			if !more {
				close(p.structC)
				return
			}
			if len(pkt.Decoders) > 0 {
				// call frame decoders, send valid return on.
				for _, decoder := range pkt.Decoders {
					stream := pgn.NewPgnDataStream(pkt.Data)
					ret, err := decoder(pkt.Info, stream)
					if err != nil {
						pkt.ParseErrors = append(pkt.ParseErrors, err)
						continue
					} else {
						p.structC <- ret
						break
					}
				}
			} else {
				// No valid decoder, so send on an UnknownPGN.
				pkt.ParseErrors = append(pkt.ParseErrors, fmt.Errorf("no matching decoder"))
				p.structC <- (pkt.UnknownPGN())
			}
		}
	}()

}
