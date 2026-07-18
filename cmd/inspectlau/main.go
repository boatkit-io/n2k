// Copyright (C) 2026 Boatkit
// SPDX-License-Identifier: MIT

// Package main reports STRING_LAU byte sequences found in raw NMEA 2000 replays.
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/boatkit-io/n2k/internal/adapter/canadapter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/sirupsen/logrus"
)

type canboat struct { PGNs []canboatPGN `json:"PGNs"` }
type canboatPGN struct { PGN uint32 `json:"PGN"`; Fields []canboatField `json:"Fields"` }
type canboatField struct { Name string `json:"Name"`; Type string `json:"FieldType"` }

func main() {
	rawDir := flag.String("raw-dir", "n2kreplays/raw", "directory containing .raw replay files")
	catalog := flag.String("catalog", "canboatjson-v7.1.0.cache", "Canboat JSON catalog")
	flag.Parse()
	fields, err := loadLAUFields(*catalog)
	if err != nil { logrus.Fatal(err) }
	files, err := filepath.Glob(filepath.Join(*rawDir, "*.raw"))
	if err != nil { logrus.Fatal(err) }
	for _, file := range files { inspectFile(file, fields) }
}

func loadLAUFields(path string) (map[uint32][]string, error) {
	f, err := os.Open(path); if err != nil { return nil, err }; defer f.Close()
	var catalog canboat
	if err := json.NewDecoder(f).Decode(&catalog); err != nil { return nil, err }
	result := make(map[uint32][]string)
	for _, def := range catalog.PGNs {
		for _, field := range def.Fields { if field.Type == "STRING_LAU" { result[def.PGN] = append(result[def.PGN], field.Name) } }
	}
	return result, nil
}

func inspectFile(path string, fields map[uint32][]string) {
	f, err := os.Open(path); if err != nil { logrus.WithError(err).Warn(path); return }; defer f.Close()
	builder := canadapter.NewMultiBuilder(logrus.New())
	reader := csv.NewReader(bufio.NewReader(f))
	for {
		record, err := reader.Read()
		if err == io.EOF { return }; if err != nil || len(record) < 7 { continue }
		pgnValue, err := strconv.ParseUint(record[2], 10, 32); if err != nil { continue }
		fieldNames, ok := fields[uint32(pgnValue)]; if !ok { continue }
		source, err := strconv.ParseUint(record[3], 10, 8); if err != nil { continue }
		data := parseBytes(record[6:]); if len(data) == 0 { continue }
		packet := &pkt.Packet{Info: pgn.MessageInfo{PGN: uint32(pgnValue), SourceId: uint8(source)}, Data: data}
		if len(data) > 8 { report(path, record[0], packet, fieldNames); continue }
		builder.Add(packet)
		if packet.Complete { report(path, record[0], packet, fieldNames) }
	}
}

func parseBytes(values []string) []byte {
	data := make([]byte, 0, len(values))
	for _, value := range values { b, err := strconv.ParseUint(strings.TrimSpace(value), 16, 8); if err == nil { data = append(data, byte(b)) } }
	return data
}

func report(file, timestamp string, packet *pkt.Packet, names []string) {
	for offset := 0; offset+2 <= len(packet.Data); offset++ {
		length, encoding := int(packet.Data[offset]), packet.Data[offset+1]
		if length < 3 || encoding > 1 || offset+length > len(packet.Data) { continue }
		value := packet.Data[offset+2 : offset+length]
		if !printable(value) { continue }
		terminated := offset+length < len(packet.Data) && packet.Data[offset+length] == 0
		fmt.Printf("file=%s time=%s pgn=%d source=%d fields=%q offset=%d length=%d encoding=%d terminated=%t value=%q raw=% x\n", filepath.Base(file), timestamp, packet.Info.PGN, packet.Info.SourceId, names, offset, length, encoding, terminated, string(value), packet.Data[offset:offset+length])
	}
}

func printable(value []byte) bool { for _, b := range value { if b < 0x20 || b > 0x7e || !unicode.IsPrint(rune(b)) { return false } }; return len(value) > 0 }
