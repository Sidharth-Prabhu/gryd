package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/logger"
)

// ServiceTestResult tracks test boot result of an individual service.
type ServiceTestResult struct {
	Name      string
	Command   string
	Port      int
	Ready     bool
	ExitError error
	Duration  time.Duration
}

// TestLogHook allows attaching interactive TUI models to receive live test outputs.
type TestHooks struct {
	OnLog          func(line string)
	OnServiceReady func(name string, d time.Duration)
}

// TestRun boots the project, verifies that all services start without crashing and
// their configured ports become active/listening (or commands complete successfully),
// then terminates all child processes and frees ports.
func TestRun(ctx context.Context, cfg *config.Config, timeout time.Duration) ([]ServiceTestResult, error) {
	return TestRunWithHooks(ctx, cfg, timeout, nil)
}

// TestRunWithHooks runs the test suite and forwards live events to optional hooks.
func TestRunWithHooks(ctx context.Context, cfg *config.Config, timeout time.Duration, hooks *TestHooks) ([]ServiceTestResult, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdoutWriter io.Writer
	if hooks != nil && hooks.OnLog != nil {
		stdoutWriter = &hookLogWriter{hook: hooks.OnLog}
	} else {
		stdoutWriter = os.Stdout
	}

	multiplexer := logger.NewLogMultiplexer(stdoutWriter, nil)

	type procItem struct {
		name      string
		svc       config.Service
		cmd       *exec.Cmd
		startTime time.Time
	}

	var procs []*procItem
	var procsMu sync.Mutex

	// Helper to shutdown all spawned test processes cleanly
	shutdown := func() {
		procsMu.Lock()
		defer procsMu.Unlock()
		for _, p := range procs {
			if p.cmd != nil && p.cmd.Process != nil {
				_ = killProcessGroup(p.cmd, syscall.SIGTERM)
			}
		}
		time.Sleep(300 * time.Millisecond)
		for _, p := range procs {
			if p.cmd != nil && p.cmd.Process != nil {
				_ = killProcessGroup(p.cmd, syscall.SIGKILL)
			}
		}
		// Also free configured ports
		for _, svc := range cfg.Services {
			if svc.Port > 0 {
				_, _ = FreePort(svc.Port)
			}
		}
		if len(cfg.Dependencies) > 0 {
			sm := NewSidecarManager(cfg.Name, cfg.Dependencies, nil)
			sm.runtime, _ = DetectContainerRuntime()
			sm.StopAll()
		}
	}
	defer shutdown()

	// Start sidecar dependencies if configured
	if len(cfg.Dependencies) > 0 {
		sm := NewSidecarManager(cfg.Name, cfg.Dependencies, nil)
		if err := sm.StartAll(testCtx); err != nil {
			return nil, fmt.Errorf("failed to start dependency sidecars: %w", err)
		}
	}

	// Launch each service
	results := make(map[string]*ServiceTestResult)
	type exitMsg struct {
		name string
		err  error
	}
	exitChan := make(chan exitMsg, len(cfg.Services))

	for name, svc := range cfg.Services {
		svcDir := cfg.WorkingDirectory
		if svc.Directory != "" {
			if filepath.IsAbs(svc.Directory) {
				svcDir = svc.Directory
			} else {
				svcDir = filepath.Join(cfg.WorkingDirectory, svc.Directory)
			}
		}

		if _, err := os.Stat(svcDir); os.IsNotExist(err) {
			_ = os.MkdirAll(svcDir, 0755)
		}

		// Ensure port is free before testing
		if svc.Port > 0 && IsPortInUse(svc.Port) {
			_, _ = FreePort(svc.Port)
		}

		cmd := shellCommand(svc.Command)
		cmd.Dir = svcDir
		cmd.Env = os.Environ()
		for k, v := range svc.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		outWriter := multiplexer.NewServiceWriter(name)
		cmd.Stdout = outWriter
		cmd.Stderr = outWriter

		setProcessGroup(cmd)

		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to start service '%s': %w", name, err)
		}

		item := &procItem{
			name:      name,
			svc:       svc,
			cmd:       cmd,
			startTime: time.Now(),
		}
		procsMu.Lock()
		procs = append(procs, item)
		procsMu.Unlock()

		results[name] = &ServiceTestResult{
			Name:    name,
			Command: svc.Command,
			Port:    svc.Port,
		}

		go func(sName string, c *exec.Cmd) {
			err := c.Wait()
			exitChan <- exitMsg{name: sName, err: err}
		}(name, cmd)
	}

	// Poll for readiness
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	readyStatus := make(map[string]bool)
	serviceBootTime := make(map[string]time.Duration)

	allReady := false

	for !allReady {
		select {
		case <-testCtx.Done():
			var unready []string
			for name := range cfg.Services {
				if !readyStatus[name] {
					unready = append(unready, name)
				}
			}
			return nil, fmt.Errorf("test run timed out after %s. Services not ready: %s", timeout, strings.Join(unready, ", "))

		case exit := <-exitChan:
			if exit.err != nil {
				results[exit.name].ExitError = exit.err
				return nil, fmt.Errorf("service '%s' failed prematurely during test run: %w", exit.name, exit.err)
			}
			readyStatus[exit.name] = true
			if _, set := serviceBootTime[exit.name]; !set {
				bootDur := time.Since(procs[0].startTime)
				serviceBootTime[exit.name] = bootDur
				if hooks != nil && hooks.OnServiceReady != nil {
					hooks.OnServiceReady(exit.name, bootDur)
				}
			}

		case <-ticker.C:
			for name, svc := range cfg.Services {
				if readyStatus[name] {
					continue
				}

				if svc.HealthCheck != nil {
					hOk, _ := CheckServiceHealth(testCtx, svc, nil)
					if hOk {
						readyStatus[name] = true
						bootDur := time.Since(procs[0].startTime)
						serviceBootTime[name] = bootDur
						if hooks != nil && hooks.OnServiceReady != nil {
							hooks.OnServiceReady(name, bootDur)
						} else {
							fmt.Printf("  \033[1;32m✓\033[0m Service '%s' passed readiness healthcheck (%s)\n",
								name, bootDur.Round(time.Millisecond))
						}
					}
				} else if svc.Port > 0 {
					if IsPortInUse(svc.Port) {
						readyStatus[name] = true
						bootDur := time.Since(procs[0].startTime)
						serviceBootTime[name] = bootDur
						if hooks != nil && hooks.OnServiceReady != nil {
							hooks.OnServiceReady(name, bootDur)
						} else {
							fmt.Printf("  \033[1;32m✓\033[0m Service '%s' listening on port %d (%s)\n",
								name, svc.Port, bootDur.Round(time.Millisecond))
						}
					}
				} else {
					if time.Since(procs[0].startTime) >= 2*time.Second {
						readyStatus[name] = true
						bootDur := time.Since(procs[0].startTime)
						serviceBootTime[name] = bootDur
						if hooks != nil && hooks.OnServiceReady != nil {
							hooks.OnServiceReady(name, bootDur)
						} else {
							fmt.Printf("  \033[1;32m✓\033[0m Service '%s' started and running stably (%s)\n",
								name, bootDur.Round(time.Millisecond))
						}
					}
				}
			}

			// Check if all are ready
			all := true
			for name := range cfg.Services {
				if !readyStatus[name] {
					all = false
					break
				}
			}
			if all {
				allReady = true
			}
		}
	}

	var output []ServiceTestResult
	for name, res := range results {
		res.Ready = true
		res.Duration = serviceBootTime[name]
		output = append(output, *res)
	}

	return output, nil
}

type hookLogWriter struct {
	hook func(line string)
}

func (w *hookLogWriter) Write(p []byte) (n int, err error) {
	if w.hook != nil {
		lines := strings.Split(string(p), "\n")
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				w.hook(l)
			}
		}
	}
	return len(p), nil
}
