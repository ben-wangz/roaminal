package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type probeWorker struct {
	ports.TerminalWorker
	available bool
}

func (w *probeWorker) Available() bool { return w.available }

func TestDatabaseFailureAffectsOnlyReadiness(t *testing.T) {
	worker := &probeWorker{available: true}
	dbAvailable, checks := true, 0
	handler := New(Dependencies{Worker: worker, DatabaseReady: func(ctx context.Context) error {
		checks++
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > time.Second {
			t.Error("database readiness check has no short deadline")
		}
		if !dbAvailable {
			return errors.New("private database error")
		}
		return nil
	}}).Handler()
	get := func(path string, want int) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != want {
			t.Fatalf("GET %s: status %d, want %d", path, response.Code, want)
		}
		return response
	}

	get("/healthz", http.StatusOK)
	get("/readyz", http.StatusOK)
	dbAvailable = false
	duringOutage := get("/readyz", http.StatusServiceUnavailable)
	if !strings.Contains(duringOutage.Body.String(), "database unavailable") || strings.Contains(duringOutage.Body.String(), "private database error") ||
		duringOutage.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("readiness failure was cacheable or leaked database details")
	}
	get("/healthz", http.StatusOK)
	dbAvailable = true
	get("/readyz", http.StatusOK)
	worker.available = false
	get("/healthz", http.StatusServiceUnavailable)
	get("/readyz", http.StatusServiceUnavailable)
	if checks != 3 {
		t.Fatalf("database probed %d times, want three readiness checks with a live worker", checks)
	}
}
