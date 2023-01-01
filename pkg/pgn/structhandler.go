package pgn

import (
	"sync"

	"github.com/boatkit-io/n2k/pkg/subscribe"
)

type StructHandler struct {
	structC chan interface{}
	sub     *subscribe.SubscribeManager
}

func NewStructHandler(s chan interface{}, sub *subscribe.SubscribeManager) *StructHandler {
	return &StructHandler{
		structC: s,
		sub:     sub,
	}
}

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
