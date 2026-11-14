package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loom-cli/gryd/internal/config"
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

	// Simulate user answering:
	// Base domain: (Enter for default gryd.test)
	// api subdomain: custom-api
	// web subdomain: (Enter for default 'web')
	// sync to hosts: n (skip sudo)
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

	// Verify updated config
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
}
