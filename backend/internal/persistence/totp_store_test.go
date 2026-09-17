package persistence

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func validEnrollment() domain.TOTPEnrollmentRecord {
	return domain.TOTPEnrollmentRecord{FormatVersion: domain.TOTPFormatVersion, EnrollmentID: "11111111-1111-4111-8111-111111111111", Secret: "JBSWY3DPEHPK3PXP", LastAcceptedTimeStep: 42}
}

func TestTOTPEnrollmentRoundTripAndPermissions(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepositories(store).TOTPEnrollment
	if _, exists, err := repo.LoadEnrollment(context.Background()); err != nil || exists {
		t.Fatalf("fresh store enrollment: exists=%v err=%v", exists, err)
	}
	record := validEnrollment()
	if err := repo.SaveEnrollment(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(store.Root, "2fa-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 0600", info.Mode().Perm())
	}
	loaded, exists, err := repo.LoadEnrollment(context.Background())
	if err != nil || !exists {
		t.Fatalf("load: exists=%v err=%v", exists, err)
	}
	if loaded != record {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
}

func TestTOTPEnrollmentMissingFileIsUnconfigured(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.LoadTOTPEnrollment(); err != nil || exists {
		t.Fatalf("absent file must be unconfigured: exists=%v err=%v", exists, err)
	}
}

func TestTOTPEnrollmentInvalidRecordsAreErrors(t *testing.T) {
	cases := map[string]string{
		"empty":          "",
		"whitespace":     "   ",
		"malformed":      "{\"formatVersion\":1",
		"extra document": `{"formatVersion":1}` + `{"formatVersion":1}`,
		"unknown field":  `{"formatVersion":1,"enrollmentId":"11111111-1111-4111-8111-111111111111","secret":"JBSWY3DPEHPK3PXP","lastAcceptedTimeStep":0,"extra":1}`,
		"future version": `{"formatVersion":2,"enrollmentId":"11111111-1111-4111-8111-111111111111","secret":"JBSWY3DPEHPK3PXP","lastAcceptedTimeStep":0}`,
		"short secret":   `{"formatVersion":1,"enrollmentId":"11111111-1111-4111-8111-111111111111","secret":"JBSW","lastAcceptedTimeStep":0}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(store.Root, "2fa-secret"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.LoadTOTPEnrollment(); err == nil {
				t.Fatal("invalid enrollment file was accepted")
			}
		})
	}
}

func TestTOTPEnrollmentRejectsWorldReadableFile(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Root, "2fa-secret")
	data, err := json.Marshal(validEnrollment())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadTOTPEnrollment(); err == nil {
		t.Fatal("world-readable enrollment file was accepted")
	}
}

func TestTOTPEnrollmentSaveRejectsInvalidRecords(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bad := validEnrollment()
	bad.Secret = "not base32!"
	if err := store.SaveTOTPEnrollment(bad); err == nil {
		t.Fatal("invalid record written to disk")
	}
	if _, err := os.Stat(filepath.Join(store.Root, "2fa-secret")); !os.IsNotExist(err) {
		t.Fatal("invalid save created a file")
	}
}

func TestStateLayoutDetectsLoneEnrollmentFile(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "state")
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "2fa-secret"), []byte(`{"formatVersion":1,"enrollmentId":"11111111-1111-4111-8111-111111111111","secret":"JBSWY3DPEHPK3PXP","lastAcceptedTimeStep":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Layout != LayoutPrivateChild {
		t.Fatalf("layout = %s, want private-child", store.Layout)
	}
	if filepath.Dir(filepath.Join(store.Root, "2fa-secret")) != inner {
		t.Fatalf("store root = %s, want %s", store.Root, inner)
	}
}
