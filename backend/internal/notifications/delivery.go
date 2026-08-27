package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// Notify is deliberately non-blocking. Message persistence has already
// succeeded by the time this method is called.
func (s *Service) Notify(record domain.MessageRecord) {
	if s == nil || !eligible(record.Kind) || !s.enabled || s.repository == nil {
		return
	}
	payload, err := notificationPayload(record)
	if err != nil {
		log.Printf("level=INFO event=web_push_payload_failed error_type=%T", err)
		return
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	if s.closed {
		return
	}
	s.pending.Add(1)
	select {
	case s.queue <- notificationJob{record: record, payload: payload}:
	default:
		s.pending.Done()
		log.Printf("level=INFO event=web_push_queue_full")
	}
}

func (s *Service) worker() {
	defer s.workers.Done()
	for job := range s.queue {
		s.dispatch(job)
		s.pending.Done()
	}
}

func (s *Service) dispatch(job notificationJob) {
	started := s.clock.Now()
	records, err := s.repository.ListPushSubscriptions(s.context)
	if err != nil {
		log.Printf("level=INFO event=web_push_delivery_list_failed error_type=%T", err)
		return
	}
	delivered, failed, removed := 0, 0, 0
	for _, record := range records {
		outcome, sendErr := s.sendWithRetry(job.payload, record)
		if outcome.Permanent {
			if err := s.DeleteByID(s.context, record.ID); err != nil {
				log.Printf("level=INFO event=web_push_subscription_delete_failed error_type=%T", err)
			} else {
				removed++
			}
		}
		if sendErr != nil {
			failed++
			continue
		}
		delivered++
	}
	log.Printf("level=INFO event=web_push_delivery_completed subscriptions=%d delivered=%d failed=%d removed=%d duration_ms=%d", len(records), delivered, failed, removed, s.clock.Since(started).Milliseconds())
}

func (s *Service) sendWithRetry(payload []byte, record domain.PushSubscriptionRecord) (SendOutcome, error) {
	var lastOutcome SendOutcome
	var lastErr error
	for attempt := 1; attempt <= s.retryAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(s.context, s.sendTimeout)
		outcome, err := s.sender.Send(ctx, payload, record)
		cancel()
		lastOutcome, lastErr = outcome, err
		if err == nil && outcome.StatusCode >= 200 && outcome.StatusCode < 300 {
			return outcome, nil
		}
		if outcome.Permanent {
			return outcome, err
		}
		if !outcome.Retryable || attempt == s.retryAttempts {
			return outcome, errOrStatus(outcome, err)
		}
		if !wait(s.context, s.retryDelay*time.Duration(attempt)) {
			return outcome, context.Canceled
		}
	}
	return lastOutcome, errOrStatus(lastOutcome, lastErr)
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func errOrStatus(outcome SendOutcome, err error) error {
	if err != nil {
		return err
	}
	if outcome.StatusCode != 0 {
		return fmt.Errorf("push service returned HTTP %d", outcome.StatusCode)
	}
	return errors.New("push delivery failed")
}

func (s *Service) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	go func() {
		s.pending.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	if !s.closed {
		s.closed = true
		if s.cancel != nil {
			s.cancel()
		}
		close(s.queue)
	}
	s.lifecycleMu.Unlock()
	s.workers.Wait()
	return nil
}

func eligible(kind string) bool {
	return kind == "codex_turn_completed" || kind == "codex_turn_failed"
}

type notificationPayloadData struct {
	MessageID string `json:"messageId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Severity  string `json:"severity"`
}

func notificationPayload(record domain.MessageRecord) ([]byte, error) {
	body := "Codex turn finished"
	if record.Kind == "codex_turn_failed" {
		body = "Codex turn failed"
	}
	return json.Marshal(notificationPayloadData{MessageID: record.MessageID, Title: "Roaminal", Body: body, Severity: record.Severity})
}
