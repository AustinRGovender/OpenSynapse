// Package httpx holds HTTP middleware shared by the control plane.
package httpx

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// SecurityConfig controls which browsers may talk to the API.
type SecurityConfig struct {
	// AllowedHosts are Host header values (without port) the API answers to.
	// Loopback names are always allowed. "*" allows any host, for servers
	// reached through a reverse proxy or a DNS name.
	AllowedHosts []string
	// AllowedOrigins are extra browser origins (scheme://host:port) allowed
	// to make cross-origin requests, e.g. the Vite dev server.
	AllowedOrigins []string
}

var loopback = map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(hostport, "[]")
}

func (c SecurityConfig) hostAllowed(host string) bool {
	h := strings.ToLower(hostOnly(host))
	if loopback[h] {
		return true
	}
	for _, a := range c.AllowedHosts {
		if a == "*" || strings.EqualFold(a, h) {
			return true
		}
	}
	return false
}

func (c SecurityConfig) originAllowed(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	// Same origin: the page was served from the host the request targets.
	if strings.EqualFold(u.Host, host) {
		return true
	}
	for _, a := range c.AllowedOrigins {
		if strings.EqualFold(strings.TrimRight(a, "/"), origin) {
			return true
		}
	}
	return false
}

// OriginPatterns returns host patterns for the WebSocket origin check.
func (c SecurityConfig) OriginPatterns() []string {
	var out []string
	for _, o := range c.AllowedOrigins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			out = append(out, u.Host)
		}
	}
	return out
}

func deny(w http.ResponseWriter, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// Middleware rejects requests from hosts and origins that are not allowed.
//
// The control plane can launch load tests and proxy arbitrary HTTP requests
// (the playground), so it must not be usable from other websites the user
// visits: cross-origin requests are refused unless explicitly allowed, and
// the Host check stops DNS-rebinding attacks that would otherwise make an
// attacker's page look same-origin.
func Middleware(cfg SecurityConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("X-Frame-Options", "DENY")

			// Health checks come from orchestrators by IP address (kubelet
			// probes, load balancers); their static answers reveal nothing.
			probe := r.Method == http.MethodGet && (r.URL.Path == "/health" || r.URL.Path == "/ready")
			if !probe && !cfg.hostAllowed(r.Host) {
				deny(w, "HOST_NOT_ALLOWED", "Requests to host "+hostOnly(r.Host)+" are not allowed. Add it to OPENSYNAPSE_ALLOWED_HOSTS.")
				return
			}

			origin := r.Header.Get("Origin")
			if origin != "" {
				if !cfg.originAllowed(origin, r.Host) {
					deny(w, "ORIGIN_NOT_ALLOWED", "Cross-origin requests from "+origin+" are not allowed. Add it to OPENSYNAPSE_ALLOWED_ORIGINS.")
					return
				}
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Add("Vary", "Origin")
			}

			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
