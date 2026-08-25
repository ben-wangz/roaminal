package clientdiag

import (
	"encoding/json"
	"errors"
	"log"
	"sort"
	"sync"
	"time"

	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

const (
	burstEvents        = 120.0
	refillEventsPerMin = 60.0
	maxLimiterEntries  = 1024
	limiterIdle        = time.Hour
)

type Logger interface {
	Printf(format string, args ...any)
}

type Sink struct {
	version string
	bootID  string
	logger  Logger
	writer  *fileWriter
	clock   ports.Clock

	mu       sync.Mutex
	limiters map[string]bucket
	seen     map[string]time.Time
	seenList []seenEntry
	warnOnce sync.Once
}

type Dependencies struct {
	Clock  ports.Clock
	Random ports.RandomSource
}

type bucket struct {
	tokens  float64
	updated time.Time
}

type seenEntry struct {
	id string
	at time.Time
}

func New(dir, version, bootID string, logger Logger, dependencies ...Dependencies) *Sink {
	if logger == nil {
		logger = log.Default()
	}
	deps := Dependencies{Clock: systemclock.System{}, Random: random.CryptoSource{}}
	if len(dependencies) > 0 {
		if dependencies[0].Clock != nil {
			deps.Clock = dependencies[0].Clock
		}
		if dependencies[0].Random != nil {
			deps.Random = dependencies[0].Random
		}
	}
	sink := &Sink{version: version, bootID: bootID, logger: logger, clock: deps.Clock, limiters: make(map[string]bucket), seen: make(map[string]time.Time)}
	if dir != "" {
		writer, err := newFileWriter(dir, Dependencies{Clock: deps.Clock, Random: deps.Random})
		if err != nil {
			sink.warn(err)
		} else {
			sink.writer = writer
		}
	}
	return sink
}

func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return nil
	}
	return s.writer.Close()
}

func (s *Sink) Accept(sessionID, userAgent string, batch Batch) error {
	return s.acceptAt(s.clock.Now().UTC(), sessionID, userAgent, batch)
}

func (s *Sink) acceptAt(now time.Time, sessionID, userAgent string, batch Batch) error {
	if sessionID == "" {
		return ErrInvalid
	}
	events, err := batch.validate(now)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	newEvents := make([]Event, 0, len(events))
	for _, event := range events {
		if _, exists := s.seen[event.EventID]; !exists {
			newEvents = append(newEvents, event)
		}
	}
	if len(newEvents) == 0 {
		return nil
	}
	if !s.allowLocked(sessionID, len(newEvents), now) {
		return ErrRateLimited
	}
	userAgent = truncateUTF8(RedactText(userAgent, MaxUserAgentBytes), MaxUserAgentBytes)
	lines := make([][]byte, 0, len(newEvents))
	for _, event := range newEvents {
		record := StoredRecord{ReceivedAt: now, RuntimeVersion: s.version, BootID: s.bootID, AuthSessionID: sessionID, UserAgent: userAgent, PageID: batch.PageID, DroppedCount: batch.DroppedCount, Event: event}
		payload, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			return marshalErr
		}
		line := append(payload, '\n')
		lines = append(lines, line)
		s.logger.Printf("client_diagnostic=%s", payload)
	}
	if s.writer != nil {
		if writeErr := s.writer.WriteBatch(lines); writeErr != nil {
			s.warn(writeErr)
		}
	}
	for _, event := range newEvents {
		s.markSeenLocked(event.EventID, now)
	}
	return nil
}

func (s *Sink) allowLocked(sessionID string, count int, now time.Time) bool {
	value, ok := s.limiters[sessionID]
	if !ok {
		value = bucket{tokens: burstEvents, updated: now}
	}
	elapsed := now.Sub(value.updated).Minutes()
	if elapsed > 0 {
		value.tokens += elapsed * refillEventsPerMin
		if value.tokens > burstEvents {
			value.tokens = burstEvents
		}
	}
	value.updated = now
	if value.tokens < float64(count) {
		s.limiters[sessionID] = value
		return false
	}
	value.tokens -= float64(count)
	s.limiters[sessionID] = value
	return true
}

func (s *Sink) markSeenLocked(id string, now time.Time) {
	s.seen[id] = now
	s.seenList = append(s.seenList, seenEntry{id: id, at: now})
	for len(s.seenList) > MaxDeduplicationIDs {
		oldest := s.seenList[0]
		s.seenList = s.seenList[1:]
		if s.seen[oldest.id].Equal(oldest.at) {
			delete(s.seen, oldest.id)
		}
	}
}

func (s *Sink) pruneLocked(now time.Time) {
	for id, value := range s.limiters {
		if now.Sub(value.updated) > limiterIdle {
			delete(s.limiters, id)
		}
	}
	if len(s.limiters) <= maxLimiterEntries {
		return
	}
	ids := make([]string, 0, len(s.limiters))
	for id := range s.limiters {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return s.limiters[ids[i]].updated.Before(s.limiters[ids[j]].updated) })
	for _, id := range ids[:len(ids)-maxLimiterEntries] {
		delete(s.limiters, id)
	}
}

func (s *Sink) warn(err error) {
	if err == nil {
		return
	}
	s.warnOnce.Do(func() {
		s.logger.Printf("client_diagnostics_sink_warning=%q", err.Error())
	})
}

func IsInvalid(err error) bool     { return errors.Is(err, ErrInvalid) }
func IsRateLimited(err error) bool { return errors.Is(err, ErrRateLimited) }
