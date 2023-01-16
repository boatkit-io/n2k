package endpoint

import (
	"sync"
)

type Endpoint interface {
	Run(*sync.WaitGroup)
	OutChannel() chan any
}
