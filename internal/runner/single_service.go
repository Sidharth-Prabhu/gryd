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

// RestartSingleService restarts only one named service in a running background project.
func RestartSingleService(projectName, serviceName string) error {
	projectState, err := state.LoadState(projectName)
	if err != nil {
		return fmt.Errorf("failed to load state for project '%s': %w", projectName, err)
	}

	cfg, err := config.Load(projectState.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to reload configuration: %w", err)
	}

	svc, exists := cfg.Services[serviceName]
	if !exists {
		return fmt.Errorf("service '%s' not found in configuration", serviceName)
	}

	// 1. Kill old process if running
	if oldProc, ok := projectState.Processes[serviceName]; ok && oldProc.PID > 0 {
		if state.IsProcessAlive(oldProc.PID) {
			_ = terminatePID(oldProc.PID)
			time.Sleep(150 * time.Millisecond)
			if state.IsProcessAlive(oldProc.PID) {
				_ = killPID(oldProc.PID)
			}
		}
	}

	// 2. Free port if needed
	if svc.Port > 0 && IsPortInUse(svc.Port) {
		_, _ = FreePort(svc.Port)
	}

	// 3. Open log file
	logFile, err := os.OpenFile(projectState.LogFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "\n=== [gryd] RELOADING SERVICE: %s (%s) ===\n", serviceName, time.Now().Format("15:04:05"))

	multiplexer := logger.NewLogMultiplexer(nil, logFile)

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

	outWriter := multiplexer.NewServiceWriter(serviceName)
	cmd.Stdout = outWriter
	cmd.Stderr = outWriter

	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart service '%s': %w", serviceName, err)
	}

	projectState.Processes[serviceName] = state.ProcessInfo{
		Name:    serviceName,
		PID:     cmd.Process.Pid,
		Command: svc.Command,
	}

	go func(c *exec.Cmd) {
		_ = c.Wait()
	}(cmd)

	return state.SaveState(projectState)
}
