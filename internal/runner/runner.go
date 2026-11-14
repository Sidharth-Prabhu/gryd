package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/logger"
	"github.com/loom-cli/gryd/internal/proxy"
	"github.com/loom-cli/gryd/internal/state"
	"go.uber.org/zap"
	"golang.org/x/term"
)

// ProcessState tracks an individual active service.
type ProcessState struct {
	Name    string
	Service config.Service
	Cmd     *exec.Cmd
	Dir     string
	Started time.Time
}

// Runner supervises and manages concurrent service execution, hot reloads,
// and background detaching with a pinned footer.
type Runner struct {
	cfg         *config.Config
	log         *zap.Logger
	multiplexer *logger.LogMultiplexer
	logFile     *os.File
	logPath     string
	processes   map[string]*ProcessState
	sidecars    *SidecarManager
	footerMgr   *PinnedFooterManager
	watcher     *FileWatcher
	ipcServer   *IPCServer
	proxyServer *proxy.ReverseProxyServer
	mu          sync.Mutex
	stopOnce    sync.Once
}

// New creates a new Runner instance.
func New(cfg *config.Config, log *zap.Logger) (*Runner, error) {
	logPath, err := state.GetLogFilePath(cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to determine log file path: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logPath, err)
	}

	footerText := "\033[1;36m'r'\033[0m reload  |  \033[1;31m'R'\033[0m hard restart  |  \033[1;32m'c'\033[0m detach  |  \033[2mCtrl+C quit\033[0m"
	footerMgr := NewPinnedFooterManager(int(os.Stdout.Fd()), footerText)

	// Wrap stdout so any write to terminal also re-draws the pinned footer
	termWriter := &footerLogWriter{
		footerMgr: footerMgr,
		out:       os.Stdout,
	}

	multiplexer := logger.NewLogMultiplexer(termWriter, logFile)

	sidecars := NewSidecarManager(cfg.Name, cfg.Dependencies, log)

	r := &Runner{
		cfg:         cfg,
		log:         log,
		multiplexer: multiplexer,
		logFile:     logFile,
		logPath:     logPath,
		processes:   make(map[string]*ProcessState),
		sidecars:    sidecars,
		footerMgr:   footerMgr,
	}

	fw, err := NewFileWatcher(cfg, log, func(serviceName string) error {
		return r.RestartService(serviceName)
	})
	if err == nil {
		r.watcher = fw
	}

	// Initialize Unix Domain Socket IPC Server
	if ipc, err := NewIPCServer(cfg.Name, r, log); err == nil {
		r.ipcServer = ipc
		r.ipcServer.Start()
	}

	// Initialize local domain reverse proxy
	proxyServer := proxy.NewReverseProxyServer(cfg, log)
	if err := proxyServer.Start(); err == nil {
		r.proxyServer = proxyServer
		// Attempt to sync /etc/hosts for any non-.localhost domains
		_ = proxy.SyncHostsFile(proxyServer.GetDomains())
	}

	return r, nil
}

type footerLogWriter struct {
	footerMgr *PinnedFooterManager
	out       *os.File
}

func (w *footerLogWriter) Write(p []byte) (n int, err error) {
	return w.footerMgr.WriteToScrollRegion(w.out, p)
}

// Run launches all services concurrently and enters the interactive event loop.
func (r *Runner) Run(ctx context.Context) error {
	defer func() {
		if r.logFile != nil {
			_ = r.logFile.Sync()
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	defer func() {
		if r.ipcServer != nil {
			_ = r.ipcServer.Close()
		}
		if r.proxyServer != nil {
			_ = r.proxyServer.Close()
		}
	}()

	// Channels for interactive keypress events
	reloadChan := make(chan struct{}, 1)
	hardRestartChan := make(chan struct{}, 1)
	detachChan := make(chan struct{}, 1)

	// Initialize terminal scrolling region and pin footer to bottom
	r.footerMgr.Init()
	defer r.footerMgr.Reset()

	// Start services initially
	if err := r.startServices(); err != nil {
		return err
	}

	// Start file watcher if configured
	if r.watcher != nil {
		_ = r.watcher.Start(ctx)
		defer r.watcher.Close()
	}

	// Enable raw terminal mode if stdout is an interactive terminal
	stopRaw := r.setupKeyboardListener(reloadChan, hardRestartChan, detachChan)
	defer stopRaw()

	for {
		select {
		case sig := <-sigChan:
			stopRaw()
			r.footerMgr.Reset()
			fmt.Printf("\nReceived %s. Initiating graceful shutdown...\n", sig)
			r.shutdownAll()
			_ = state.RemoveState(r.cfg.Name)
			return nil

		case <-reloadChan:
			r.footerMgr.Reset()
			fmt.Println("\n\033[1;33m[gryd] Hot reloading project services...\033[0m")
			r.restartServices()
			r.footerMgr.Init()

		case <-hardRestartChan:
			r.footerMgr.Reset()
			fmt.Println("\n\033[1;31m[gryd] Hard restarting all processes & ports...\033[0m")
			r.shutdownAll()
			for _, svc := range r.cfg.Services {
				if svc.Port > 0 && IsPortInUse(svc.Port) {
					_, _ = FreePort(svc.Port)
				}
			}
			_ = r.startServices()
			r.footerMgr.Init()

		case <-detachChan:
			stopRaw()
			r.footerMgr.Reset()
			fmt.Println("\n\033[1;32m[gryd] Running in background.\033[0m")
			fmt.Printf("Services will continue running.\nView live logs at any time using: \033[1mgryd look %s\033[0m\n", r.cfg.Name)
			return nil

		case <-ctx.Done():
			stopRaw()
			r.footerMgr.Reset()
			r.shutdownAll()
			_ = state.RemoveState(r.cfg.Name)
			return nil
		}
	}
}

// setupKeyboardListener reads single keystrokes from stdin in raw mode.
func (r *Runner) setupKeyboardListener(reloadChan, hardRestartChan, detachChan chan struct{}) func() {
	stdinFd := int(os.Stdin.Fd())
	if !term.IsTerminal(stdinFd) {
		return func() {}
	}

	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return func() {}
	}

	stopChan := make(chan struct{})
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			close(stopChan)
			_ = term.Restore(stdinFd, oldState)
		})
	}

	go func() {
		buf := make([]byte, 1)
		for {
			select {
			case <-stopChan:
				return
			default:
				n, err := os.Stdin.Read(buf)
				if err != nil || n == 0 {
					return
				}
				b := buf[0]
				switch b {
				case 'r':
					select {
					case reloadChan <- struct{}{}:
					default:
					}
				case 'R':
					select {
					case hardRestartChan <- struct{}{}:
					default:
					}
				case 'c', 'C':
					select {
					case detachChan <- struct{}{}:
					default:
					}
				case 3: // Ctrl+C
					p, _ := os.FindProcess(os.Getpid())
					_ = p.Signal(os.Interrupt)
					return
				}
			}
		}
	}()

	return cleanup
}

// startServices launches all configured services.
func (r *Runner) startServices() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopOnce = sync.Once{}
	r.processes = make(map[string]*ProcessState)

	r.log.Info("Starting Gryd project",
		zap.String("project", r.cfg.Name),
		zap.String("working_dir", r.cfg.WorkingDirectory),
		zap.Int("services_count", len(r.cfg.Services)),
	)

	stateInfo := &state.ProjectState{
		ProjectName: r.cfg.Name,
		ConfigPath:  r.cfg.ConfigPath,
		LogFilePath: r.logPath,
		StartedAt:   time.Now(),
		Processes:   make(map[string]state.ProcessInfo),
	}

	// Start declared infrastructure sidecars first
	if r.sidecars != nil {
		if err := r.sidecars.StartAll(context.Background()); err != nil {
			r.log.Warn("Failed to start some sidecar dependencies", zap.Error(err))
		}
	}

	tiers, err := BuildStartupPlan(r.cfg.Services)
	if err != nil {
		return err
	}

	for tierIdx, tier := range tiers {
		r.log.Info("Starting service tier", zap.Int("tier", tierIdx+1), zap.Strings("services", tier))

		// 1. Start all services in current tier
		for _, name := range tier {
			svc := r.cfg.Services[name]
			svcDir := r.cfg.WorkingDirectory
			if svc.Directory != "" {
				if filepath.IsAbs(svc.Directory) {
					svcDir = svc.Directory
				} else {
					svcDir = filepath.Join(r.cfg.WorkingDirectory, svc.Directory)
				}
			}

			if _, err := os.Stat(svcDir); os.IsNotExist(err) {
				_ = os.MkdirAll(svcDir, 0755)
			}

			cmd := shellCommand(svc.Command)
			cmd.Dir = svcDir
			cmd.Env = os.Environ()
			for k, v := range svc.Env {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}

			outWriter := r.multiplexer.NewServiceWriter(name)
			cmd.Stdout = outWriter
			cmd.Stderr = outWriter

			setProcessGroup(cmd)

			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start service '%s': %w", name, err)
			}

			procState := &ProcessState{
				Name:    name,
				Service: svc,
				Cmd:     cmd,
				Dir:     svcDir,
				Started: time.Now(),
			}
			r.processes[name] = procState

			stateInfo.Processes[name] = state.ProcessInfo{
				Name:    name,
				PID:     cmd.Process.Pid,
				Command: svc.Command,
			}

			r.log.Info("Service started",
				zap.String("service", name),
				zap.Int("pid", cmd.Process.Pid),
				zap.String("command", svc.Command),
			)
		}

		// 2. If subsequent tiers depend on any service in this tier with condition: service_healthy, wait for readiness
		if tierIdx < len(tiers)-1 {
			for _, name := range tier {
				svc := r.cfg.Services[name]
				// Check if any service in a future tier depends on this service with condition == service_healthy
				needsHealthy := false
				for futureIdx := tierIdx + 1; futureIdx < len(tiers); futureIdx++ {
					for _, futureName := range tiers[futureIdx] {
						futureSvc := r.cfg.Services[futureName]
						if depCond, ok := futureSvc.DependsOn[name]; ok {
							if depCond.Condition == "service_healthy" || svc.HealthCheck != nil {
								needsHealthy = true
								break
							}
						}
					}
					if needsHealthy {
						break
					}
				}

				if needsHealthy {
					r.log.Info("Waiting for dependency to become healthy", zap.String("service", name))
					waitCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					err := WaitForServiceHealth(waitCtx, name, svc, r.log)
					cancel()
					if err != nil {
						r.log.Warn("Dependency healthcheck timed out or failed", zap.String("service", name), zap.Error(err))
					}
				}
			}
		}
	}

	// Persist active state to ~/.gryd/run/<project>.json
	if err := state.SaveState(stateInfo); err != nil {
		r.log.Warn("Failed to persist running state", zap.Error(err))
	}

	return nil
}

// RestartService cleanly terminates and restarts a single service process.
func (r *Runner) RestartService(name string) error {
	r.mu.Lock()
	proc, exists := r.processes[name]
	svc, svcExists := r.cfg.Services[name]
	if !svcExists {
		r.mu.Unlock()
		return fmt.Errorf("service '%s' not configured", name)
	}

	if exists && proc.Cmd != nil && proc.Cmd.Process != nil {
		r.log.Info("Restarting service", zap.String("service", name), zap.Int("pid", proc.Cmd.Process.Pid))
		_ = killProcessGroup(proc.Cmd, syscall.SIGTERM)
		time.Sleep(100 * time.Millisecond)
		if state.IsProcessAlive(proc.Cmd.Process.Pid) {
			_ = killProcessGroup(proc.Cmd, syscall.SIGKILL)
		}
	}
	delete(r.processes, name)
	r.mu.Unlock()

	// Port release if defined
	if svc.Port > 0 && IsPortInUse(svc.Port) {
		_, _ = FreePort(svc.Port)
	}

	svcDir := r.cfg.WorkingDirectory
	if svc.Directory != "" {
		if filepath.IsAbs(svc.Directory) {
			svcDir = svc.Directory
		} else {
			svcDir = filepath.Join(r.cfg.WorkingDirectory, svc.Directory)
		}
	}

	cmd := shellCommand(svc.Command)
	cmd.Dir = svcDir
	cmd.Env = os.Environ()
	for k, v := range svc.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	outWriter := r.multiplexer.NewServiceWriter(name)
	cmd.Stdout = outWriter
	cmd.Stderr = outWriter

	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		r.log.Error("Failed to restart service", zap.String("service", name), zap.Error(err))
		return err
	}

	r.mu.Lock()
	r.processes[name] = &ProcessState{
		Name:    name,
		Service: svc,
		Cmd:     cmd,
		Dir:     svcDir,
		Started: time.Now(),
	}
	r.mu.Unlock()

	// Update persisted project state
	if st, err := state.LoadState(r.cfg.Name); err == nil {
		st.Processes[name] = state.ProcessInfo{
			Name:    name,
			PID:     cmd.Process.Pid,
			Command: svc.Command,
		}
		_ = state.SaveState(st)
	}

	return nil
}

// restartServices stops all current processes and starts them anew.
func (r *Runner) restartServices() {
	r.shutdownAll()
	if err := r.startServices(); err != nil {
		r.log.Error("Failed to restart services", zap.Error(err))
	}
}

// shutdownAll cleanly terminates all active service processes.
func (r *Runner) shutdownAll() {
	r.stopOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		if len(r.processes) == 0 {
			return
		}

		for name, proc := range r.processes {
			if proc.Cmd != nil && proc.Cmd.Process != nil {
				r.log.Debug("Sending SIGTERM to process group",
					zap.String("service", name),
					zap.Int("pid", proc.Cmd.Process.Pid),
				)
				_ = killProcessGroup(proc.Cmd, syscall.SIGTERM)
			}
		}

		done := make(chan struct{})
		go func() {
			for _, proc := range r.processes {
				if proc.Cmd != nil && proc.Cmd.Process != nil {
					_, _ = proc.Cmd.Process.Wait()
				}
			}
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			for _, proc := range r.processes {
				if proc.Cmd != nil && proc.Cmd.Process != nil {
					_ = killProcessGroup(proc.Cmd, syscall.SIGKILL)
				}
			}
		}

		if r.sidecars != nil {
			r.sidecars.StopAll()
		}

		r.processes = make(map[string]*ProcessState)
	})
}
