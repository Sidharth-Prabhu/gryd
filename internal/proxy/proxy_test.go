package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loom-cli/gryd/internal/config"
)

func TestReverseProxyRouting(t *testing.T) {
	// 1. Create a dummy backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Hello from backend service! Host: %s", r.Header.Get("X-Forwarded-Host"))
	}))
	defer backend.Close()

	backendPort := backend.Listener.Addr().(*net.TCPAddr).Port

	cfg := &config.Config{
		Name: "test-app",
		Services: map[string]config.Service{
			"api": {
				Command: "echo api",
				Port:    backendPort,
				Domain:  "api.custom.local",
			},
		},
	}

	proxyServer := NewReverseProxyServer(cfg, nil)
	err := proxyServer.Start()
	if err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer proxyServer.Close()

	// 2. Test request to explicit domain
	client := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/test", proxyServer.Port()), nil)
	req.Host = "api.custom.local"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Hello from backend service!") {
		t.Errorf("unexpected proxy response body: %s", string(body))
	}

	// 3. Test automatic convention: api.localhost
	req2, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/test", proxyServer.Port()), nil)
	req2.Host = "api.localhost"

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("proxy request 2 failed: %v", err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), "Hello from backend service!") {
		t.Errorf("unexpected proxy response for api.localhost: %s", string(body2))
	}

	// 4. Test unconfigured host returns 404 directory
	req3, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", proxyServer.Port()), nil)
	req3.Host = "nonexistent.localhost"

	resp3, err := client.Do(req3)
	if err != nil {
		t.Fatalf("proxy request 3 failed: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unconfigured host, got %d", resp3.StatusCode)
	}
}
