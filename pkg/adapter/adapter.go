// Package adapter defines an interface to read NMEA 2000 messages and output Packets, an intermediate representation.
// Implement a type satisfying the interface for a specific NMEA gateway/endpoint.
//
// Different NMEA 2000 gateways and endpoints produce messages in different formats. The
// adapter package provides a common Message interface that all gateway-specific adapters
// accept as input. Each adapter implementation (e.g., canadapter for raw CAN bus frames)
// knows how to parse its specific input format and produce pkt.Packet instances for
// downstream decoding.
//
// The data flow is:
//
//	Physical NMEA 2000 bus -> Gateway hardware -> Endpoint software -> adapter.Message
//	-> Adapter implementation -> pkt.Packet -> pkt.PacketStruct -> Typed Go struct
package adapter

// Message is a generic type for messages passed between an endpoint and an adapter.
// Each adapter implementation type-asserts the Message to its expected concrete type
// (e.g., *can.Frame for the CAN adapter). This interface allows the adapter layer to
// accept messages from any endpoint without coupling to a specific wire format.
type Message interface {
}
