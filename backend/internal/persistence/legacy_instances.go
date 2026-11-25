package persistence

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// LegacyInstance represents one active or audit record. An interrupted
// retirement can leave both records for the same ID.
type LegacyInstance struct {
	Metadata domain.ConnectionInstanceMeta
	Archived bool
	// Nil for metadata without a snapshot. Snapshot bytes stay on the source
	// and are read one instance at a time, under the recorded envelope digest.
	Header       *domain.SnapshotHeader
	snapshotPath string
	snapshotHash [sha256.Size]byte
	metadataHash [sha256.Size]byte
}

func (instance LegacyInstance) Snapshot() (*domain.SnapshotHeader, []byte, error) {
	if instance.Header == nil {
		return nil, nil, nil
	}
	data, err := readLegacyFile(instance.snapshotPath, SnapshotMaxSize+4096, true)
	if err != nil || sha256.Sum256(data) != instance.snapshotHash {
		return nil, nil, errors.New("legacy terminal snapshot changed after inspection")
	}
	header, body, err := DecodeSnapshot(data)
	if err != nil || header != *instance.Header {
		return nil, nil, errors.New("invalid legacy terminal snapshot")
	}
	return &header, body, nil
}

func (r legacyReader) readInstances(root string, out *LegacyData) error {
	// Keep the first nested projection's index for pair validation, and
	// continue to reject flat records that overlap either nested projection.
	seen := make(map[string]int)
	for _, archive := range []bool{false, true} {
		dir := filepath.Join(root, "connection-instances")
		if archive {
			dir = filepath.Join(root, "audit", "connection-instances")
		}
		entries, err := listLegacyDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			id := entry.Name()
			if !entry.IsDir() || !uuidPattern.MatchString(id) {
				return errors.New("unexpected legacy connection instance")
			}
			instance, err := r.readNestedInstance(filepath.Join(dir, id), id, archive)
			if err != nil {
				return err
			}
			if i, exists := seen[id]; exists {
				if err := checkLegacyArchivePair(out.Instances[i], instance); err != nil {
					return err
				}
			} else {
				seen[id] = len(out.Instances)
			}
			out.Instances = append(out.Instances, instance)
		}
	}
	return r.readFlatInstances(filepath.Join(root, "sessions"), seen, out)
}

// Legacy retirement copies metadata and any snapshot byte for byte before a
// separate active delete. Matching metadata and no conflicting snapshots make
// both projections safe to keep as found; don't infer which copy is newer.
func checkLegacyArchivePair(active, audit LegacyInstance) error {
	id := audit.Metadata.ID
	if active.Archived || !audit.Archived {
		return fmt.Errorf("duplicate legacy connection instance %s", id)
	}
	if active.metadataHash != audit.metadataHash {
		return fmt.Errorf("conflicting legacy active and audit metadata for connection instance %s; reconcile the backup before import", id)
	}
	if active.Header != nil && audit.Header != nil && active.snapshotHash != audit.snapshotHash {
		return fmt.Errorf("conflicting legacy active and audit snapshots for connection instance %s; reconcile the backup before import", id)
	}
	return nil
}

func (r legacyReader) readNestedInstance(dir, id string, archived bool) (LegacyInstance, error) {
	entries, err := listLegacyDir(dir)
	if err != nil {
		return LegacyInstance{}, err
	}
	for _, entry := range entries {
		if entry.Name() != "metadata.json" && entry.Name() != "terminal.snapshot" {
			return LegacyInstance{}, errors.New("unexpected legacy connection artifact; reconcile quarantined or incomplete files")
		}
	}
	data, err := r.read(filepath.Join(dir, "metadata.json"), legacyJSONMax, true)
	if err != nil {
		return LegacyInstance{}, errors.New("legacy connection instance metadata is missing or unreadable")
	}
	meta, err := decodeConnectionInstanceMeta(data)
	if err != nil || meta.ID != id || validateConnectionInstanceMeta(meta) != nil {
		return LegacyInstance{}, errors.New("invalid legacy connection instance metadata")
	}
	metadataHash := sha256.Sum256(data)
	instance, err := r.readLegacyInstance(meta, archived, filepath.Join(dir, "terminal.snapshot"))
	instance.metadataHash = metadataHash
	return instance, err
}

func (r legacyReader) readLegacyInstance(meta domain.ConnectionInstanceMeta, archived bool, snapshot string) (LegacyInstance, error) {
	result := LegacyInstance{Metadata: meta, Archived: archived}
	data, err := r.read(snapshot, SnapshotMaxSize+4096, true)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	header, _, decodeErr := DecodeSnapshot(data)
	if err != nil || decodeErr != nil {
		return LegacyInstance{}, errors.New("invalid legacy terminal snapshot")
	}
	result.Header = &header
	result.snapshotPath = snapshot
	result.snapshotHash = sha256.Sum256(data)
	return result, nil
}

func (r legacyReader) readFlatInstances(dir string, seen map[string]int, out *LegacyData) error {
	entries, err := listLegacyDir(dir)
	if err != nil {
		return err
	}
	files := make(map[string]bool, len(entries))
	for _, entry := range entries {
		files[entry.Name()] = true
	}
	for _, entry := range entries {
		name := entry.Name()
		id := strings.TrimSuffix(name, filepath.Ext(name))
		if !uuidPattern.MatchString(id) || !entry.Type().IsRegular() {
			return errors.New("unexpected legacy flat connection artifact")
		}
		if strings.HasSuffix(name, ".snapshot") && files[id+".json"] {
			continue
		}
		if _, exists := seen[id]; !strings.HasSuffix(name, ".json") || exists {
			return errors.New("orphan snapshot or duplicate legacy flat connection instance")
		}
		data, err := r.read(filepath.Join(dir, name), legacyJSONMax, true)
		if err != nil {
			return errors.New("legacy flat connection instance is unreadable")
		}
		meta, err := decodeFlatInstance(data)
		if err != nil || meta.ID != id {
			return errors.New("invalid legacy flat connection instance")
		}
		instance, err := r.readLegacyInstance(meta, false, filepath.Join(dir, id+".snapshot"))
		if err != nil {
			return err
		}
		seen[id] = len(out.Instances)
		out.Instances = append(out.Instances, instance)
	}
	return nil
}

func decodeFlatInstance(data []byte) (domain.ConnectionInstanceMeta, error) {
	var record legacySessionRecord
	if err := decodeStrict(data, &record); err != nil {
		return domain.ConnectionInstanceMeta{}, err
	}
	if (record.FormatVersion != LegacyConnectionFormatVersion && record.FormatVersion != PreviousConnectionFormatVersion) ||
		(record.ID != "" && record.ConnectionInstanceID != "" && record.ID != record.ConnectionInstanceID) {
		return domain.ConnectionInstanceMeta{}, errors.New("unsupported flat connection metadata")
	}
	if record.ID == "" {
		record.ID = record.ConnectionInstanceID
	}
	if record.AutomaticTitle == "" {
		record.AutomaticTitle = record.Title
	}
	meta := domain.ConnectionInstanceMeta{
		FormatVersion: ConnectionFormatVersion, ID: record.ID, BackendRuntimeID: record.BackendRuntimeID,
		ConnectionDefinitionID: record.ConnectionDefinitionID, Type: record.Type, Purpose: record.Purpose, SourceHostAlias: record.SourceHostAlias,
		Lifecycle: record.Lifecycle, SourceState: record.SourceState, AutomaticTitle: record.AutomaticTitle, TitleOverride: record.TitleOverride,
		InitialCwd: record.InitialCwd, Cwd: record.Cwd, Cols: record.Cols, Rows: record.Rows, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		ExitCode: record.ExitCode, ExitSignal: record.ExitSignal, ReuseFromConnectionInstanceID: record.ReuseFromConnectionInstanceID,
		GenerationStatus: record.GenerationStatus, GenerationError: record.GenerationError, TmuxEnabled: record.TmuxEnabled,
		TmuxSessionName: record.TmuxSessionName, TmuxPrefixKey: record.TmuxPrefixKey, TmuxPrefixSource: record.TmuxPrefixSource,
	}
	meta.SyncEffectiveTitle()
	return meta, validateConnectionInstanceMeta(meta)
}
