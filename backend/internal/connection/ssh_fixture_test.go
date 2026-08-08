package connection

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type sshdFixture struct {
	config, control, marker, logPath string
}

func TestOpenSSHReuseDoesNotFallback(t *testing.T) {
	fixture := newSSHDFixture(t)
	base := []string{"-F", fixture.config, "-o", "BatchMode=yes", "-o", "ConnectTimeout=3", "-o", "ControlPath=" + fixture.control}
	owner := append(append([]string{}, base...), "-o", "ControlMaster=yes", "-o", "ControlPersist=60", "-o", "ProxyCommand=none", "fixture", "printf", "owner")
	if output, err := exec.Command("ssh", owner...).CombinedOutput(); err != nil {
		t.Fatalf("owner ssh: %v (%s)", err, output)
	}
	waitFor(t, func() bool { return fileExists(fixture.control) && fixture.connections() >= 1 })
	initial := fixture.connections()

	reuse := append(append([]string{}, base...), "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ProxyCommand=/bin/false", "fixture", "printf", "reuse")
	if output, err := exec.Command("ssh", reuse...).CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "reuse" {
		t.Fatalf("reuse ssh: %v (%s)", err, output)
	}
	waitFor(t, func() bool { return fixture.connections() == initial })

	if err := os.Remove(fixture.control); err != nil {
		t.Fatal(err)
	}
	markerBefore := readFile(fixture.marker)
	if output, err := exec.Command("ssh", reuse...).CombinedOutput(); err == nil {
		t.Fatalf("reuse without socket unexpectedly succeeded: %s", output)
	}
	if got := readFile(fixture.marker); got != markerBefore || fixture.connections() != initial {
		t.Fatalf("fallback changed server or marker: connections=%d marker=%q", fixture.connections(), got)
	}

	independent := append(append([]string{}, base...), "-o", "ControlMaster=no", "-o", "ControlPath=none", "-o", "ProxyCommand=none", "fixture", "printf", "independent")
	if output, err := exec.Command("ssh", independent...).CombinedOutput(); err != nil || !strings.HasSuffix(strings.TrimSpace(string(output)), "independent") {
		t.Fatalf("independent ssh: %v (%s)", err, output)
	}
	waitFor(t, func() bool { return fixture.connections() == initial+1 })
}

func newSSHDFixture(t *testing.T) *sshdFixture {
	t.Helper()
	sshdPath, err := exec.LookPath("sshd")
	if err != nil {
		t.Skip("sshd is unavailable")
	}
	dir := t.TempDir()
	hostKey := filepath.Join(dir, "host_ed25519")
	clientKey := filepath.Join(dir, "client_ed25519")
	runKeygen(t, hostKey)
	runKeygen(t, clientKey)
	public, err := os.ReadFile(clientKey + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	authorized := filepath.Join(dir, "authorized_keys")
	if err := os.WriteFile(authorized, public, 0o600); err != nil {
		t.Fatal(err)
	}
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	marker := filepath.Join(dir, "fallback.log")
	serverConfig := filepath.Join(dir, "sshd_config")
	configData := fmt.Sprintf("Port %d\nListenAddress 127.0.0.1\nHostKey %s\nAuthorizedKeysFile %s\nAllowUsers %s\nPermitRootLogin yes\nStrictModes no\nUsePAM no\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nPubkeyAuthentication yes\nAuthenticationMethods publickey\nPidFile none\nPrintMotd no\nLogLevel VERBOSE\n", port, hostKey, authorized, current.Username)
	if err := os.WriteFile(serverConfig, []byte(configData), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "sshd.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	process := exec.Command(sshdPath, "-D", "-e", "-f", serverConfig)
	process.Stdout, process.Stderr = logFile, logFile
	if err := process.Start(); err != nil {
		_ = logFile.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = process.Process.Kill()
		_ = process.Wait()
		_ = logFile.Close()
	})
	address := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		connection, dialErr := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			ready = true
			break
		}
		if process.ProcessState != nil {
			t.Fatalf("sshd exited: %s", readFile(logPath))
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("sshd fixture did not start: %s", readFile(logPath))
	}
	control := filepath.Join(dir, "control")
	config := filepath.Join(dir, "ssh_config")
	clientData := fmt.Sprintf("Host fixture\n  HostName 127.0.0.1\n  Port %d\n  User %s\n  IdentityFile %s\n  IdentitiesOnly yes\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n  ProxyCommand /bin/sh -c 'echo fallback >> %s; exit 1'\n", port, current.Username, clientKey, marker)
	if err := os.WriteFile(config, []byte(clientData), 0o600); err != nil {
		t.Fatal(err)
	}
	return &sshdFixture{config: config, control: control, marker: marker, logPath: logPath}
}

func runKeygen(t *testing.T, path string) {
	t.Helper()
	if output, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v (%s)", err, output)
	}
}

func (f *sshdFixture) connections() int {
	data, err := os.ReadFile(f.logPath)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "Connection from ")
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("fixture condition timed out")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFile(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}
