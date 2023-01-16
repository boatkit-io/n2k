package pkt

import (
	"fmt"
	"sync"

	"github.com/boatkit-io/n2k/pkg/pgn"
)

type PacketStruct struct {
	packetC chan Packet
	structC chan interface{}
}

func NewPacketStruct() *PacketStruct {

	return &PacketStruct{
		packetC: make(chan Packet, 10),
		structC: make(chan any, 10),
	}
}

func (p *PacketStruct) InChannel() chan Packet {
	return p.packetC
}

func (p *PacketStruct) OutChannel() chan any {
	return p.structC
}

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
				// call frame decoders, pass a valid match to the callback
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
				pkt.ParseErrors = append(pkt.ParseErrors, fmt.Errorf("no matching decoder"))
				p.structC <- (pkt.UnknownPGN())
			}
		}
	}()

}
