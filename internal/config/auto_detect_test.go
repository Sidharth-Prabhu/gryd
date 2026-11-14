package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoDetectStack(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gryd-autodetect-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Subdir 1: Backend (Go)
	backendDir := filepath.Join(tempDir, "backend")
	_ = os.MkdirAll(backendDir, 0755)
	_ = os.WriteFile(filepath.Join(backendDir, "go.mod"), []byte("module mybackend\n\ngo 1.22\n"), 0644)

	// Subdir 2: Frontend (Node with Vite + pnpm)
	frontendDir := filepath.Join(tempDir, "frontend")
	_ = os.MkdirAll(frontendDir, 0755)
	pkgJSON := `{
		"name": "my-frontend",
		"scripts": {
			"dev": "vite"
		},
		"dependencies": {
			"vite": "^5.0.0"
		}
	}`
	_ = os.WriteFile(filepath.Join(frontendDir, "package.json"), []byte(pkgJSON), 0644)
	_ = os.WriteFile(filepath.Join(frontendDir, "pnpm-lock.yaml"), []byte(""), 0644)

	// Docker compose
	composeYAML := `
services:
  postgres:
    image: postgres:15
    ports:
      - "5432:5432"
`
	_ = os.WriteFile(filepath.Join(tempDir, "docker-compose.yml"), []byte(composeYAML), 0644)

	// Run AutoDetectStack
	cfg, err := AutoDetectStack(tempDir)
	if err != nil {
		t.Fatalf("AutoDetectStack failed: %v", err)
	}

	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}

	// Verify backend
	backend, ok := cfg.Services["backend"]
	if !ok {
		t.Fatalf("expected 'backend' service")
	}
	if backend.Command != "go run ." {
		t.Errorf("expected backend command 'go run .', got '%s'", backend.Command)
	}

	// Verify frontend
	frontend, ok := cfg.Services["frontend"]
	if !ok {
		t.Fatalf("expected 'frontend' service")
	}
	if frontend.Command != "pnpm run dev" {
		t.Errorf("expected frontend command 'pnpm run dev', got '%s'", frontend.Command)
	}
	if frontend.Port != 5173 {
		t.Errorf("expected frontend port 5173, got %d", frontend.Port)
	}
	if _, ok := frontend.DependsOn["backend"]; !ok {
		t.Errorf("expected frontend to depend on backend, got %v", frontend.DependsOn)
	}

	// Verify docker dependency
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(cfg.Dependencies))
	}
	pg, ok := cfg.Dependencies["postgres"]
	if !ok {
		t.Fatalf("expected 'postgres' dependency")
	}
	if pg.Image != "postgres:15" {
		t.Errorf("expected image 'postgres:15', got '%s'", pg.Image)
	}
}

func TestAutoDetectStackEmpty(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gryd-autodetect-empty-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_, err = AutoDetectStack(tempDir)
	if err == nil {
		t.Fatal("expected error on empty directory, got nil")
	}
}
