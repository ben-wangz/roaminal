package filesystem

import (
	"context"
	"errors"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type rootOptionReader func(map[string]bool) (connectionoptions.Collection, error)

func (read rootOptionReader) Load(aliases map[string]bool) (connectionoptions.Collection, error) {
	return read(aliases)
}

func TestConfiguredRootDistinguishesMissingOptionsFromFailedRead(t *testing.T) {
	alias := "fixture"
	executor := &fakeExecutor{summary: ports.ConnectionInstanceView{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	executor.run = func(command ports.RemoteCommand) ([]byte, error) {
		return rootOutput("/home/coder"), nil
	}
	unavailable := true
	reader := rootOptionReader(func(aliases map[string]bool) (connectionoptions.Collection, error) {
		if !aliases[alias] || len(aliases) != 1 {
			t.Fatalf("root looked up the wrong options: %v", aliases)
		}
		if unavailable {
			return connectionoptions.Collection{}, errors.New("injected SQL read failure")
		}
		return connectionoptions.Collection{Source: connectionoptions.Source{Status: "missing"}}, nil
	})
	service := New(executor, reader)
	if _, err := service.Root(context.Background(), "instance"); !errors.Is(err, connectionoptions.ErrOptionsUnavailable) {
		t.Fatalf("root during DB outage: %v", err)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("outage probed a different remote root: %+v", executor.calls)
	}
	unavailable = false
	root, err := service.Root(context.Background(), "instance")
	if err != nil || root.AbsolutePath != "/home/coder" || root.Source != "configured" {
		t.Fatalf("root with no stored override: %+v, %v", root, err)
	}
	if len(executor.calls) != 1 || len(executor.calls[0].Args) != 1 || executor.calls[0].Args[0] != connectionoptions.DefaultPwd {
		t.Fatalf("missing options did not use the default root: %+v", executor.calls)
	}
}
