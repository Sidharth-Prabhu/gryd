package runner

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/loom-cli/gryd/internal/config"
)

func startExternalListener(t *testing.T) (int, func()) {
	// Find a free port first
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Launch external python3 listener process
	cmd := exec.Command("python3", "-c", fmt.Sprintf(`
import socket, time
s = socket.socket()
s.bind(('127.0.0.1', %d))
s.listen(1)
print("READY", flush=True)
time.sleep(10)
`, port))

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe error: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start external listener process: %v", err)
	}

	buf := make([]byte, 10)
	_, _ = outPipe.Read(buf)
	time.Sleep(100 * time.Millisecond)

	cleanup := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return port, cleanup
}

func TestPromptCollision_Assign(t *testing.T) {
	occupiedPort, cleanup := startExternalListener(t)
	defer cleanup()

	cfg := &config.Config{
		Name: "test-collision",
		Services: map[string]config.Service{
			"api": {
				Command: "echo api",
				Port:    occupiedPort,
			},
		},
	}

	// Simulate user typing 'a' (assign)
	in := strings.NewReader("a\n")
	var out bytes.Buffer

	err := InteractiveResolvePortConflicts(cfg, in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Services["api"].Port == occupiedPort {
		t.Errorf("expected port to be reassigned, remained %d", occupiedPort)
	}
	if cfg.Services["api"].Env["PORT"] != fmt.Sprintf("%d", cfg.Services["api"].Port) {
		t.Errorf("expected PORT env to match %d, got %s", cfg.Services["api"].Port, cfg.Services["api"].Env["PORT"])
	}
}

func TestPromptCollision_Cancel(t *testing.T) {
	occupiedPort, cleanup := startExternalListener(t)
	defer cleanup()

	cfg := &config.Config{
		Name: "test-collision",
		Services: map[string]config.Service{
			"api": {
				Command: "echo api",
				Port:    occupiedPort,
			},
		},
	}

	// Simulate user typing 'c' (cancel)
	in := strings.NewReader("c\n")
	var out bytes.Buffer

	err := InteractiveResolvePortConflicts(cfg, in, &out)
	if err == nil {
		t.Fatalf("expected cancel error, got nil")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("expected cancelled error message, got: %v", err)
	}
}

func TestStartupPlan_Order(t *testing.T) {
	services := map[string]config.Service{
		"frontend": {
			Command: "echo frontend",
			DependsOn: config.DependsOnMap{
				"backend": config.DepCondition{Condition: "service_healthy"},
			},
		},
		"backend": {
			Command: "echo backend",
			DependsOn: config.DependsOnMap{
				"db": config.DepCondition{},
			},
		},
		"db": {
			Command: "echo db",
		},
	}

	tiers, err := BuildStartupPlan(services)
	if err != nil {
		t.Fatalf("BuildStartupPlan error: %v", err)
	}

	if len(tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d: %v", len(tiers), tiers)
	}
	if tiers[0][0] != "db" {
		t.Errorf("expected tier 0 to be db, got %v", tiers[0])
	}
	if tiers[1][0] != "backend" {
		t.Errorf("expected tier 1 to be backend, got %v", tiers[1])
	}
	if tiers[2][0] != "frontend" {
		t.Errorf("expected tier 2 to be frontend, got %v", tiers[2])
	}
}

func TestStartupPlan_Cycle(t *testing.T) {
	services := map[string]config.Service{
		"a": {
			Command: "echo a",
			DependsOn: config.DependsOnMap{
				"b": config.DepCondition{},
			},
		},
		"b": {
			Command: "echo b",
			DependsOn: config.DependsOnMap{
				"a": config.DepCondition{},
			},
		},
	}

	_, err := BuildStartupPlan(services)
	if err == nil {
		t.Fatalf("expected cycle error, got nil")
	}
}

func TestHealthCheckProbe(t *testing.T) {
	// Test TCP health check
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	svc := config.Service{
		Port: port,
		HealthCheck: &config.HealthCheckConfig{
			TCP:      fmt.Sprintf("127.0.0.1:%d", port),
			Interval: "50ms",
			Timeout:  "200ms",
			Retries:  3,
		},
	}

	ok, err := CheckServiceHealth(context.Background(), svc, nil)
	if !ok || err != nil {
		t.Errorf("expected TCP health check to succeed, got %v, %v", ok, err)
	}

	// Test non-listening port
	svcUnhealthy := config.Service{
		HealthCheck: &config.HealthCheckConfig{
			TCP:      "127.0.0.1:59999",
			Interval: "50ms",
			Timeout:  "100ms",
			Retries:  1,
		},
	}
	ok, _ = CheckServiceHealth(context.Background(), svcUnhealthy, nil)
	if ok {
		t.Errorf("expected non-listening port to fail health check")
	}
}
