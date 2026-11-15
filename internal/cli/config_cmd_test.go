package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/state"
)

func TestDomainRoutingConfigInteractive(t *testing.T) {
	tempDir := t.TempDir()
	configContent := `name: testproj
services:
  api:
    command: python api.py
    port: 8000
  web:
    command: npm run dev
    port: 3000
`
	configPath := filepath.Join(tempDir, "gryd.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	configFile = configPath
	defer func() { configFile = "" }()

	input := "\ncustom-api\n\nn\n"

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		_, _ = w.Write([]byte(input))
		_ = w.Close()
	}()

	err := runDomainRoutingConfig()
	if err != nil {
		t.Fatalf("runDomainRoutingConfig failed: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	if cfg.Services["api"].Domain != "custom-api.gryd.test" {
		t.Errorf("expected api domain 'custom-api.gryd.test', got '%s'", cfg.Services["api"].Domain)
	}
	if cfg.Services["web"].Domain != "web.gryd.test" {
		t.Errorf("expected web domain 'web.gryd.test', got '%s'", cfg.Services["web"].Domain)
	}

	revertInput := "n\n"
	r2, w2, _ := os.Pipe()
	os.Stdin = r2
	go func() {
		_, _ = w2.Write([]byte(revertInput))
		_ = w2.Close()
	}()

	err = runRemoveDomainRouting()
	if err != nil {
		t.Fatalf("runRemoveDomainRouting failed: %v", err)
	}

	cfgReverted, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	if cfgReverted.Services["api"].Domain != "" {
		t.Errorf("expected empty domain after removal, got '%s'", cfgReverted.Services["api"].Domain)
	}
	if cfgReverted.Services["web"].Domain != "" {
		t.Errorf("expected empty domain after removal, got '%s'", cfgReverted.Services["web"].Domain)
	}
}

func TestShowConfigAndJSON(t *testing.T) {
	tempDir := t.TempDir()
	configContent := `name: show-test
services:
  api:
    command: python api.py
    port: 8000
`
	configPath := filepath.Join(tempDir, "gryd.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	configFile = configPath
	defer func() { configFile = "" }()

	// Test runShowConfig
	err := runShowConfig()
	if err != nil {
		t.Fatalf("runShowConfig failed: %v", err)
	}

	// Test runShowJSONConfig
	err = runShowJSONConfig()
	if err != nil {
		t.Fatalf("runShowJSONConfig failed: %v", err)
	}
}

func TestEditorPreferences(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	// Initially empty
	if def := state.GetDefaultEditor(); def != "" {
		t.Errorf("expected empty default editor, got %s", def)
	}

	// Set default to nano
	err := state.SetDefaultEditor("nano")
	if err != nil {
		t.Fatalf("failed to set default editor: %v", err)
	}

	if def := state.GetDefaultEditor(); def != "nano" {
		t.Errorf("expected default editor nano, got %s", def)
	}

	// Change to vim
	err = state.SetDefaultEditor("vim")
	if err != nil {
		t.Fatalf("failed to set default editor: %v", err)
	}

	if def := state.GetDefaultEditor(); def != "vim" {
		t.Errorf("expected default editor vim, got %s", def)
	}
}
