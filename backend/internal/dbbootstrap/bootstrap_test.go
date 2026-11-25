package dbbootstrap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const fixturePath = "/api/v1/namespaces/app/"

func bootstrapFixture() Config {
	return Config{Namespace: "app", Secret: "pg-credentials", Marker: "pg-bootstrap", Owner: "fixture.app",
		Host: "pg-rw.app.svc", Database: "app", Username: "_app", DSNKey: "dsn", CAFile: "/etc/postgres/ca.crt"}
}

type fakeAPI struct {
	mu            sync.Mutex
	marker        resource
	secret        resource
	secretCreates int
	markerCreates int
	deny          bool
	failCreate    bool
}

func (a *fakeAPI) client(t *testing.T) *apiClient {
	t.Helper()
	s := httptest.NewTLSServer(http.HandlerFunc(a.serve))
	t.Cleanup(s.Close)
	return &apiClient{base: s.URL, token: "fixture-token", http: s.Client()}
}

func (a *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.deny || r.Header.Get("Authorization") != "Bearer fixture-token" {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "private server error body")
		return
	}
	fixture := bootstrapFixture()
	switch r.Method + " " + r.URL.Path {
	case "GET " + fixturePath + "secrets/" + fixture.Secret:
		serveResource(w, a.secret)
	case "GET " + fixturePath + "configmaps/" + fixture.Marker:
		serveResource(w, a.marker)
	case "POST " + fixturePath + "configmaps":
		if a.marker.Metadata.Name != "" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if json.NewDecoder(r.Body).Decode(&a.marker) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		a.markerCreates++
		serveResource(w, a.marker)
	case "POST " + fixturePath + "secrets":
		if a.failCreate {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.Copy(w, r.Body) // Deliberately echo private request data.
			return
		}
		if a.secret.Metadata.Name != "" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if json.NewDecoder(r.Body).Decode(&a.secret) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		a.secretCreates++
		a.secret.Data = make(map[string]string)
		for key, raw := range a.secret.StringData {
			a.secret.Data[key] = base64.StdEncoding.EncodeToString([]byte(raw))
		}
		a.secret.StringData = nil
		serveResource(w, a.secret)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func serveResource(w http.ResponseWriter, value resource) {
	if value.Metadata.Name == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}

func TestConcurrentJobsReuseOnlyCreatedCredentials(t *testing.T) {
	fixture := bootstrapFixture()
	if err := fixture.validate(); err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{}
	client := api.client(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var jobs sync.WaitGroup
	for range 8 {
		jobs.Go(func() {
			if err := bootstrap(ctx, client, fixture); err != nil {
				t.Error(err)
			}
		})
	}
	jobs.Wait()
	api.mu.Lock()
	original := api.secret
	created, reserved := api.secretCreates, api.markerCreates
	api.mu.Unlock()
	if created != 1 || reserved != 1 || verifySecret(original, fixture) != nil {
		t.Fatal("bootstrap did not publish exactly one matching credential set")
	}
	for range 2 {
		if err := bootstrap(ctx, client, fixture); err != nil {
			t.Fatal(err)
		}
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.secretCreates != 1 || api.markerCreates != 1 || !reflect.DeepEqual(api.secret, original) {
		t.Fatal("reconciliation rewrote the credentials or marker")
	}
}

func TestRetainedReservationBlocksRegeneration(t *testing.T) {
	api := &fakeAPI{failCreate: true}
	client := api.client(t)
	fixture := bootstrapFixture()
	if err := bootstrap(t.Context(), client, fixture); err == nil || err.Error() != "Kubernetes bootstrap API returned HTTP 500" {
		t.Fatalf("unexpected response to a failed creation: %v", err)
	}
	api.mu.Lock()
	api.failCreate = false
	api.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	if err := bootstrap(ctx, client, fixture); err == nil || !strings.Contains(err.Error(), "Secret is missing") {
		t.Fatalf("missing credential guard: %v", err)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.secretCreates != 0 || api.markerCreates != 1 {
		t.Fatal("credentials were regenerated after reservation")
	}
}

func TestBootstrapPermissionAndIdentityFailClosed(t *testing.T) {
	api := &fakeAPI{deny: true}
	client := api.client(t)
	fixture := bootstrapFixture()
	if err := bootstrap(t.Context(), client, fixture); err == nil || err.Error() != "Kubernetes bootstrap API returned HTTP 403" {
		t.Fatalf("unexpected permission result: %v", err)
	}
	api.mu.Lock()
	if api.markerCreates != 0 || api.secretCreates != 0 {
		t.Error("permission failure created resources")
	}
	api.deny = false
	api.mu.Unlock()
	if err := bootstrap(t.Context(), client, fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Host = "different.app.svc"
	if err := bootstrap(t.Context(), client, fixture); err == nil {
		t.Fatal("accepted an existing secret for a different server")
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.secretCreates != 1 || api.markerCreates != 1 {
		t.Fatal("mismatched identity rewrote bootstrap resources")
	}
}

func TestExistingSecretCanReestablishMarkerWithoutChangingCredentials(t *testing.T) {
	api := &fakeAPI{}
	client := api.client(t)
	fixture := bootstrapFixture()
	if err := bootstrap(t.Context(), client, fixture); err != nil {
		t.Fatal(err)
	}
	api.mu.Lock()
	original := api.secret
	api.marker = resource{}
	api.mu.Unlock()
	if err := bootstrap(t.Context(), client, fixture); err != nil {
		t.Fatal(err)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.secretCreates != 1 || api.markerCreates != 2 || !reflect.DeepEqual(api.secret, original) {
		t.Fatal("restoring only the marker changed credentials")
	}
}
