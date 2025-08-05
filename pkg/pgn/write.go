package pgn

import "fmt"

// PgnWriter defines an interface for values that can write Pgns.
type PgnWriter interface {
	WritePgn(MessageInfo, []uint8) error
}

// Publisher defines an object that can interact with a PgnWriter
type Publisher struct {
	handler PgnWriter
}

// NewPublisher returns a new Publisher instance with the specified PgnWriter
func NewPublisher(handler PgnWriter) Publisher {
	return Publisher{
		handler: handler,
	}
}

// Write writes a golang type describing a PGN to the n2k network.
// If validates the type passed in and returns an error if invalid
// The pgn is written to the network asynchronously, so errors are logged
func (p *Publisher) Write(s any) error {
	var info *MessageInfo
	var err error
	data := make([]uint8, 223)
	stream := NewDataStream(data)
	if v, ok := s.(PgnStruct); ok {
		info, err = v.Encode(stream) // Encode returns the MessageInfo for the PGN, since we're using an interface and can't acess it directly
	} else {
		return fmt.Errorf("trying to write a struct that isn't a PGN")
	}
	if err == nil {
		if p.handler != nil {
			err = p.handler.WritePgn(*info, data[0:stream.byteOffset:stream.byteOffset])
		}
	}
	return err
}
