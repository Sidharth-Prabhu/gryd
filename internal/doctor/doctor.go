package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/loom-cli/gryd/internal/config"
)

// ToolStatus describes the health and accessibility of a single required tool.
type ToolStatus struct {
	Name     string   `json:"name"`
	Found    bool     `json:"found"`
	Path     string   `json:"path"`
	Version  string   `json:"version,omitempty"`
	Source   string   `json:"source"` // "custom_path", "system_path", "standard_location", "not_found"
	Error    string   `json:"error,omitempty"`
	UsedBy   []string `json:"used_by"`
	Required bool     `json:"required"`
}

// Report holds the complete diagnostic result for a project.
type Report struct {
	ProjectName string       `json:"project_name"`
	ConfigPath  string       `json:"config_path"`
	Tools       []ToolStatus `json:"tools"`
	AllPassed   bool         `json:"all_passed"`
	Summary     string       `json:"summary"`
}

// MissingTools returns any required tools that failed detection.
func (r *Report) MissingTools() []ToolStatus {
	var missing []ToolStatus
	for _, t := range r.Tools {
		if !t.Found && t.Required {
			missing = append(missing, t)
		}
	}
	return missing
}

// CheckProject inspects all tools required by services and explicitly configured in gryd.yaml.
func CheckProject(cfg *config.Config) *Report {
	report := &Report{
		ProjectName: cfg.Name,
		ConfigPath:  cfg.ConfigPath,
		AllPassed:   true,
	}

	// 1. Map of toolName -> list of usages
	usages := make(map[string][]string)
	requiredMap := make(map[string]bool)

	// Inferred tools from service commands
	for svcName, svc := range cfg.Services {
		if svc.Command != "" {
			bins := ExtractCommandBinaries(svc.Command)
			for _, b := range bins {
				usages[b] = append(usages[b], fmt.Sprintf("service '%s'", svcName))
				requiredMap[b] = true
			}
		}
	}

	// Sidecars require a container runtime
	if len(cfg.Dependencies) > 0 {
		usages["docker"] = append(usages["docker"], "container sidecars")
		requiredMap["docker"] = true
	}

	// Explicit tools defined in gryd.yaml
	if cfg.Tools != nil {
		for toolName, toolCfg := range cfg.Tools {
			if _, exists := usages[toolName]; !exists {
				usages[toolName] = []string{"gryd.yaml tools"}
			}
			if toolCfg.Required {
				requiredMap[toolName] = true
			}
		}
	}

	// 2. Evaluate each tool
	for toolName, usedBy := range usages {
		req := requiredMap[toolName]
		status := evaluateTool(toolName, cfg, usedBy, req)
		report.Tools = append(report.Tools, status)
		if !status.Found && req {
			report.AllPassed = false
		}
	}

	// Sort tools deterministically by name
	for i := 0; i < len(report.Tools)-1; i++ {
		for j := i + 1; j < len(report.Tools); j++ {
			if report.Tools[i].Name > report.Tools[j].Name {
				report.Tools[i], report.Tools[j] = report.Tools[j], report.Tools[i]
			}
		}
	}

	missing := report.MissingTools()
	if len(missing) == 0 {
		report.Summary = fmt.Sprintf("All %d tools verified and accessible.", len(report.Tools))
	} else {
		report.Summary = fmt.Sprintf("%d required tool(s) missing or inaccessible.", len(missing))
	}

	return report
}

func evaluateTool(name string, cfg *config.Config, usedBy []string, required bool) ToolStatus {
	status := ToolStatus{
		Name:     name,
		UsedBy:   usedBy,
		Required: required,
	}

	// Case 1: Custom path configured in gryd.yaml
	if cfg != nil && cfg.Tools != nil {
		if tc, ok := cfg.Tools[name]; ok && strings.TrimSpace(tc.Path) != "" {
			customPath := expandPath(tc.Path, cfg.WorkingDirectory)
			info, err := os.Stat(customPath)
			if err != nil {
				status.Found = false
				status.Source = "custom_path"
				status.Path = customPath
				status.Error = fmt.Sprintf("configured path does not exist (%s)", customPath)
				return status
			}
			if info.IsDir() {
				status.Found = false
				status.Source = "custom_path"
				status.Path = customPath
				status.Error = fmt.Sprintf("configured path is a directory, expected executable file")
				return status
			}
			if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
				status.Found = false
				status.Source = "custom_path"
				status.Path = customPath
				status.Error = fmt.Sprintf("configured path is not executable (permissions: %s)", info.Mode())
				return status
			}

			status.Found = true
			status.Path = customPath
			status.Source = "custom_path"
			status.Version = detectVersion(customPath)
			return status
		}
	}

	// Special case for container runtime
	if name == "docker" || name == "podman" {
		if path, err := exec.LookPath(name); err == nil {
			// Check if daemon is active
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, path, "info")
			if err := cmd.Run(); err == nil {
				status.Found = true
				status.Path = path
				status.Source = "system_path"
				status.Version = detectVersion(path)
				return status
			} else {
				status.Found = false
				status.Path = path
				status.Source = "system_path"
				status.Error = "daemon is not running or not responsive"
				return status
			}
		}
	}

	// Case 2: Standard lookup in system PATH
	if path, err := exec.LookPath(name); err == nil && path != "" {
		absPath, _ := filepath.Abs(path)
		status.Found = true
		status.Path = absPath
		status.Source = "system_path"
		status.Version = detectVersion(absPath)
		return status
	}

	// Case 3: Standard OS fallback directories
	if fallback := findFallbackBinary(name); fallback != "" {
		status.Found = true
		status.Path = fallback
		status.Source = "standard_location"
		status.Version = detectVersion(fallback)
		return status
	}

	// Case 4: Not found
	status.Found = false
	status.Source = "not_found"
	status.Error = "not found in PATH or standard installation directories"
	return status
}

// ExtractCommandBinaries extracts the base executable names invoked by a shell command string.
func ExtractCommandBinaries(cmdStr string) []string {
	// Split compound commands by &&, ||, ;, |
	delims := regexp.MustCompile(`(&&|\|\||;|\|)`)
	parts := delims.Split(cmdStr, -1)

	var binaries []string
	seen := make(map[string]bool)

	// Shell builtins to ignore
	builtins := map[string]bool{
		"cd": true, "echo": true, "export": true, "set": true, "exit": true,
		"source": true, ".": true, "test": true, "true": true, "false": true,
		"exec": true, "trap": true, "wait": true, "shift": true,
	}

	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}

		// Skip environment variable assignments like PORT=3000 NODE_ENV=development
		idx := 0
		for idx < len(fields) && strings.Contains(fields[idx], "=") && !strings.HasPrefix(fields[idx], "-") {
			idx++
		}
		if idx >= len(fields) {
			continue
		}

		bin := fields[idx]
		// Clean up quotes
		bin = strings.Trim(bin, `"'`)

		// If path specified, take base name
		base := filepath.Base(bin)
		// Strip extension on Windows if present
		if runtime.GOOS == "windows" {
			base = strings.TrimSuffix(base, ".exe")
			base = strings.TrimSuffix(base, ".cmd")
			base = strings.TrimSuffix(base, ".bat")
		}

		if base != "" && !builtins[base] && !seen[base] {
			seen[base] = true
			binaries = append(binaries, base)
		}
	}

	return binaries
}

// findFallbackBinary checks well-known installation paths for common developer tools.
func findFallbackBinary(name string) string {
	home, _ := os.UserHomeDir()

	var searchDirs []string
	if runtime.GOOS == "windows" {
		localApp := os.Getenv("LOCALAPPDATA")
		appData := os.Getenv("APPDATA")
		progFiles := os.Getenv("ProgramFiles")
		searchDirs = []string{
			filepath.Join(home, ".gryd", "bin"),
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(appData, "npm"),
			filepath.Join(localApp, "pnpm"),
			filepath.Join(localApp, "Programs", name),
			filepath.Join(progFiles, name),
		}
	} else {
		searchDirs = []string{
			"/opt/homebrew/bin",
			"/opt/homebrew/sbin",
			"/usr/local/bin",
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, ".nvm", "current", "bin"),
			filepath.Join(home, ".local", "share", "pnpm"),
			filepath.Join(home, "Library", "pnpm"),
			filepath.Join(home, ".pnpm"),
			"/usr/bin",
			"/bin",
		}
	}

	exts := []string{""}
	if runtime.GOOS == "windows" {
		exts = []string{".exe", ".cmd", ".bat"}
	}

	for _, dir := range searchDirs {
		for _, ext := range exts {
			candidate := filepath.Join(dir, name+ext)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				if runtime.GOOS == "windows" || info.Mode()&0111 != 0 {
					return candidate
				}
			}
		}
	}

	return ""
}

// detectVersion runs a quick `--version` check to grab tool version string.
func detectVersion(binaryPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try --version first
	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to -v
		ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel2()
		cmd2 := exec.CommandContext(ctx2, binaryPath, "-v")
		out, err = cmd2.CombinedOutput()
		if err != nil {
			return ""
		}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}

	firstLine := strings.TrimSpace(lines[0])
	// Limit length
	if len(firstLine) > 40 {
		firstLine = firstLine[:37] + "..."
	}
	return firstLine
}

func expandPath(path, workingDir string) string {
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	path = os.ExpandEnv(path)
	if !filepath.IsAbs(path) && workingDir != "" {
		path = filepath.Join(workingDir, path)
	}
	return filepath.Clean(path)
}

// ApplyToolsToEnv updates environment variables with paths to configured tools,
// ensuring child processes execute using the verified tool locations.
func ApplyToolsToEnv(cfg *config.Config, baseEnv []string) []string {
	if cfg == nil {
		return baseEnv
	}

	var extraDirs []string

	// 1. If tools have custom paths in gryd.yaml, prepend their directories to PATH
	if cfg.Tools != nil {
		for _, tc := range cfg.Tools {
			if tc.Path != "" {
				resolved := expandPath(tc.Path, cfg.WorkingDirectory)
				dir := filepath.Dir(resolved)
				if _, err := os.Stat(dir); err == nil {
					extraDirs = append(extraDirs, dir)
				}
			}
		}
	}

	// 2. Also check if fallback locations were needed for any inferred service tool
	for _, svc := range cfg.Services {
		for _, bin := range ExtractCommandBinaries(svc.Command) {
			if _, err := exec.LookPath(bin); err != nil {
				if fallback := findFallbackBinary(bin); fallback != "" {
					dir := filepath.Dir(fallback)
					extraDirs = append(extraDirs, dir)
				}
			}
		}
	}

	if len(extraDirs) == 0 {
		return baseEnv
	}

	// Remove duplicates
	var uniqueDirs []string
	dirSeen := make(map[string]bool)
	for _, d := range extraDirs {
		if !dirSeen[d] {
			dirSeen[d] = true
			uniqueDirs = append(uniqueDirs, d)
		}
	}

	pathSep := string(os.PathListSeparator)
	prefix := strings.Join(uniqueDirs, pathSep)

	var newEnv []string
	pathFound := false
	for _, envVar := range baseEnv {
		if strings.HasPrefix(strings.ToUpper(envVar), "PATH=") {
			pathFound = true
			oldPath := envVar[5:]
			newEnv = append(newEnv, fmt.Sprintf("PATH=%s%s%s", prefix, pathSep, oldPath))
		} else {
			newEnv = append(newEnv, envVar)
		}
	}

	if !pathFound {
		newEnv = append(newEnv, fmt.Sprintf("PATH=%s", prefix))
	}

	return newEnv
}
