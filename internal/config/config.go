package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/loom-cli/gryd/internal/state"
	"gopkg.in/yaml.v3"
)

// Service defines configuration for an individual service/process.
type Service struct {
	Directory   string             `yaml:"directory"`
	Command     string             `yaml:"command"`
	Port        int                `yaml:"port,omitempty"`
	DependsOn   DependsOnMap       `yaml:"depends_on,omitempty"`
	HealthCheck *HealthCheckConfig `yaml:"healthcheck,omitempty"`
	Env         map[string]string  `yaml:"env,omitempty"`
	Watch       []string           `yaml:"watch,omitempty"`
	Ignore      []string           `yaml:"ignore,omitempty"`
}

// Settings contains global execution options for the project.
type Settings struct {
	HealthCheck      bool `yaml:"health_check"`
	RestartOnFailure bool `yaml:"restart_on_failure"`
}

// Dependency defines configuration for an external infrastructure sidecar (e.g., PostgreSQL, Redis).
type Dependency struct {
	Image   string            `yaml:"image"`
	Ports   []string          `yaml:"ports,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Volumes []string          `yaml:"volumes,omitempty"`
	Command string            `yaml:"command,omitempty"`
}

// ProfileConfig defines customized overrides or subset of services for a named profile.
type ProfileConfig struct {
	Services     []string              `yaml:"services,omitempty"`     // Subset of services to run
	Dependencies []string              `yaml:"dependencies,omitempty"` // Subset of dependencies to run
	Env          map[string]string     `yaml:"env,omitempty"`          // Global env overrides
	EnvFile      string                `yaml:"env_file,omitempty"`     // Path to custom .env file
}

// Config represents the complete gryd.yaml (or legacy loom.yaml) configuration file.
type Config struct {
	Name             string                   `yaml:"name"`
	WorkingDirectory string                   `yaml:"working_directory"`
	Services         map[string]Service       `yaml:"services"`
	Dependencies     map[string]Dependency    `yaml:"dependencies,omitempty"`
	Profiles         map[string]ProfileConfig `yaml:"profiles,omitempty"`
	Settings         Settings                 `yaml:"settings"`

	// ActiveProfile is the profile applied to this configuration instance
	ActiveProfile string `yaml:"-"`

	// ConfigPath is the absolute path to the file this config was loaded from.
	ConfigPath string `yaml:"-"`
}

// Load reads and parses a gryd.yaml configuration file from the specified path.
func Load(path string) (*Config, error) {
	resolvedPath, err := resolveFilePath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path: %w", err)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", resolvedPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML configuration: %w", err)
	}

	cfg.ConfigPath = resolvedPath

	if err := cfg.ValidateAndResolve(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Register this project automatically into global registry
	_ = state.RegisterProject(cfg.Name, cfg.ConfigPath, cfg.WorkingDirectory)

	return &cfg, nil
}

// ValidateAndResolve validates the config values and resolves relative and home paths.
func (c *Config) ValidateAndResolve() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("project 'name' is required")
	}

	if len(c.Services) == 0 {
		return errors.New("at least one service must be defined under 'services'")
	}

	// Resolve working directory
	if c.WorkingDirectory == "" {
		// Default to the directory where the config file is located
		c.WorkingDirectory = filepath.Dir(c.ConfigPath)
	} else {
		resolvedWorkingDir, err := resolveFilePath(c.WorkingDirectory)
		if err != nil {
			return fmt.Errorf("invalid working_directory: %w", err)
		}
		c.WorkingDirectory = resolvedWorkingDir
	}

	// Validate each service
	for name, svc := range c.Services {
		if strings.TrimSpace(svc.Command) == "" {
			return fmt.Errorf("service '%s' must have a non-empty 'command'", name)
		}

		// Ensure dependencies exist
		for dep := range svc.DependsOn {
			if dep == name {
				return fmt.Errorf("service '%s' cannot depend on itself", name)
			}
			_, existsInSvc := c.Services[dep]
			_, existsInDeps := c.Dependencies[dep]
			if !existsInSvc && !existsInDeps {
				return fmt.Errorf("service '%s' depends on unknown service or dependency '%s'", name, dep)
			}
		}
	}

	// Validate each infrastructure dependency
	for depName, dep := range c.Dependencies {
		if strings.TrimSpace(dep.Image) == "" {
			return fmt.Errorf("dependency '%s' must specify an 'image'", depName)
		}
	}

	return nil
}

// FindProjectConfig discovers a config file:
// 1. If explicit specifiedPath is provided, use it.
// 2. If projectName is provided:
//    - Look in direct path/directory.
//    - Look in active runtime state (~/.gryd/run/<projectName>.json).
//    - Look in global project registry (~/.gryd/projects.json).
//    - Look in ~/.config/gryd/<projectName>.yaml or ~/.config/loom/.
// 3. If projectName is empty:
//    - Search current directory or walk up parent directories (like git repository root)
//      for gryd.yaml, gryd.yml, loom.yaml, loom.yml.
func FindConfigFile(specifiedPath string, projectName string) (string, error) {
	if specifiedPath != "" {
		resolved, err := resolveFilePath(specifiedPath)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(resolved); err != nil {
			return "", fmt.Errorf("specified config file '%s' does not exist", specifiedPath)
		}
		return resolved, nil
	}

	// If projectName is provided
	if projectName != "" {
		// 1. Direct path to a file
		if strings.HasSuffix(projectName, ".yaml") || strings.HasSuffix(projectName, ".yml") {
			if _, err := os.Stat(projectName); err == nil {
				return resolveFilePath(projectName)
			}
		}

		// 2. Direct directory containing config
		for _, fname := range []string{"gryd.yaml", "gryd.yml", "loom.yaml", "loom.yml"} {
			dirCfg := filepath.Join(projectName, fname)
			if _, err := os.Stat(dirCfg); err == nil {
				return resolveFilePath(dirCfg)
			}
		}

		// 3. Check active state (~/.gryd/run/<projectName>.json)
		if st, err := state.LoadState(projectName); err == nil && st.ConfigPath != "" {
			if _, err := os.Stat(st.ConfigPath); err == nil {
				return st.ConfigPath, nil
			}
		}

		// 4. Check global project registry (~/.gryd/projects.json)
		if entry, ok := state.FindProjectInRegistry(projectName); ok && entry.ConfigPath != "" {
			if _, err := os.Stat(entry.ConfigPath); err == nil {
				return entry.ConfigPath, nil
			}
		}

		// 5. Check user config directory ~/.config/gryd/<projectName>.yaml or ~/.config/loom/
		if home, err := os.UserHomeDir(); err == nil {
			for _, dirName := range []string{"gryd", "loom"} {
				userConfig := filepath.Join(home, ".config", dirName, projectName+".yaml")
				if _, err := os.Stat(userConfig); err == nil {
					return userConfig, nil
				}
				userConfigYml := filepath.Join(home, ".config", dirName, projectName+".yml")
				if _, err := os.Stat(userConfigYml); err == nil {
					return userConfigYml, nil
				}
			}
		}

		return "", fmt.Errorf("project '%s' was not found (not found locally, in active projects, or in registry)", projectName)
	}

	// If no projectName is provided, walk up from current working directory to find project root config
	cwd, err := os.Getwd()
	if err == nil {
		currentDir := cwd
		for {
			for _, candidate := range []string{"gryd.yaml", "gryd.yml", "loom.yaml", "loom.yml"} {
				candPath := filepath.Join(currentDir, candidate)
				if _, err := os.Stat(candPath); err == nil {
					return resolveFilePath(candPath)
				}
			}
			parent := filepath.Dir(currentDir)
			if parent == currentDir || parent == "" {
				break
			}
			currentDir = parent
		}
	}

	return "", errors.New("no gryd.yaml found in current or parent directories. Specify project name e.g. 'gryd run <project>' or run 'gryd init'")
}

// DetectCurrentProject attempts to detect the project in current directory or active state.
func DetectCurrentProject() (string, error) {
	configPath, err := FindConfigFile("", "")
	if err == nil {
		cfg, err := Load(configPath)
		if err == nil {
			return cfg.Name, nil
		}
	}
	return "", errors.New("no active project detected in current directory")
}

// resolveFilePath expands ~ home directory and cleans the path.
func resolveFilePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}

	return filepath.Abs(path)
}
