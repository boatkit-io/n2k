package pgn

import (
	"sync"

	"github.com/boatkit-io/n2k/pkg/subscribe"
)

// StructHandler reads incoming structs and distributes them via subscription.
type StructHandler struct {
	structC chan interface{}
	sub     *subscribe.SubscribeManager
}

// NewStructHandler initializes a new StructHandler.
func NewStructHandler(s chan interface{}, sub *subscribe.SubscribeManager) *StructHandler {
	return &StructHandler{
		structC: s,
		sub:     sub,
	}
}

// Run method kicks off a goroutine that reads structs from its input channel and distributes them via subscription.
func (s *StructHandler) Run(wg *sync.WaitGroup) {
	go func() {
		defer wg.Done()
		for {
			x, more := <-s.structC
			if !more {
				return
			}

			s.sub.ServeStruct(x)
		}
	}()
}
