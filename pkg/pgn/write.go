package pgn

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
	Encode(stream *DataStream) (*MessageInfo, error)
}

// Write writes a golang type describing a PGN to the n2k network.
// If validates the type passed in and returns an error if invalid
// The pgn is written to the network asynchronously, so errors are logged
func (p *Publish) Write(pgn PgnStruct) error {
	data := make([]uint8, 0, 223)
	stream := NewDataStream(data)
	info, err := pgn.Encode(stream)
	if err == nil {
		if p.handler != nil {
			err = p.handler.WritePgn(*info, data)
		}
	}
	return err
}
