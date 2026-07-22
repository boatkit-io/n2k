// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package n2kinternal

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
)

const (
	processingMetricsTopCount = 5
	maxInt64                  = uint64(1<<63 - 1)
)

type processingMetrics struct {
	mu          sync.Mutex
	windowStart time.Time

	frameStats      durationStats
	packetStats     durationStats
	subscriberStats durationStats
	callbackStats   durationStats
	queueWaitStats  durationStats

	pgns      map[uint32]uint64
	callbacks map[string]*durationStats

	inFlightCallback      string
	inFlightCallbackStart time.Time
}

type durationStats struct {
	count uint64
	total time.Duration
	max   time.Duration
}

type processingMetricsSnapshot struct {
	interval time.Duration

	frameStats      durationStats
	packetStats     durationStats
	subscriberStats durationStats
	callbackStats   durationStats
	queueWaitStats  durationStats

	topPGNs             string
	topCallbacks        string
	inFlightCallback    string
	inFlightCallbackAge time.Duration
}

func newProcessingMetrics() *processingMetrics {
	return &processingMetrics{
		windowStart: time.Now(),
		pgns:        map[uint32]uint64{},
		callbacks:   map[string]*durationStats{},
	}
}

func (m *processingMetrics) observeFrame(pgn uint32, hasPGN bool, duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frameStats.observe(duration)
	if hasPGN {
		m.pgns[pgn]++
	}
}

func (m *processingMetrics) observePacket(duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.packetStats.observe(duration)
}

func (m *processingMetrics) observeSubscriber(duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscriberStats.observe(duration)
}

func (m *processingMetrics) observeCallback(structName, callbackName string, duration time.Duration) {
	if m == nil {
		return
	}
	key := structName + "/" + callbackName
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbackStats.observe(duration)
	stats := m.callbacks[key]
	if stats == nil {
		stats = &durationStats{}
		m.callbacks[key] = stats
	}
	stats.observe(duration)
}

func (m *processingMetrics) callbackStarted(structName, callbackName string, now time.Time) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.inFlightCallback = structName + "/" + callbackName
	m.inFlightCallbackStart = now
	m.mu.Unlock()
}

func (m *processingMetrics) callbackFinished() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.inFlightCallback = ""
	m.inFlightCallbackStart = time.Time{}
	m.mu.Unlock()
}

func (m *processingMetrics) observeQueueWait(duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueWaitStats.observe(duration)
}

func (m *processingMetrics) snapshot(now time.Time) processingMetricsSnapshot {
	if m == nil {
		return processingMetricsSnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	snapshot := processingMetricsSnapshot{
		interval:         now.Sub(m.windowStart),
		frameStats:       m.frameStats,
		packetStats:      m.packetStats,
		subscriberStats:  m.subscriberStats,
		callbackStats:    m.callbackStats,
		queueWaitStats:   m.queueWaitStats,
		topPGNs:          formatTopPGNs(m.pgns, processingMetricsTopCount),
		topCallbacks:     formatTopCallbacks(m.callbacks, processingMetricsTopCount),
		inFlightCallback: m.inFlightCallback,
	}
	if !m.inFlightCallbackStart.IsZero() {
		snapshot.inFlightCallbackAge = now.Sub(m.inFlightCallbackStart)
	}

	m.windowStart = now
	m.frameStats = durationStats{}
	m.packetStats = durationStats{}
	m.subscriberStats = durationStats{}
	m.callbackStats = durationStats{}
	m.queueWaitStats = durationStats{}
	m.pgns = map[uint32]uint64{}
	m.callbacks = map[string]*durationStats{}

	return snapshot
}

func (s *processingMetricsSnapshot) addFields(fields logrus.Fields) {
	if s.interval > 0 {
		fields["processingInterval"] = s.interval.String()
		fields["processedFrameRateHz"] = rate(float64(s.frameStats.count), s.interval)
	}
	fields["processedFrames"] = s.frameStats.count
	fields["framePipelineAvgMs"] = millis(s.frameStats.avg())
	fields["framePipelineMaxMs"] = millis(s.frameStats.max)
	fields["packetStructs"] = s.packetStats.count
	fields["packetPipelineAvgMs"] = millis(s.packetStats.avg())
	fields["packetPipelineMaxMs"] = millis(s.packetStats.max)
	fields["subscriberFanouts"] = s.subscriberStats.count
	fields["subscriberFanoutAvgMs"] = millis(s.subscriberStats.avg())
	fields["subscriberFanoutMaxMs"] = millis(s.subscriberStats.max)
	fields["subscriberCallbacks"] = s.callbackStats.count
	fields["subscriberCallbackAvgMs"] = millis(s.callbackStats.avg())
	fields["subscriberCallbackMaxMs"] = millis(s.callbackStats.max)
	fields["queueWaitAvgMs"] = millis(s.queueWaitStats.avg())
	fields["queueWaitMaxMs"] = millis(s.queueWaitStats.max)
	if s.topPGNs != "" {
		fields["topProcessedPGNs"] = s.topPGNs
	}
	if s.topCallbacks != "" {
		fields["topSubscriberCallbacks"] = s.topCallbacks
	}
	if s.inFlightCallback != "" {
		fields["subscriberCallbackInFlight"] = s.inFlightCallback
		fields["subscriberCallbackInFlightAge"] = s.inFlightCallbackAge.String()
	}
}

func (s *durationStats) observe(duration time.Duration) {
	s.count++
	s.total += duration
	if duration > s.max {
		s.max = duration
	}
}

func (s durationStats) avg() time.Duration {
	if s.count == 0 {
		return 0
	}
	if s.count > maxInt64 {
		return 0
	}
	return s.total / time.Duration(s.count)
}

func millis(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func rate(count float64, elapsed time.Duration) float64 {
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		return 0
	}
	return count / seconds
}

func messagePGN(message endpoint.Message) (uint32, bool) {
	frame, ok := message.(*can.Frame)
	if !ok || frame == nil {
		return 0, false
	}
	return converter.DecodeCanID(frame.ID).PGN, true
}

func formatTopPGNs(counts map[uint32]uint64, limit int) string {
	type item struct {
		pgn   uint32
		count uint64
	}
	items := make([]item, 0, len(counts))
	for pgn, count := range counts {
		items = append(items, item{pgn: pgn, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].pgn < items[j].pgn
		}
		return items[i].count > items[j].count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%d=%d", item.pgn, item.count))
	}
	return strings.Join(parts, ",")
}

func formatTopCallbacks(callbacks map[string]*durationStats, limit int) string {
	type item struct {
		name  string
		stats durationStats
	}
	items := make([]item, 0, len(callbacks))
	for name, stats := range callbacks {
		if stats == nil {
			continue
		}
		items = append(items, item{name: name, stats: *stats})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].stats.total == items[j].stats.total {
			return items[i].name < items[j].name
		}
		return items[i].stats.total > items[j].stats.total
	})
	if len(items) > limit {
		items = items[:limit]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s count=%d avgMs=%.3f maxMs=%.3f",
			item.name, item.stats.count, millis(item.stats.avg()), millis(item.stats.max)))
	}
	return strings.Join(parts, "; ")
}
