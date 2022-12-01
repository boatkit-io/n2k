package main

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"net/http"

	//	"math"

	"strconv"

	//	"math"
	"os"
	"strings"
	"time"

	"github.com/brutella/can"
	//	"github.com/Masterminds/sprig/v3"
	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
)

var log = logrus.StandardLogger()
var seqId uint8

// basic strategy:
// read input into slice
// use filetypeIn to select which type to start with
// convert input type to canonical packets type
// use filetypeOut to select which type to init with packets
// use groupByPgns to choose output ordering
// write the (transformed) contents to filePathOut

type packet struct {
	time        time.Time
	pgn         uint32
	source      uint8
	destination uint8
	priority    uint8
	timeDelta   float32
	canDead     string
	frame       can.Frame
}

type genericFmt interface {
	SetContents(in []byte)
	SetPackets(in []packet)
	GetContents() []byte
	GetPackets() []packet
	SetGrouping(on bool)
}

type rawFmt struct {
	contents []byte
	packets  []packet
	grouping bool
}

type canFmt struct {
	contents []byte
	packets  []packet
	grouping bool
}

type n2kFmt struct {
	contents []byte
	packets  []packet
	grouping bool
}

// Convert various Can log file formats into our preferred format
// raw (2 date/time formats)
// extended raw (2 date/time formats)
// actisense
// ?

func main() {
	fmt.Println("Entered Main")
	// Command-line parsing, largely for local testing
	var canDumpUrl string
	var fileTypeIn string
	var fileTypeOut string
	var filePathIn string
	var filePathOut string
	var groupPGNs bool
	flag.StringVar(&canDumpUrl, "canDumpUrl", "", "url of can messages to convert")
	flag.StringVar(&filePathIn, "filePathIn", "", "path of local file to convert")
	flag.StringVar(&filePathOut, "filePathOut", "", "output path")
	flag.StringVar(&fileTypeIn, "fileTypeIn", "", "Format of input file (raw, CAN)")
	flag.StringVar(&fileTypeOut, "fileTypeOut", "", "Format of output file (raw, n2k)")
	flag.BoolVar(&groupPGNs, "groupPGNs", false, "Group messages by PGN (raw output only")
	flag.Parse()

	var content []byte = make([]byte, 0)
	var inVar genericFmt
	var outVar genericFmt
	var err error
	switch fileTypeIn {
	case "raw":
		inVar = &rawFmt{
			contents: make([]byte, 0),
			packets:  make([]packet, 0),
		}
	case "CAN":
		inVar = &canFmt{
			contents: make([]byte, 0),
			packets:  make([]packet, 0),
		}
	case "n2k":
		inVar = &n2kFmt{
			contents: make([]byte, 0),
			packets:  make([]packet, 0),
		}
	default:
		panic("don't recognize dump file of type: " + fileTypeIn)
	}
	switch fileTypeOut {
	case "raw":
		outVar = &rawFmt{
			contents: make([]byte, 0),
			packets:  make([]packet, 0),
		}
		//	case "CAN":
		//		outVar = &canFmt{}
	case "n2k":
		outVar = &n2kFmt{
			contents: make([]byte, 0),
			packets:  make([]packet, 0),
		}
	default:
		panic("don't recognize dump file of type: " + fileTypeOut)
	}
	if len(canDumpUrl) > 0 {
		if len(filePathIn) > 0 {
			panic("Choose one of: canDumpUrl or filePathIn")
		}
		content, err = loadCachedWebContent("dump.cache", canDumpUrl)
		if err != nil {
			panic(err)
		}
	} else if len(filePathIn) > 0 {
		content, err = loadLocalFile(filePathIn)
		if err != nil {
			panic(err)
		}
	}
	inVar.SetGrouping(groupPGNs)
	inVar.SetContents(content)
	outVar.SetPackets(inVar.GetPackets())
	writeDumpFile(outVar.GetContents(), filePathOut, fileTypeOut)
}

func (r *rawFmt) Update() {
	if len(r.contents) != 0 {
		r.processContents()
		if r.grouping {
			r.packets = group(r.packets)
		}
	} else {
		r.processPackets()
	}
}

func (r *rawFmt) SetContents(in []byte) {
	r.contents = in
	r.Update()
}

func (r *rawFmt) GetContents() []byte {
	return r.contents
}

func (r *rawFmt) SetPackets(in []packet) {
	r.packets = in
	r.Update()
}

func (r *rawFmt) GetPackets() []packet {
	return r.packets
}

func (r *rawFmt) SetGrouping(on bool) {
	r.grouping = on
}

func (r *rawFmt) processContents() {
	var result []packet
	var baseTime time.Time
	content := string(r.contents)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		pkt := packet{canDead: "can1"}
		if (len(line) == 0) || strings.HasPrefix(line, "#") || strings.Compare(line, "\n") == 0 {
			continue
		}
		elems := strings.Split(line, ",")
		messageTime, err := time.Parse("2006-01-02-15:04:05", elems[0])
		if err != nil {
			messageTime, err = time.Parse("2006-01-02T15:04:05Z", elems[0])
			if err != nil {
				messageTime, err = time.Parse("2006-01-02T15:04:05", elems[0])
				if err != nil {
					break
				}
			}
		}
		pkt.time = messageTime
		if baseTime.IsZero() {
			baseTime = messageTime
		}
		pkt.timeDelta = float32(messageTime.Sub(baseTime))
		if pkt.timeDelta < 0 {
			pkt.timeDelta = 0.003
		} else if pkt.timeDelta > 1.0 {
			pkt.timeDelta = 0.03
		}
		priority, _ := strconv.ParseUint(elems[1], 10, 8)
		pgn, _ := strconv.ParseUint(elems[2], 10, 32)
		source, _ := strconv.ParseUint(elems[3], 10, 8)
		destination, _ := strconv.ParseUint(elems[4], 10, 8)
		pkt.frame.ID = uint32(formCanFrameID(uint64(pgn), uint64(priority), uint64(source), uint64(destination)))
		length, _ := strconv.ParseUint(elems[5], 10, 8)
		if length > 8 {
			result = append(result, makeFastPackets(pkt, elems[6:])...)
		} else {
			pkt.frame.Length = uint8(length)
			for i := 0; i < int(length); i++ {
				b, _ := strconv.ParseUint(elems[i+6], 16, 8)
				pkt.frame.Data[i] = uint8(b)
			}
			result = append(result, pkt)
		}
	}
	r.packets = result
}

func (r *rawFmt) processPackets() {
	for _, paket := range r.packets {
		line := fmt.Sprintf("%s,%d,%d,%d,%d,%d,%02x,%02x,%02x,%02x,%02x,%02x,%02x,%02x\n", paket.time.Format("2006-01-02T15:04:05Z"), paket.priority, paket.pgn, paket.source, paket.destination, paket.frame.Length, paket.frame.Data[0], paket.frame.Data[1], paket.frame.Data[2], paket.frame.Data[3], paket.frame.Data[4], paket.frame.Data[5], paket.frame.Data[6], paket.frame.Data[7])
		r.contents = append(r.contents, line...)
	}
}

func (n *n2kFmt) Update() {
	if len(n.contents) != 0 {
		n.processContents()
		if n.grouping {
			n.packets = group(n.packets)
		}
	} else {
		n.processPackets()
	}
}

func (n *n2kFmt) SetContents(in []byte) {
	n.contents = in
	n.Update()
}

func (n *n2kFmt) GetContents() []byte {
	return n.contents
}

func (n *n2kFmt) SetPackets(in []packet) {
	n.packets = in
	n.Update()
}

func (n *n2kFmt) GetPackets() []packet {
	return n.packets
}

func (n *n2kFmt) SetGrouping(on bool) {
	n.grouping = on
}

func (n *n2kFmt) processContents() {
	result := packet{}
	n.packets = make([]packet, 0)
	baseTime := time.Now()
	content := string(n.contents)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		fmt.Sscanf(line, " (%f)  %s  %8X   [%d]  %X %X %X %X %X %X %X %X", &result.timeDelta, &result.canDead, &result.frame.ID, &result.frame.Length, &result.frame.Data[0], &result.frame.Data[1], &result.frame.Data[2], &result.frame.Data[3], &result.frame.Data[4], &result.frame.Data[5], &result.frame.Data[6], &result.frame.Data[7])
		baseTime = baseTime.Add(time.Duration(result.timeDelta))
		result.time = baseTime
		result.decodeCanFrameID()
		n.packets = append(n.packets, result)
	}

}

func (n *n2kFmt) processPackets() {
	for _, paket := range n.packets {
		line := fmt.Sprintf(" (%f)	%s	%08X	[%d]  %02x %02x %02x %02x %02x %02x %02x %02x\n", paket.timeDelta, paket.canDead, paket.frame.ID, paket.frame.Length, paket.frame.Data[0], paket.frame.Data[1], paket.frame.Data[2], paket.frame.Data[3], paket.frame.Data[4], paket.frame.Data[5], paket.frame.Data[6], paket.frame.Data[7])
		n.contents = append(n.contents, line...)
	}
}

func (c *canFmt) Update() {
	if len(c.contents) == 0 {
		panic("we don't write .CAN files, sorry")
	} else {
		c.processContents()
		if c.grouping {
			c.packets = group(c.packets)
		}
	}
}
func (c *canFmt) SetContents(in []byte) {
	c.contents = in
	c.Update()
}

func (c *canFmt) GetContents() []byte {
	return c.contents
}

func (c *canFmt) SetPackets(in []packet) {
	c.packets = in
	c.Update()
}

func (c *canFmt) GetPackets() []packet {
	return c.packets
}

func (c *canFmt) SetGrouping(on bool) {
	c.grouping = on
}

func (c *canFmt) processContents() {
	c.packets = make([]packet, 0)
	baseTime := time.Now()
	var baseMinutes, baseMillis uint16
	content := c.contents
	for {
		if len(content) < 16 { // invariant is len(content) MOD 16 == 0
			break
		} else {
			buf := content[:16]
			content = content[16:]
			paket, err := toPacket(buf, &baseMinutes, &baseMillis)
			if err != nil {
				continue // probably a service record
			}
			paket.decodeCanFrameID()
			baseTime = baseTime.Add(time.Duration(paket.timeDelta))
			paket.time = baseTime
			c.packets = append(c.packets, paket)
		}
	}
}

func group(in []packet) []packet {
	group := make(map[uint32][]packet)
	for _, pkt := range in {
		if group[pkt.pgn] == nil {
			group[pkt.pgn] = make([]packet, 0)
		}
		group[pkt.pgn] = append(group[pkt.pgn], pkt)
	}
	out := make([]packet, len(in))
	for _, pkts := range group {
		out = append(out, pkts...)
	}
	return out
}

func loadLocalFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		panic(err)
	}
	return content, nil
}

func writeDumpFile(out []byte, path, fType string) {
	if f, err := os.Create(path + "." + fType); err != nil {
		panic(err)
	} else {
		defer f.Close()
		_, err := f.Write(out)
		if err != nil {
			panic(err)
		}
	}

}

func toPacket(line []byte, baseMinutes *uint16, baseMillis *uint16) (packet, error) {
	result := packet{}
	header := getuint16(line)
	if header&0x8000 != 0 {
		panic("require 29 bit ID")
	}
	if header&0x4000 != 0 {
		return result, fmt.Errorf("service record") // service record
	}
	result.frame.Length = uint8(header&0x3800>>11) + 1
	if header&0x0400 == 0 {
		result.canDead = "can1"
	} else {
		result.canDead = "can2"
	}

	minutes := header & 0x03ff
	if *baseMinutes == uint16(0) {
		*baseMinutes = minutes
	}
	deltaMinutes := minutes - *baseMinutes
	*baseMinutes = minutes

	millis := getuint16(line[2:])
	if *baseMillis == 0 {
		*baseMillis = millis
	}
	deltaMillis := millis - *baseMillis
	*baseMillis = millis
	tDelta := float32(deltaMinutes) * 60
	tDelta += float32(deltaMillis) / 60000
	result.timeDelta = tDelta
	result.frame.ID = getuint32(line[4:])
	copy(result.frame.Data[:], line[8:])

	return result, nil
}

func getuint16(buf []byte) uint16 {
	return uint16(buf[0]) + (uint16(buf[1]) << 8)
}

func getuint32(buf []byte) uint32 {
	return uint32(getuint16(buf[0:])) + (uint32(getuint16(buf[2:])) << 16)
}

func makeFastPackets(paket packet, data []string) []packet {

	result := []packet{}
	if seqId > 7 {
		seqId = 0
	}
	seqFrameNum := uint8(seqId << 5)
	length := len(data)
	next := paket
	next.frame.Length = 8
	next.frame.Data[0] = seqFrameNum
	next.frame.Data[1] = uint8(length)
	for i := 0; i < 6; i++ {
		b, _ := strconv.ParseUint(data[i], 16, 8)
		next.frame.Data[i+2] = uint8(b)
	}
	data = data[6:]
	result = append(result, next)
	for {
		seqFrameNum++
		next.frame.Data[0] = seqFrameNum
		limit := 7
		if len(data) < limit {
			limit = len(data)
		}
		for i := 0; i < limit; i++ {
			b, _ := strconv.ParseUint(data[i], 16, 8)
			next.frame.Data[i+1] = uint8(b)
		}
		result = append(result, next)
		data = data[limit:]
		if len(data) == 0 {
			break
		}
	}
	return result
}

// Utility functions

func cacheFromWeb(name, url string) (string, error) {
	// get stats on cached file (name+cache)
	// if not exist or expired, get contents from web and save in cached file
	// read cached file into buffer and return
	var cacheDuration = 1 * time.Hour
	var cachedName = name + ".cache"
	fstat, err := os.Stat(cachedName)
	if err != nil || time.Since(fstat.ModTime()) > cacheDuration {
		log.Infof("Downloading source data...")

		req, _ := http.NewRequest("GET", url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return cachedName, err
		}
		defer resp.Body.Close()

		f, _ := os.OpenFile(cachedName, os.O_CREATE|os.O_WRONLY, 0644)

		bar := progressbar.DefaultBytes(
			resp.ContentLength,
			fmt.Sprintf("Downloading %s\n", name),
		)
		_, _ = io.Copy(io.MultiWriter(f, bar), resp.Body)

		f.Close()
	} else {
		log.Infof(fmt.Sprintf("Using cached file %s\n", name))
	}
	return cachedName, nil
}

func loadCachedWebContent(name, url string) ([]byte, error) {
	cachedName, err := cacheFromWeb(name, url)
	if err != nil {
		panic(err)
	}
	cacheContent, err := loadLocalFile(cachedName)
	if err != nil {
		panic(err)
	}
	return cacheContent, nil
}

func (p *packet) decodeCanFrameID() {
	p.source = uint8(p.frame.ID & 0xFF)
	p.pgn = (p.frame.ID & 0x3FFFF00) >> 8
	p.priority = uint8((p.frame.ID & 0x1C000000) >> 26)
	pduFormat := uint8((p.pgn & 0xFF00) >> 8)
	if pduFormat < 240 {
		// This is a targeted packet, and the lower PS has the address
		p.destination = uint8(p.pgn & 0xFF)
		p.pgn &= 0xFFF00
	}
}

func formCanFrameID(pgn, priority, source, destination uint64) uint64 {

	if destination != 255 {
		// This is a targeted packet, and the lower PS has the address
		pgn |= destination
	}
	result := pgn << 8
	result |= source
	result |= (priority << 26)
	return result
}
