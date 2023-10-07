// Package adapter defines an interface to read NMEA 2000 messages and output Packets, an intermediate representation.
// Implement a type satisfying the interface for a specific NMEA gateway/endpoint.
package adapter

// Message is a generic type for messages passed between and endpoint and an adapter.
type Message interface {
}
