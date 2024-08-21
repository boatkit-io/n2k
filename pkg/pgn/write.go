package pgn

import "fmt"

type PgnWriter interface {
	WritePgn(MessageInfo, []uint8) error
}

type Publish struct {
	handler PgnWriter
}

func NewPublisher(handler PgnWriter) Publish {
	return Publish{
		handler: handler,
	}
}

type PgnStruct interface {
	Encode(*DataStream) (*MessageInfo, error)
	Marshal() ([]byte, error)
}

// Write writes a golang type describing a PGN to the n2k network.
// If validates the type passed in and returns an error if invalid
// The pgn is written to the network asynchronously, so errors are logged
func (p *Publish) Write(s any) error {
	var info *MessageInfo
	var err error
	data := make([]uint8, 223, 223)
	stream := NewDataStream(data)
	if v, ok := s.(PgnStruct); ok {
		info, err = v.Encode(stream)
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
