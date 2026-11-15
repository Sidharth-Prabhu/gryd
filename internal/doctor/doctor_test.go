package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loom-cli/gryd/internal/config"
)

func TestExtractCommandBinaries(t *testing.T) {
	tests := []struct {
		cmd      string
		expected []string
	}{
		{"pnpm run dev --port 3001", []string{"pnpm"}},
		{"PORT=3000 NODE_ENV=development node server.js", []string{"node"}},
		{"python main.py && npm test", []string{"python", "npm"}},
		{"/usr/local/bin/go run .", []string{"go"}},
		{"echo 'hello'", []string{}}, // builtin ignored
		{"cd frontend && pnpm dev", []string{"pnpm"}},
	}

	for _, tt := range tests {
		got := ExtractCommandBinaries(tt.cmd)
		if len(got) != len(tt.expected) {
			t.Errorf("ExtractCommandBinaries(%q) = %v; expected %v", tt.cmd, got, tt.expected)
			continue
		}
		for i, exp := range tt.expected {
			if got[i] != exp {
				t.Errorf("ExtractCommandBinaries(%q)[%d] = %q; expected %q", tt.cmd, i, got[i], exp)
			}
		}
	}
}

func TestCheckProjectWithCustomTools(t *testing.T) {
	tempDir := t.TempDir()

	// Create a fake custom tool executable
	fakeTool := filepath.Join(tempDir, "my-tool")
	if err := os.WriteFile(fakeTool, []byte("#!/bin/sh\necho v1.2.3\n"), 0755); err != nil {
		t.Fatalf("failed to create fake tool: %v", err)
	}

	cfg := &config.Config{
		Name:             "test-project",
		WorkingDirectory: tempDir,
		Services: map[string]config.Service{
			"worker": {
				Command: "my-tool process",
			},
		},
		Tools: config.ToolsMap{
			"my-tool": config.ToolConfig{
				Path: fakeTool,
			},
		},
	}

	report := CheckProject(cfg)
	if !report.AllPassed {
		t.Errorf("expected all passed, got failed report: %+v", report)
	}

	found := false
	for _, tool := range report.Tools {
		if tool.Name == "my-tool" {
			found = true
			if !tool.Found {
				t.Errorf("expected my-tool to be found")
			}
			if tool.Source != "custom_path" {
				t.Errorf("expected source 'custom_path', got '%s'", tool.Source)
			}
			if tool.Path != fakeTool {
				t.Errorf("expected path '%s', got '%s'", fakeTool, tool.Path)
			}
		}
	}
	if !found {
		t.Errorf("my-tool was not evaluated in report")
	}
}

func TestCheckProjectMissingTool(t *testing.T) {
	cfg := &config.Config{
		Name:             "missing-test",
		WorkingDirectory: "/tmp",
		Services: map[string]config.Service{
			"worker": {
				Command: "nonexistent-fake-binary-12345 --run",
			},
		},
	}

	report := CheckProject(cfg)
	if report.AllPassed {
		t.Errorf("expected report to fail for missing tool")
	}

	missing := report.MissingTools()
	if len(missing) != 1 || missing[0].Name != "nonexistent-fake-binary-12345" {
		t.Errorf("expected missing nonexistent-fake-binary-12345, got %+v", missing)
	}
}

func TestApplyToolsToEnv(t *testing.T) {
	tempDir := t.TempDir()
	fakeTool := filepath.Join(tempDir, "custom-tool")
	_ = os.WriteFile(fakeTool, []byte("#!/bin/sh\n"), 0755)

	cfg := &config.Config{
		WorkingDirectory: tempDir,
		Tools: config.ToolsMap{
			"custom-tool": config.ToolConfig{
				Path: fakeTool,
			},
		},
	}

	baseEnv := []string{"USER=test", "PATH=/usr/bin:/bin"}
	updatedEnv := ApplyToolsToEnv(cfg, baseEnv)

	foundPath := false
	for _, env := range updatedEnv {
		if strings.HasPrefix(env, "PATH=") {
			foundPath = true
			if !strings.Contains(env, tempDir) {
				t.Errorf("expected PATH to contain tempDir %s, got %s", tempDir, env)
			}
		}
	}
	if !foundPath {
		t.Errorf("PATH not found in updated env")
	}
}

func TestRenderCard(t *testing.T) {
	cfg := &config.Config{
		Name:       "render-test",
		ConfigPath: "/tmp/gryd.yaml",
		Services: map[string]config.Service{
			"api": {Command: "echo hello"},
		},
	}
	report := CheckProject(cfg)
	output := RenderCard(report, 80)
	if !strings.Contains(output, "DOCTOR") {
		t.Errorf("expected card to contain DOCTOR header")
	}
}
