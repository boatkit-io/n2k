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

func NewPacketStruct(p chan Packet, s chan interface{}) *PacketStruct {

	return &PacketStruct{
		packetC: p,
		structC: s,
	}
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
