package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/boatkit-io/n2k/internal/adapter/canadapter"
	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
)

// FilterOptions contains the options for filtering raw log files
type FilterOptions struct {
	InputFile   string
	Unseen      bool
	Unknown     bool
	SpecificPGN uint32
}

// FilterRawFile filters a raw log file based on the options
func FilterRawFile(opts FilterOptions) {
	log := logrus.New()
	builder := canadapter.NewMultiBuilder(log)

	file, err := os.Open(opts.InputFile)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.WithError(err).Errorf("failed to close input file %s", opts.InputFile)
		}
	}()

	reader := csv.NewReader(bufio.NewReader(file))

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading record: %v\n", err)
			continue
		}

		if len(record) < 7 {
			continue
		}

		// Parse basic fields
		pgnNum, err := strconv.ParseInt(record[2], 10, 32)
		if err != nil {
			continue
		}

		// Apply filters
		if !shouldProcessPGN(uint32(pgnNum), opts) {
			continue
		}

		sourceId, err := strconv.ParseUint(record[3], 10, 8)
		if err != nil {
			continue
		}

		dataLen, err := strconv.ParseInt(record[5], 10, 32)
		if err != nil {
			continue
		}

		// Convert hex data bytes to uint8 slice
		data := make([]uint8, 0, dataLen)
		for _, hex := range record[6:] {
			if hex == "" {
				continue
			}
			b, err := strconv.ParseUint(strings.TrimSpace(hex), 16, 8)
			if err != nil {
				continue
			}
			data = append(data, uint8(b))
		}

		if len(data) == 0 {
			continue
		}

		p := &pkt.Packet{
			Info: pgn.MessageInfo{
				PGN:      uint32(pgnNum),
				SourceId: uint8(sourceId),
			},
			Data: data,
		}

		if len(data) > 8 {
			// Fast format (consolidated)
			processPacket(p)
		} else {
			// Plain format (need to reconstruct)
			seqId := (data[0] >> 5) & 0x07
			frameNum := data[0] & 0x1F
			p.SeqId = uint8(seqId)
			p.FrameNum = uint8(frameNum)

			builder.Add(p)
			if p.Complete {
				processPacket(p)
			}
		}
	}
}

// shouldProcessPGN determines if a PGN should be processed based on the options
func shouldProcessPGN(pgnNum uint32, opts FilterOptions) bool {
	if opts.SpecificPGN != 0 {
		return pgnNum == opts.SpecificPGN
	}
	return true
}

// processPacket processes a packet and prints the raw output
func processPacket(p *pkt.Packet) {
	// Create a CAN frame from the packet
	frame := can.Frame{
		ID:     converter.CanIdFromData(p.Info.PGN, p.Info.SourceId, 2, 255), // using priority 2 and broadcast address
		Length: uint8(len(p.Data)),
		Data:   [8]uint8{},
	}
	copy(frame.Data[:], p.Data)

	// Use the converter package to format the output
	fmt.Print(converter.RawFromCanFrame(frame))
}
