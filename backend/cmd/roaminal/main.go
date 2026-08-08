package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/buildinfo"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/frontend"
	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/server"
	"github.com/ben-wangz/roaminal/backend/internal/worker"
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
	store, err := persistence.NewConnection(cfg.StateDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Roaminal state layout=%s\n", store.Layout)
	authManager, err := auth.New(cfg, store)
	if err != nil {
		return err
	}
	workerPath := resolveWorkerPath(cfg.WorkerPath)
	if workerPath == "" {
		return fmt.Errorf("terminal worker not found")
	}
	bootID, err := randomID()
	if err != nil {
		return err
	}
	var terminals *connection.Manager
	terminalWorker := worker.New(workerPath, func(err error) {
		if terminals != nil {
			terminals.WorkerFatal(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultWorkerHandshake)
	defer cancel()
	if err := terminalWorker.Start(ctx); err != nil {
		return err
	}
	terminals = connection.NewManager(cfg, store, terminalWorker)
	terminals.SetRuntimeID(bootID)
	if err := terminals.Start(context.Background()); err != nil {
		_ = terminalWorker.Shutdown(context.Background())
		return err
	}
	static, err := frontend.Handler(cfg.FrontendDir)
	if err != nil {
		terminals.Shutdown(context.Background())
		return err
	}
	service := server.NewWithStatic(cfg, buildinfo.Version, bootID, authManager, terminals, monitor.New(), terminalWorker, static)
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
func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:]), nil
}
