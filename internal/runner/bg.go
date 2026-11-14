package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/logger"
	"github.com/loom-cli/gryd/internal/state"
)

// StopProjectProcesses kills all running processes associated with a project state,
// and terminates any containerized sidecars.
func StopProjectProcesses(st *state.ProjectState) {
	for name, proc := range st.Processes {
		if proc.PID > 0 && state.IsProcessAlive(proc.PID) {
			_ = terminatePID(proc.PID)
		}
		_ = name
	}

	// Give up to 500ms for graceful exit
	time.Sleep(500 * time.Millisecond)

	for _, proc := range st.Processes {
		if proc.PID > 0 && state.IsProcessAlive(proc.PID) {
			_ = killPID(proc.PID)
		}
	}

	// Clean up sidecars if project config is accessible
	if st != nil && st.ConfigPath != "" {
		if cfg, err := config.Load(st.ConfigPath); err == nil && len(cfg.Dependencies) > 0 {
			sm := NewSidecarManager(cfg.Name, cfg.Dependencies, nil)
			sm.runtime, _ = DetectContainerRuntime()
			sm.StopAll()
		}
	}
}

// ReloadBackgroundProject reloads a background project: stops old processes,
// ensures ports are freed, re-reads config, restarts services, updates ~/.gryd/run/<project>.json,
// and appends new output to log file.
func ReloadBackgroundProject(projectName string) (*state.ProjectState, error) {
	st, _ := state.LoadState(projectName)

	// Resolve config path
	var configPath string
	if st != nil && st.ConfigPath != "" {
		configPath = st.ConfigPath
	} else {
		foundPath, err := config.FindConfigFile("", projectName)
		if err != nil {
			return nil, err
		}
		configPath = foundPath
	}

	// 1. Stop current processes if active
	if st != nil {
		StopProjectProcesses(st)
	}

	// 2. Re-read config
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to reload config from %s: %w", configPath, err)
	}

	// 3. Ensure configured ports are free before restarting
	for _, svc := range cfg.Services {
		if svc.Port > 0 && IsPortInUse(svc.Port) {
			_, _ = FreePort(svc.Port)
		}
	}

	// 3b. Start sidecars if defined
	if len(cfg.Dependencies) > 0 {
		sm := NewSidecarManager(cfg.Name, cfg.Dependencies, nil)
		_ = sm.StartAll(context.Background())
	}

	// 4. Open log file
	logFilePath, err := state.GetLogFilePath(cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to determine log file path: %w", err)
	}

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	fmt.Fprintf(logFile, "\n\n=== [gryd] RELOADING SERVICES (%s) ===\n\n", time.Now().Format("2006-01-02 15:04:05"))

	multiplexer := logger.NewLogMultiplexer(nil, logFile)

	newState := &state.ProjectState{
		ProjectName: cfg.Name,
		ConfigPath:  configPath,
		LogFilePath: logFilePath,
		StartedAt:   time.Now(),
		Processes:   make(map[string]state.ProcessInfo),
	}

	tiers, err := BuildStartupPlan(cfg.Services)
	if err != nil {
		return nil, err
	}

	for tierIdx, tier := range tiers {
		for _, name := range tier {
			svc := cfg.Services[name]
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
				return nil, fmt.Errorf("failed to restart service '%s': %w", name, err)
			}

			newState.Processes[name] = state.ProcessInfo{
				Name:    name,
				PID:     cmd.Process.Pid,
				Command: svc.Command,
			}

			go func(c *exec.Cmd) {
				_ = c.Wait()
			}(cmd)
		}

		// Wait for health if future tiers depend on services in this tier
		if tierIdx < len(tiers)-1 {
			for _, name := range tier {
				svc := cfg.Services[name]
				needsHealthy := false
				for futureIdx := tierIdx + 1; futureIdx < len(tiers); futureIdx++ {
					for _, futureName := range tiers[futureIdx] {
						futureSvc := cfg.Services[futureName]
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
					waitCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					_ = WaitForServiceHealth(waitCtx, name, svc, nil)
					cancel()
				}
			}
		}
	}

	if err := state.SaveState(newState); err != nil {
		return nil, fmt.Errorf("failed to save state: %w", err)
	}

	return newState, nil
}
