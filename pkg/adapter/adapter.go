package adapter

import (
	"sync"

	"github.com/boatkit-io/n2k/pkg/pkt"
)

type Adapter interface {
	Run(*sync.WaitGroup)
	SetInChannel(chan any)
	SetOutChannel(chan pkt.Packet)
}
