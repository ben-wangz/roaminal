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
	"github.com/ben-wangz/roaminal/backend/internal/browser"
	"github.com/ben-wangz/roaminal/backend/internal/buildinfo"
	"github.com/ben-wangz/roaminal/backend/internal/clientdiag"
	"github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
	"github.com/ben-wangz/roaminal/backend/internal/frontend"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/imagepreview"
	"github.com/ben-wangz/roaminal/backend/internal/messages"
	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/notifications"
	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	args := os.Args[1:]
	if len(args) != 0 && (args[0] == "db" || args[0] == "admin") {
		if err := runMaintenance(ctx, args, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	cfg, err := config.Load(args)
	if err != nil {
		if errors.Is(err, config.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, cfg config.Config, store *db.Store, sshRoot *sshfs.Root) error {
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
	configRepo := sshconfig.New(sshRoot)
	keyInventory := sshkey.New(sshRoot)
	connectionOptions := store.ConnectionOptions()
	definitions, err := definition.NewWithRepository(configRepo, keyInventory, connectionOptions, store.DefinitionMutations())
	if err != nil {
		return err
	}
	authManager, err := auth.NewWithRepositories(cfg, store, auth.Dependencies{Clock: clockSource, IDs: idGenerator, Random: randomSource, Enrollment: store})
	if err != nil {
		return err
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := authManager.StartEnrollmentWatch(watchCtx, time.Second)
	defer func() {
		stopWatch()
		<-watchDone
	}()
	notificationService, err := notifications.New(store, idGenerator, notifications.Options{
		PublicKey: cfg.WebPushVAPIDPublicKey, PrivateKey: cfg.WebPushVAPIDPrivateKey, Subject: cfg.WebPushSubject,
		Clock: clockSource, PreferenceRepository: store, UserKey: authManager.Fingerprint(), SessionActive: authManager.IsSessionActive,
	})
	if err != nil {
		return err
	}
	defer notificationService.Close()
	if err := notificationService.Prune(context.Background()); err != nil {
		log.Printf("level=INFO event=web_push_startup_prune_failed error_type=%T", err)
	}
	terminalRuntime = terminal.NewManagerWithRepositories(cfg, terminal.Repositories{
		Instances: store, Audit: store, Snapshots: store, PersistenceDegraded: store.PersistenceDegraded,
		RetireConnectionInstance: store.RetireConnectionInstance,
	}, terminalWorker, clockSource, idGenerator, bootID)
	terminals := connection.NewManager(connection.Dependencies{
		Config: cfg, Runtime: terminalRuntime, Clock: clockSource, IDs: idGenerator, Random: randomSource,
		Definitions: definitions, Keys: keyInventory,
	})
	workerCtx, cancelWorker := context.WithTimeout(ctx, config.DefaultWorkerHandshake)
	err = terminalWorker.Start(workerCtx)
	cancelWorker()
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), config.DefaultShutdownDeadline)
		defer cancel()
		terminals.Shutdown(shutdownCtx)
	}()
	if err := terminals.Start(ctx); err != nil {
		return err
	}
	browserRuntime := browser.New(cfg)
	defer browserRuntime.Shutdown(context.Background())
	var diagnostics *clientdiag.Sink
	if cfg.ClientDiagnosticsEnabled {
		diagnostics = clientdiag.New(filepath.Join(cfg.StateDir, "diagnostics"), buildinfo.Version, bootID, log.Default(), clientdiag.Dependencies{Clock: clockSource, Random: randomSource})
		defer diagnostics.Close()
	}
	static, err := frontend.Handler(cfg.FrontendDir)
	if err != nil {
		return err
	}
	messageService := messages.NewWithNotifier(store, idGenerator, notificationService)
	agentService := agent.NewWithRepository(cfg, store.AgentRepository(), terminals, agent.Dependencies{Clock: clockSource, IDs: idGenerator, Random: randomSource, Messages: messageService})
	agentService.Start(ctx)
	defer agentService.Close()
	fileSystem := filesystem.NewWithRepositories(terminals, definitions, store, cfg.StateDir, filesystem.Dependencies{Clock: clockSource, Random: randomSource})
	imagePreview := imagepreview.New(imagepreview.Options{
		CacheDir:          cfg.FilesystemImagePreviewCacheDir,
		CacheTargetBytes:  mibBytes(cfg.FilesystemImagePreviewCacheTargetMiB),
		CacheMaxAge:       cfg.FilesystemImagePreviewCacheMaxAge,
		CleanupInterval:   cfg.FilesystemImagePreviewCacheCleanupInterval,
		MaxConversions:    cfg.FilesystemImagePreviewMaxConversions,
		MaxSourceBytes:    mibBytes(cfg.FilesystemImagePreviewMaxSourceMiB),
		MaxOutputBytes:    mibBytes(cfg.FilesystemImagePreviewMaxOutputMiB),
		MaxStaticPixels:   cfg.FilesystemImagePreviewMaxStaticPixels,
		MaxFrames:         cfg.FilesystemImagePreviewMaxFrames,
		MaxAnimatedPixels: cfg.FilesystemImagePreviewMaxAnimatedPixels,
		ConversionTimeout: cfg.FilesystemImagePreviewConversionTimeout,
		Logger:            log.Default(),
	})
	defer imagepreview.ShutdownVips()
	defer imagePreview.Close()
	serverDependencies := server.Dependencies{
		Config: cfg, Version: buildinfo.Version, BootID: bootID, Auth: authManager, Workspace: workspace.New(store),
		DatabaseReady: store.Ready,
		Connections:   terminals, Monitor: monitor.NewWithClock(clockSource), Worker: terminalWorker,
		Browser: browserRuntime,
		Static:  static, Definitions: definitions, Diagnostics: diagnostics,
		FileSystem:        fileSystem,
		ImagePreview:      imagePreview,
		AgentProvisioning: agentService.Provisioning(),
		AgentProjection:   agentService.Projection(),
		Messages:          messageService,
		Notifications:     notificationService,
		IDs:               idGenerator, Clock: clockSource,
	}
	if err := serverDependencies.Validate(); err != nil {
		return fmt.Errorf("build server: %w", err)
	}
	return serveHTTP(ctx, cfg, server.New(serverDependencies).Handler(), terminals.Fatal())
}

func serveHTTP(ctx context.Context, cfg config.Config, handler http.Handler, workerFailure <-chan error) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
	if err != nil {
		return fmt.Errorf("listen on %s:%d: %w", cfg.Host, cfg.Port, err)
	}
	httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 70 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 2 * time.Minute}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()
	var fatalErr error
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			fatalErr = err
		}
	case <-ctx.Done():
	case err := <-workerFailure:
		fatalErr = fmt.Errorf("terminal worker failed: %w", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.DefaultShutdownDeadline)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		_ = httpServer.Close()
	}
	return fatalErr
}

func mibBytes(value int) int64 {
	if value <= 0 {
		return 0
	}
	return int64(value) * 1024 * 1024
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
