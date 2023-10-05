// Package adapter defines an interface to read NMEA 2000 messages and output Packets, an intermediate representation.
// Implement a type satisfying the interface for a specific NMEA gateway/endpoint.
package adapter

import (
	"sync"

	"github.com/boatkit-io/n2k/pkg/pkt"
)

// Message is a generic type for messages passed between and endpoint and an adapter.
type Message interface {
}

// Adapter declares a interface defintion for an Adapter.
// Implement the interface to adapt a specific NMEA 2000 gateway/endpoint.
type Adapter interface {
	Run(*sync.WaitGroup) error
	SetInChannel(chan Message)
	SetOutChannel(chan pkt.Packet)
}
