package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/logger"
	"github.com/loom-cli/gryd/internal/state"
)

// HotReloadBackgroundProject performs an in-place hot reload on running services.
// If a process is running, sends a SIGHUP/SIGUSR2 or restarts services gracefully
// keeping existing sidecars alive and appending to logs.
func HotReloadBackgroundProject(projectName string) (*state.ProjectState, error) {
	st, err := state.LoadState(projectName)
	if err != nil {
		return nil, fmt.Errorf("project '%s' is not running: %w", projectName, err)
	}

	cfg, err := config.Load(st.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to reload config from %s: %w", st.ConfigPath, err)
	}

	logFile, err := os.OpenFile(st.LogFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "\n\n=== [gryd] HOT RELOADING SERVICES (%s) ===\n\n", time.Now().Format("2006-01-02 15:04:05"))
	multiplexer := logger.NewLogMultiplexer(nil, logFile)

	// Gracefully restart individual application services without wiping sidecars or project session
	for name := range cfg.Services {
		oldProc, exists := st.Processes[name]
		if exists && oldProc.PID > 0 && state.IsProcessAlive(oldProc.PID) {
			_ = terminatePID(oldProc.PID)
		}
	}

	// Short wait for graceful exit
	time.Sleep(200 * time.Millisecond)

	for name, svc := range cfg.Services {
		oldProc, exists := st.Processes[name]
		if exists && oldProc.PID > 0 && state.IsProcessAlive(oldProc.PID) {
			_ = killPID(oldProc.PID)
		}

		if svc.Port > 0 && IsPortInUse(svc.Port) {
			_, _ = FreePort(svc.Port)
		}

		svcDir := cfg.WorkingDirectory
		if svc.Directory != "" {
			if filepath.IsAbs(svc.Directory) {
				svcDir = svc.Directory
			} else {
				svcDir = filepath.Join(cfg.WorkingDirectory, svc.Directory)
			}
		}

		cmd := shellCommand(svc.Command)
		cmd.Dir = svcDir
		cmd.Env = BuildServiceEnv(cfg, svc.Env)

		outWriter := multiplexer.NewServiceWriter(name)
		cmd.Stdout = outWriter
		cmd.Stderr = outWriter

		setProcessGroup(cmd)

		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to hot reload service '%s': %w", name, err)
		}

		st.Processes[name] = state.ProcessInfo{
			Name:    name,
			PID:     cmd.Process.Pid,
			Command: svc.Command,
		}

		go func(c *exec.Cmd) {
			_ = c.Wait()
		}(cmd)
	}

	if err := state.SaveState(st); err != nil {
		return nil, fmt.Errorf("failed to save state: %w", err)
	}

	return st, nil
}
