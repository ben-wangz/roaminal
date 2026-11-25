package connection

import (
	"context"
	"errors"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
)

type runtimeDefinitionReader func(string, func(string) (string, error)) (definition.RuntimeSSH, error)

func (r runtimeDefinitionReader) ResolveSSH(alias string, probe func(string) (string, error)) (definition.RuntimeSSH, error) {
	return r(alias, probe)
}

func (runtimeDefinitionReader) NativeCollection() (sshconfig.Collection, error) {
	return sshconfig.Collection{}, errors.New("unexpected native collection read")
}

func TestRemoteLaunchDoesNotFallBackWhenDefinitionOptionsAreUnavailable(t *testing.T) {
	for _, reuse := range []string{"", "existing-instance"} {
		for _, launch := range []bool{false, true} {
			reads := 0
			manager := &Manager{
				sshPath: "not-invoked", runtimeDir: t.TempDir(),
				definitions: runtimeDefinitionReader(func(alias string, _ func(string) (string, error)) (definition.RuntimeSSH, error) {
					reads++
					if alias != "fixture" {
						t.Fatalf("unexpected alias: %q", alias)
					}
					return definition.RuntimeSSH{}, definition.ErrUnavailable
				}),
			}
			var err error
			if launch {
				_, err = manager.CreateRemoteLaunchOwned(context.Background(), sshconfig.DefinitionID("fixture"), 80, 24, reuse, "owner")
			} else {
				_, err = manager.CreateRemote(context.Background(), sshconfig.DefinitionID("fixture"), 80, 24, reuse)
			}
			if !errors.Is(err, definition.ErrUnavailable) || reads != 1 {
				t.Fatalf("launch=%v reuse=%q: definition reads=%d, error=%v", launch, reuse, reads, err)
			}
		}
	}
}

func TestPendingTmuxRequiresTheSameNativeRevisionAndOptions(t *testing.T) {
	expected := definition.RuntimeSSH{SourceRevision: "ssh-revision", Option: connectionoptions.Tmux{
		Enabled: true, SessionName: "work", Pwd: "~/project",
	}}
	current := expected
	manager := &Manager{definitions: runtimeDefinitionReader(func(string, func(string) (string, error)) (definition.RuntimeSSH, error) {
		return current, nil
	})}
	if !manager.tmuxLaunchCurrent("fixture", expected) {
		t.Fatal("matching definition did not allow promotion")
	}
	current.Option.Pwd = "~/different"
	if manager.tmuxLaunchCurrent("fixture", expected) {
		t.Fatal("launch started under an outdated Pwd was still current")
	}
	current = expected
	current.SourceRevision = "edited-ssh-revision"
	if manager.tmuxLaunchCurrent("fixture", expected) {
		t.Fatal("launch started under an outdated native source was still current")
	}
}
