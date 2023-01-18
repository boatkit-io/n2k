package adapter

import (
	"sync"

	"github.com/boatkit-io/n2k/pkg/pkt"
)

type Message interface {
}

type Adapter interface {
	Run(*sync.WaitGroup) error
	SetInChannel(chan Message)
	SetOutChannel(chan pkt.Packet)
}
