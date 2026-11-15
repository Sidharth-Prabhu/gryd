package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestToolsMapUnmarshalScalarMap(t *testing.T) {
	input := `
tools:
  pnpm: /usr/local/bin/pnpm
  node: /opt/homebrew/bin/node
`
	var cfg Config
	err := yaml.Unmarshal([]byte(input), &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(cfg.Tools))
	}
	if cfg.Tools["pnpm"].Path != "/usr/local/bin/pnpm" {
		t.Errorf("expected pnpm path '/usr/local/bin/pnpm', got '%s'", cfg.Tools["pnpm"].Path)
	}
	if cfg.Tools["node"].Path != "/opt/homebrew/bin/node" {
		t.Errorf("expected node path '/opt/homebrew/bin/node', got '%s'", cfg.Tools["node"].Path)
	}
}

func TestToolsMapUnmarshalStructuredMap(t *testing.T) {
	input := `
tools:
  pnpm:
    path: /custom/pnpm
    required: true
  python:
    path: /usr/bin/python3
`
	var cfg Config
	err := yaml.Unmarshal([]byte(input), &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if cfg.Tools["pnpm"].Path != "/custom/pnpm" || !cfg.Tools["pnpm"].Required {
		t.Errorf("unexpected pnpm config: %+v", cfg.Tools["pnpm"])
	}
	if cfg.Tools["python"].Path != "/usr/bin/python3" || cfg.Tools["python"].Required {
		t.Errorf("unexpected python config: %+v", cfg.Tools["python"])
	}
}

func TestToolsMapUnmarshalList(t *testing.T) {
	input := `
tools:
  - pnpm
  - node
  - name: docker
    path: /usr/local/bin/docker
`
	var cfg Config
	err := yaml.Unmarshal([]byte(input), &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := cfg.Tools["pnpm"]; !ok {
		t.Errorf("expected pnpm in tools")
	}
	if _, ok := cfg.Tools["node"]; !ok {
		t.Errorf("expected node in tools")
	}
	if cfg.Tools["docker"].Path != "/usr/local/bin/docker" {
		t.Errorf("expected docker path '/usr/local/bin/docker', got '%s'", cfg.Tools["docker"].Path)
	}
}
