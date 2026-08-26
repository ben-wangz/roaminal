package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/buildinfo"
	"github.com/ben-wangz/roaminal/backend/internal/clientdiag"
	"github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
	"github.com/ben-wangz/roaminal/backend/internal/frontend"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/messages"
	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"github.com/ben-wangz/roaminal/backend/internal/server"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
	"github.com/ben-wangz/roaminal/backend/internal/worker"
	"github.com/ben-wangz/roaminal/backend/internal/workspace"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Fprintln(os.Stdout, buildinfo.Version)
			return
		}
	}
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		if errors.Is(err, config.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	store, err := persistence.New(cfg.StateDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Roaminal state layout=%s\n", store.Layout)
	workerPath := resolveWorkerPath(cfg.WorkerPath)
	if workerPath == "" {
		return fmt.Errorf("terminal worker not found")
	}
	randomSource := random.CryptoSource{}
	clockSource := clock.System{}
	idGenerator := identity.UUIDGenerator{Random: randomSource}
	bootID, err := idGenerator.NewID()
	if err != nil {
		return err
	}
	var terminalRuntime *terminal.Manager
	terminalWorker := worker.New(workerPath, func(err error) {
		if terminalRuntime != nil {
			terminalRuntime.WorkerFatal(err)
		}
	}, worker.Dependencies{Clock: clockSource, IDs: idGenerator})
	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultWorkerHandshake)
	defer cancel()
	if err := terminalWorker.Start(ctx); err != nil {
		return err
	}
	sshRoot, sshErr := sshfs.Open()
	if sshErr != nil {
		fmt.Fprintf(os.Stderr, "Roaminal SSH source unavailable: %v\n", sshErr)
	}
	configRepo := sshconfig.New(sshRoot)
	keyInventory := sshkey.New(sshRoot)
	connectionOptions := connectionoptions.New(cfg.StateDir)
	definitions, err := definition.New(configRepo, keyInventory, connectionOptions)
	if err != nil {
		_ = terminalWorker.Shutdown(context.Background())
		return err
	}
	fileRepositories := persistence.NewRepositories(store)
	authManager, err := auth.NewWithRepositories(cfg, fileRepositories.Auth, auth.Dependencies{Clock: clockSource, IDs: idGenerator, Random: randomSource})
	if err != nil {
		_ = terminalWorker.Shutdown(context.Background())
		return err
	}
	terminalRuntime = terminal.NewManagerWithRepositories(cfg, terminal.Repositories{Instances: fileRepositories.Connection, Audit: fileRepositories.Audit, Snapshots: fileRepositories.TerminalSnapshots, PersistenceDegraded: store.PersistenceDegraded}, terminalWorker, clockSource, idGenerator, bootID)
	terminals := connection.NewManager(connection.Dependencies{
		Config: cfg, Runtime: terminalRuntime, Clock: clockSource, IDs: idGenerator, Random: randomSource,
		ConfigRepo: configRepo, Keys: keyInventory, Options: connectionOptions,
	})
	if err := terminals.Start(context.Background()); err != nil {
		_ = terminalWorker.Shutdown(context.Background())
		return err
	}
	var diagnostics *clientdiag.Sink
	if cfg.ClientDiagnosticsEnabled {
		diagnostics = clientdiag.New(store.DiagnosticsDir, buildinfo.Version, bootID, log.Default(), clientdiag.Dependencies{Clock: clockSource, Random: randomSource})
		defer diagnostics.Close()
	}
	static, err := frontend.Handler(cfg.FrontendDir)
	if err != nil {
		terminals.Shutdown(context.Background())
		return err
	}
	messageService := messages.New(fileRepositories.Messages, idGenerator)
	agentService := agent.NewWithRepository(cfg, agent.OpenStore(store.Root), terminals, agent.Dependencies{Clock: clockSource, IDs: idGenerator, Random: randomSource, Messages: messageService})
	serverDependencies := server.Dependencies{
		Config: cfg, Version: buildinfo.Version, BootID: bootID, Auth: authManager, Workspace: workspace.New(fileRepositories.Workspace),
		Connections: terminals, Monitor: monitor.NewWithClock(clockSource), Worker: terminalWorker,
		Static: static, Definitions: definitions, Diagnostics: diagnostics,
		FileSystem:        filesystem.NewWithRepositories(terminals, connectionOptions, fileRepositories.Upload, cfg.StateDir, filesystem.Dependencies{Clock: clockSource, Random: randomSource}),
		AgentProvisioning: agentService.Provisioning(),
		AgentTelemetry:    agentService.Telemetry(),
		Messages:          messageService,
		IDs:               idGenerator, Clock: clockSource,
	}
	if err := serverDependencies.Validate(); err != nil {
		terminals.Shutdown(context.Background())
		_ = terminalWorker.Shutdown(context.Background())
		return fmt.Errorf("build server: %w", err)
	}
	service := server.New(serverDependencies)
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
	if err != nil {
		_ = terminalWorker.Shutdown(context.Background())
		return fmt.Errorf("listen on %s:%d: %w", cfg.Host, cfg.Port, err)
	}
	httpServer := &http.Server{Handler: service.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 70 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 2 * time.Minute}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var fatalErr error
	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			fatalErr = err
		}
	case <-signalCtx.Done():
	case err := <-terminals.Fatal():
		fatalErr = fmt.Errorf("terminal worker failed: %w", err)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), config.DefaultShutdownDeadline)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	terminals.Shutdown(shutdownCtx)
	if fatalErr != nil {
		return fatalErr
	}
	return nil
}

func resolveWorkerPath(configured string) string {
	paths := []string{configured, os.Getenv("ROAMINAL_WORKER_PATH"), filepath.Join(mustWD(), "terminal-worker", "src", "index.mjs"), filepath.Join(mustWD(), "..", "terminal-worker", "src", "index.mjs"), "/opt/roaminal/terminal-worker/src/index.mjs"}
	for _, path := range paths {
		if path != "" {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}
func mustWD() string { path, _ := os.Getwd(); return path }
