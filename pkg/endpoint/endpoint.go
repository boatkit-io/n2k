// Package endpoint declares an interface. Create a type satisfying it to support a new gateway or log file format.
package endpoint

import (
	"sync"

	"github.com/boatkit-io/n2k/pkg/adapter"
)

// Endpoint declares the interface for endpoints.
type Endpoint interface {
	Run(*sync.WaitGroup) error
	OutChannel() chan adapter.Message
}
