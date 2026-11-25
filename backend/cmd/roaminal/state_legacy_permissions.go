package main

import (
	"os"
	"regexp"
	"strings"
)

var (
	legacyInstanceName = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	legacyUploadName   = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}\.json$`)
)

// Legacy files are repaired only on explicit request. Once the database is
// authoritative, a leftover or unsafe legacy copy must not affect startup.
func preparePrivateLegacyFiles(state *os.Root) error {
	if err := prepareStateFiles(state, "definition-mutation.json"); err != nil {
		return err
	}
	if err := prepareLegacyRoot(state); err != nil {
		return err
	}
	return withPrivateStateSubdir(state, "state", prepareLegacyRoot)
}

func prepareLegacyRoot(root *os.Root) error {
	if err := prepareStateFiles(root, "2fa-secret", "auth-sessions.json", "messages.json", "agent-endpoints.json", "push-subscriptions.json", "notification-preferences.json"); err != nil {
		return err
	}
	if err := prepareLegacyInstances(root); err != nil {
		return err
	}
	if err := withPrivateStateSubdir(root, "audit", prepareLegacyInstances); err != nil {
		return err
	}
	if err := withPrivateStateSubdir(root, "sessions", func(flat *os.Root) error {
		return prepareLegacyEntries(flat, func(name string) error {
			for _, suffix := range []string{".json", ".snapshot"} {
				if id, ok := strings.CutSuffix(name, suffix); ok && legacyInstanceName.MatchString(id) {
					return prepareStateFiles(flat, name)
				}
			}
			return nil
		})
	}); err != nil {
		return err
	}
	return withPrivateStateSubdir(root, "uploads", func(uploads *os.Root) error {
		return prepareLegacyEntries(uploads, func(name string) error {
			if legacyUploadName.MatchString(name) {
				return prepareStateFiles(uploads, name)
			}
			return nil
		})
	})
}

func prepareLegacyInstances(root *os.Root) error {
	return withPrivateStateSubdir(root, "connection-instances", func(instances *os.Root) error {
		return prepareLegacyEntries(instances, func(name string) error {
			if !legacyInstanceName.MatchString(name) {
				return nil
			}
			return withPrivateStateSubdir(instances, name, func(instance *os.Root) error {
				return prepareStateFiles(instance, "metadata.json", "terminal.snapshot")
			})
		})
	})
}

func prepareLegacyEntries(root *os.Root, prepare func(string) error) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := prepare(name); err != nil {
			return err
		}
	}
	return nil
}
