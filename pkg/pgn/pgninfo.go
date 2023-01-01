package pgn

type PgnInfo struct {
	PGN         uint32
	Description string
	Fast        bool
	ManId       ManufacturerCodeConst
	Decoder     func(MessageInfo, *PGNDataStream) (interface{}, error)
	FieldInfo   map[int]FieldDescriptor
}

type FieldDescriptor struct {
	BitLength         uint16
	BitLengthVariable bool
	FieldType         string
	Resolution        float32
	Signed            bool
}

var PgnInfoLookup map[uint32][]*PgnInfo

func init() {
	PgnInfoLookup = make(map[uint32][]*PgnInfo)

	for i := range pgnList {
		pi := &pgnList[i]
		val := PgnInfoLookup[pi.PGN]
		if val == nil {
			val = make([]*PgnInfo, 1)
			val[0] = pi
		} else {
			val = append(val, pi)
		}
		PgnInfoLookup[pi.PGN] = val
	}
}

func IsProprietaryPGN(pgn uint32) bool {
	if pgn >= 0x0EF00 && pgn <= 0x0EFFF {
		// proprietary PDU1 (addressed) single-frame range 0EF00 to 0xEFFF (61184 - 61439) messages.
		// Addressed means that you send it to specific node on the bus. This you can easily use for responding,
		// since you know the sender. For sender it is bit more complicate since your device address may change
		// due to address claiming. There is N2kDeviceList module for handling devices on bus and find them by
		// "NAME" (= 64 bit value set by SetDeviceInformation ).
		return true
	} else if pgn >= 0x0FF00 && pgn <= 0x0FFFF {
		// proprietary PDU2 (non addressed) single-frame range 0xFF00 to 0xFFFF (65280 - 65535).
		// Non addressed means that destination wil be 255 (=broadcast) so any cabable device can handle it.
		return true
	} else if pgn >= 0x1EF00 && pgn <= 0x1EFFF {
		// proprietary PDU1 (addressed) fast-packet PGN range 0x1EF00 to 0x1EFFF (126720 - 126975)
		return true
	} else if pgn >= 0x1FF00 && pgn <= 0x1FFFF {
		// proprietary PDU2 (non addressed) fast packet range 0x1FF00 to 0x1FFFF (130816 - 131071)
		return true
	}

	return false
}

func GetProprietaryInfo(data []uint8) (ManufacturerCodeConst, IndustryCodeConst, error) {
	stream := NewPgnDataStream(data)
	var man ManufacturerCodeConst
	var ind IndustryCodeConst
	var err error
	if v, err := stream.readLookupField(11); err == nil {
		man = ManufacturerCodeConst(v)
	}
	_ = stream.skipBits(2)
	if v, err := stream.readLookupField(3); err == nil {
		ind = IndustryCodeConst(v)
	}
	return man, ind, err
}
