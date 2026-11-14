package runner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/loom-cli/gryd/internal/config"
	"go.uber.org/zap"
)

// CheckServiceHealth evaluates whether a service's healthcheck passes.
// If no healthcheck is configured, it falls back to port check if configured, or returns true.
func CheckServiceHealth(ctx context.Context, svc config.Service, log *zap.Logger) (bool, error) {
	if svc.HealthCheck == nil {
		if svc.Port > 0 {
			return IsPortInUse(svc.Port), nil
		}
		return true, nil
	}

	hc := svc.HealthCheck
	timeout := hc.GetTimeout()

	// 1. HTTP Probe
	if hc.HTTP != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, hc.HTTP, nil)
		if err != nil {
			return false, err
		}
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return true, nil
		}
		return false, fmt.Errorf("HTTP probe returned status %d", resp.StatusCode)
	}

	// 2. TCP Probe
	if hc.TCP != "" {
		target := hc.TCP
		if !strings.Contains(target, ":") && svc.Port > 0 {
			target = fmt.Sprintf("%s:%d", target, svc.Port)
		}
		var d net.Dialer
		d.Timeout = timeout
		conn, err := d.DialContext(ctx, "tcp", target)
		if err != nil {
			return false, err
		}
		_ = conn.Close()
		return true, nil
	}

	// 3. If port is specified without explicit http/tcp probe
	if svc.Port > 0 {
		return IsPortInUse(svc.Port), nil
	}

	return true, nil
}

// WaitForServiceHealth polls a service until its healthcheck passes or timeout/retries expire.
func WaitForServiceHealth(ctx context.Context, serviceName string, svc config.Service, log *zap.Logger) error {
	hc := svc.HealthCheck
	retries := 10
	interval := 1 * time.Second

	if hc != nil {
		retries = hc.GetRetries()
		interval = hc.GetInterval()
	}

	if log != nil {
		log.Info("Waiting for service readiness",
			zap.String("service", serviceName),
			zap.Int("max_retries", retries),
			zap.Duration("interval", interval),
		)
	}

	for attempt := 1; attempt <= retries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ok, err := CheckServiceHealth(ctx, svc, log)
		if ok {
			if log != nil {
				log.Info("Service passed readiness healthcheck",
					zap.String("service", serviceName),
					zap.Int("attempt", attempt),
				)
			}
			return nil
		}

		if log != nil && err != nil {
			log.Debug("Service health check not ready yet",
				zap.String("service", serviceName),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
		}

		time.Sleep(interval)
	}

	return fmt.Errorf("service '%s' failed health check after %d attempts", serviceName, retries)
}
