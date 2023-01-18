package endpoint

import (
	"sync"

	"github.com/boatkit-io/n2k/pkg/adapter"
)

type Endpoint interface {
	Run(*sync.WaitGroup) error
	OutChannel() chan adapter.Message
}
