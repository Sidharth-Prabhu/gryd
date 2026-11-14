package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AutoDetectStack scans the given root directory and subdirectories, identifying
// frameworks, commands, default ports, and docker-compose sidecars.
func AutoDetectStack(rootDir string) (*Config, error) {
	if rootDir == "" {
		rootDir = "."
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}

	projName := filepath.Base(absRoot)
	if projName == "." || projName == "/" || projName == "" {
		projName = "my-project"
	}

	cfg := &Config{
		Name:             projName,
		WorkingDirectory: ".",
		Services:         make(map[string]Service),
		Dependencies:     make(map[string]Dependency),
		Settings: Settings{
			HealthCheck:      true,
			RestartOnFailure: false,
		},
	}

	// 1. Scan root and immediate subdirectories for services
	scanDirForServices(absRoot, "", cfg)

	// Scan 1 level deep (e.g. backend/, frontend/, api/, web/, services/*)
	entries, _ := os.ReadDir(absRoot)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subName := entry.Name()
		if isIgnoredDir(subName) {
			continue
		}
		subPath := filepath.Join(absRoot, subName)
		scanDirForServices(subPath, subName, cfg)
	}

	// 2. Scan for docker-compose.yml / docker-compose.yaml to import sidecars
	importDockerCompose(absRoot, cfg)

	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("no recognizable project stack detected in '%s' (no package.json, pyproject.toml, go.mod, Cargo.toml)", rootDir)
	}

	// Wire dependencies: frontends depend on backends if both exist
	var hasBackend, hasFrontend bool
	for sName := range cfg.Services {
		lower := strings.ToLower(sName)
		if strings.Contains(lower, "backend") || strings.Contains(lower, "api") || strings.Contains(lower, "server") {
			hasBackend = true
		}
		if strings.Contains(lower, "frontend") || strings.Contains(lower, "web") || strings.Contains(lower, "client") {
			hasFrontend = true
		}
	}

	if hasBackend && hasFrontend {
		for sName, s := range cfg.Services {
			lower := strings.ToLower(sName)
			if strings.Contains(lower, "frontend") || strings.Contains(lower, "web") || strings.Contains(lower, "client") {
				for bName := range cfg.Services {
					bLower := strings.ToLower(bName)
					if strings.Contains(bLower, "backend") || strings.Contains(bLower, "api") || strings.Contains(bLower, "server") {
						if s.DependsOn == nil {
							s.DependsOn = make(DependsOnMap)
						}
						s.DependsOn[bName] = DepCondition{}
						cfg.Services[sName] = s
						break
					}
				}
			}
		}
	}

	return cfg, nil
}

func isIgnoredDir(name string) bool {
	return strings.HasPrefix(name, ".") ||
		name == "node_modules" ||
		name == "vendor" ||
		name == "dist" ||
		name == "build" ||
		name == "target" ||
		name == "bin" ||
		name == "pkg"
}

func scanDirForServices(dir, relDir string, cfg *Config) {
	// A. Check Node / JavaScript / TypeScript (package.json)
	pkgPath := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		svc := detectNodeService(dir, data, relDir)
		name := relDir
		if name == "" {
			name = "web"
		}
		name = sanitizeServiceName(name, cfg.Services)
		cfg.Services[name] = svc
		return
	}

	// B. Check Python (pyproject.toml, requirements.txt, Pipfile)
	if hasFile(dir, "pyproject.toml") || hasFile(dir, "requirements.txt") || hasFile(dir, "Pipfile") {
		svc := detectPythonService(dir, relDir)
		name := relDir
		if name == "" {
			name = "api"
		}
		name = sanitizeServiceName(name, cfg.Services)
		cfg.Services[name] = svc
		return
	}

	// C. Check Go (go.mod)
	if hasFile(dir, "go.mod") {
		svc := Service{
			Directory: relDir,
			Command:   "go run .",
			Watch:     []string{"**/*.go"},
			Ignore:    []string{"*_test.go"},
		}
		name := relDir
		if name == "" {
			name = "backend"
		}
		name = sanitizeServiceName(name, cfg.Services)
		cfg.Services[name] = svc
		return
	}

	// D. Check Rust (Cargo.toml)
	if hasFile(dir, "Cargo.toml") {
		svc := Service{
			Directory: relDir,
			Command:   "cargo run",
			Watch:     []string{"src/**"},
		}
		name := relDir
		if name == "" {
			name = "backend"
		}
		name = sanitizeServiceName(name, cfg.Services)
		cfg.Services[name] = svc
		return
	}
}

func detectNodeService(dir string, pkgJSON []byte, relDir string) Service {
	var pkg struct {
		Scripts      map[string]string `json:"scripts"`
		Dependencies map[string]string `json:"dependencies"`
		DevDeps      map[string]string `json:"devDependencies"`
	}
	_ = json.Unmarshal(pkgJSON, &pkg)

	// Detect package manager (bun > pnpm > yarn > npm)
	pm := "npm"
	if hasFile(dir, "bun.lockb") || hasFile(dir, "bun.lock") {
		pm = "bun"
	} else if hasFile(dir, "pnpm-lock.yaml") {
		pm = "pnpm"
	} else if hasFile(dir, "yarn.lock") {
		pm = "yarn"
	}

	cmd := fmt.Sprintf("%s run dev", pm)
	if _, ok := pkg.Scripts["dev"]; !ok {
		if _, ok := pkg.Scripts["start"]; ok {
			cmd = fmt.Sprintf("%s start", pm)
		}
	}

	port := 3000
	// Detect framework
	allDeps := make(map[string]string)
	for k, v := range pkg.Dependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.DevDeps {
		allDeps[k] = v
	}

	if _, ok := allDeps["vite"]; ok {
		port = 5173
	} else if _, ok := allDeps["@nestjs/core"]; ok {
		port = 3000
	} else if _, ok := allDeps["express"]; ok {
		port = 8000
	}

	return Service{
		Directory: relDir,
		Command:   cmd,
		Port:      port,
		Watch:     []string{"src/**", "pages/**", "app/**"},
		Ignore:    []string{"node_modules/**", ".next/**", "dist/**"},
	}
}

func detectPythonService(dir, relDir string) Service {
	cmd := "python main.py"
	port := 8000

	if hasFile(dir, "manage.py") {
		cmd = "python manage.py runserver"
		port = 8000
	} else if hasFile(dir, "app.py") {
		cmd = "uvicorn app:app --reload"
		port = 8000
	} else if hasFile(dir, "pyproject.toml") {
		cmd = "uv run fastapi dev"
		port = 8000
	}

	return Service{
		Directory: relDir,
		Command:   cmd,
		Port:      port,
		Watch:     []string{"**/*.py"},
		Ignore:    []string{"__pycache__/**", ".venv/**"},
	}
}

func importDockerCompose(dir string, cfg *Config) {
	composeFiles := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
	for _, f := range composeFiles {
		path := filepath.Join(dir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var compose struct {
			Services map[string]struct {
				Image       string            `yaml:"image"`
				Ports       []string          `yaml:"ports"`
				Environment map[string]string `yaml:"environment"`
				Volumes     []string          `yaml:"volumes"`
			} `yaml:"services"`
		}

		if err := yaml.Unmarshal(data, &compose); err == nil && len(compose.Services) > 0 {
			for name, s := range compose.Services {
				if s.Image != "" {
					cfg.Dependencies[name] = Dependency{
						Image:   s.Image,
						Ports:   s.Ports,
						Env:     s.Environment,
						Volumes: s.Volumes,
					}
				}
			}
			break
		}
	}
}

func hasFile(dir, filename string) bool {
	p := filepath.Join(dir, filename)
	_, err := os.Stat(p)
	return err == nil
}

func sanitizeServiceName(base string, existing map[string]Service) string {
	clean := strings.ToLower(strings.TrimSpace(base))
	clean = strings.ReplaceAll(clean, "/", "-")
	clean = strings.ReplaceAll(clean, "\\", "-")
	clean = strings.ReplaceAll(clean, "_", "-")
	clean = strings.ReplaceAll(clean, " ", "-")

	if clean == "" {
		clean = "service"
	}

	name := clean
	counter := 1
	for {
		if _, exists := existing[name]; !exists {
			return name
		}
		counter++
		name = fmt.Sprintf("%s-%d", clean, counter)
	}
}
