package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/loom-cli/gryd/internal/config"
	"go.uber.org/zap"
)

// Route maps a domain or hostname to a backend target port.
type Route struct {
	Domain      string
	ServiceName string
	TargetPort  int
	Proxy       *httputil.ReverseProxy
}

// ReverseProxyServer is a zero-configuration local reverse proxy that routes
// incoming HTTP traffic based on Host headers (e.g. api.localhost or web.gryd.test)
// to their configured service ports.
type ReverseProxyServer struct {
	server   *http.Server
	listener net.Listener
	routes   map[string]*Route
	port     int
	log      *zap.Logger
	mu       sync.RWMutex
}

// NewReverseProxyServer creates a reverse proxy server for the project configuration.
func NewReverseProxyServer(cfg *config.Config, log *zap.Logger) *ReverseProxyServer {
	routes := make(map[string]*Route)

	proxyPort := 80
	if cfg.Settings.ProxyPort > 0 {
		proxyPort = cfg.Settings.ProxyPort
	}

	r := &ReverseProxyServer{
		routes: routes,
		port:   proxyPort,
		log:    log,
	}

	for svcName, svc := range cfg.Services {
		if svc.Port <= 0 {
			continue
		}

		targetURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", svc.Port))
		proxy := httputil.NewSingleHostReverseProxy(targetURL)

		// Custom error handler to present clean diagnostic page if service is down
		proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>502 Bad Gateway - Gryd</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #141212; color: #F0E6E2; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
.card { background: #191716; border: 1px solid #302C2A; padding: 2rem; border-radius: 8px; max-width: 500px; }
h1 { color: #FF5A1F; font-size: 1.5rem; margin-top: 0; }
code { background: #221F1E; padding: 0.2rem 0.4rem; border-radius: 4px; color: #FF5A1F; }
</style>
</head>
<body>
<div class="card">
  <h1>⚡ Service Offline (502)</h1>
  <p>Gryd could not reach <strong>%s</strong> on port <code>%d</code>.</p>
  <p>Check your service logs with <code>gryd look</code> or make sure the service has booted.</p>
</div>
</body>
</html>`, svcName, svc.Port)
		}

		// 1. Explicit domain configured
		if svc.Domain != "" {
			d := strings.ToLower(strings.TrimSpace(svc.Domain))
			routes[d] = &Route{
				Domain:      d,
				ServiceName: svcName,
				TargetPort:  svc.Port,
				Proxy:       proxy,
			}
		}

		// 2. Default automatic conventions:
		// <service>.localhost and <service>.<project>.localhost
		// e.g. api.localhost and api.myapp.localhost
		defaultLocalhost := fmt.Sprintf("%s.localhost", strings.ToLower(svcName))
		if _, exists := routes[defaultLocalhost]; !exists {
			routes[defaultLocalhost] = &Route{
				Domain:      defaultLocalhost,
				ServiceName: svcName,
				TargetPort:  svc.Port,
				Proxy:       proxy,
			}
		}

		projLocalhost := fmt.Sprintf("%s.%s.localhost", strings.ToLower(svcName), strings.ToLower(cfg.Name))
		if _, exists := routes[projLocalhost]; !exists {
			routes[projLocalhost] = &Route{
				Domain:      projLocalhost,
				ServiceName: svcName,
				TargetPort:  svc.Port,
				Proxy:       proxy,
			}
		}

		// Also register <service>.<project>.test and <service>.gryd.test
		testDomain := fmt.Sprintf("%s.gryd.test", strings.ToLower(svcName))
		if _, exists := routes[testDomain]; !exists {
			routes[testDomain] = &Route{
				Domain:      testDomain,
				ServiceName: svcName,
				TargetPort:  svc.Port,
				Proxy:       proxy,
			}
		}
	}

	return r
}

// GetRoutes returns the active routing table.
func (s *ReverseProxyServer) GetRoutes() map[string]*Route {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copied := make(map[string]*Route, len(s.routes))
	for k, v := range s.routes {
		copied[k] = v
	}
	return copied
}

// GetDomains returns a slice of all registered domain names.
func (s *ReverseProxyServer) GetDomains() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var domains []string
	for d := range s.routes {
		domains = append(domains, d)
	}
	return domains
}

// Port returns the listening port of the proxy.
func (s *ReverseProxyServer) Port() int {
	return s.port
}

// Start begins listening and serving reverse proxy requests.
func (s *ReverseProxyServer) Start() error {
	// Try preferred port (e.g. 80, or fall back to 8080/8888 if port 80 requires root / is busy)
	portsToTry := []int{s.port}
	if s.port != 80 {
		portsToTry = append(portsToTry, 80)
	}
	portsToTry = append(portsToTry, 8080, 8888, 3333)

	var ln net.Listener
	var err error
	actualPort := s.port

	for _, p := range portsToTry {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			actualPort = p
			break
		}
	}

	if ln == nil {
		return fmt.Errorf("failed to bind reverse proxy on any local port: %w", err)
	}

	s.port = actualPort
	s.listener = ln

	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		host := req.Host
		if colon := strings.Index(host, ":"); colon != -1 {
			host = host[:colon]
		}
		host = strings.ToLower(host)

		s.mu.RLock()
		route, ok := s.routes[host]
		s.mu.RUnlock()

		if ok && route != nil && route.Proxy != nil {
			req.Header.Set("X-Forwarded-Host", req.Host)
			req.Header.Set("X-Forwarded-Proto", "http")
			route.Proxy.ServeHTTP(w, req)
			return
		}

		// If no matching route found, display a directory of available local domains
		s.renderDirectory(w, req, host)
	})

	s.server = &http.Server{
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		_ = s.server.Serve(ln)
	}()

	if s.log != nil {
		s.log.Info("Local domain reverse proxy started", zap.Int("port", s.port))
	}

	return nil
}

func (s *ReverseProxyServer) renderDirectory(w http.ResponseWriter, req *http.Request, requestedHost string) {
	w.WriteHeader(http.StatusNotFound)
	routes := s.GetRoutes()

	rows := ""
	for domain, r := range routes {
		url := fmt.Sprintf("http://%s", domain)
		if s.port != 80 {
			url = fmt.Sprintf("http://%s:%d", domain, s.port)
		}
		rows += fmt.Sprintf(`<tr>
  <td><a href="%s">%s</a></td>
  <td><code>%s</code></td>
  <td><code>127.0.0.1:%d</code></td>
</tr>`, url, domain, r.ServiceName, r.TargetPort)
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Gryd Local Proxy</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #141212; color: #F0E6E2; margin: 0; padding: 2rem; display: flex; justify-content: center; }
.container { max-width: 650px; width: 100%%; background: #191716; border: 1px solid #302C2A; padding: 2rem; border-radius: 8px; }
h1 { color: #FF5A1F; font-size: 1.5rem; margin-top: 0; }
table { width: 100%%; border-collapse: collapse; margin-top: 1.5rem; }
th, td { text-align: left; padding: 0.6rem 0.8rem; border-bottom: 1px solid #302C2A; font-size: 0.95rem; }
th { color: #A9A19E; font-weight: 500; font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.05em; }
a { color: #FF5A1F; text-decoration: none; font-weight: 500; }
a:hover { text-decoration: underline; }
code { background: #221F1E; padding: 0.2rem 0.4rem; border-radius: 4px; font-size: 0.85rem; }
.notice { color: #A9A19E; font-size: 0.9rem; margin-top: 1.5rem; line-height: 1.4; }
</style>
</head>
<body>
<div class="container">
  <h1>⚡ Gryd Local Domain Router</h1>
  <p>No service configured for host <code>%s</code>.</p>
  <h3>Available Local Routes:</h3>
  <table>
    <thead>
      <tr>
        <th>Domain</th>
        <th>Service</th>
        <th>Target Port</th>
      </tr>
    </thead>
    <tbody>
      %s
    </tbody>
  </table>
  <div class="notice">
    Domains ending in <code>.localhost</code> resolve natively without editing your hosts file.<br>
    For custom domains like <code>.test</code>, run <code>sudo gryd hosts sync</code>.
  </div>
</div>
</body>
</html>`, requestedHost, rows)
}

// Close gracefully stops the proxy server.
func (s *ReverseProxyServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}
