package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorCommand(t *testing.T) {
	tempDir := t.TempDir()

	// Create dummy executable
	fakeTool := filepath.Join(tempDir, "my-compiler")
	_ = os.WriteFile(fakeTool, []byte("#!/bin/sh\necho v1.0.0\n"), 0755)

	configContent := `name: doctor-test
working_directory: ` + tempDir + `
tools:
  my-compiler: ` + fakeTool + `
services:
  build:
    command: my-compiler build
`
	configPath := filepath.Join(tempDir, "gryd.yaml")
	_ = os.WriteFile(configPath, []byte(configContent), 0644)

	configFile = configPath
	defer func() { configFile = "" }()

	// Test normal run
	doctorJSONFlag = false
	err := doctorCmd.RunE(doctorCmd, []string{})
	if err != nil {
		t.Fatalf("expected doctorCmd to succeed, got: %v", err)
	}

	// Test JSON flag
	doctorJSONFlag = true
	defer func() { doctorJSONFlag = false }()
	err = doctorCmd.RunE(doctorCmd, []string{})
	if err != nil {
		t.Fatalf("expected doctorCmd with --json to succeed, got: %v", err)
	}
}
