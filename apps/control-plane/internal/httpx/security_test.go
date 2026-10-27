package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := Middleware(SecurityConfig{AllowedOrigins: []string{"http://localhost:5173"}, AllowedHosts: []string{"perf.internal"}})(ok)

	cases := []struct {
		name, host, origin string
		want               int
	}{
		{"loopback without origin", "localhost:8090", "", 200},
		{"same origin", "localhost:8080", "http://localhost:8080", 200},
		{"configured dev origin", "localhost:5173", "http://localhost:5173", 200},
		{"configured host", "perf.internal:8080", "http://perf.internal:8080", 200},
		{"cross-site page", "localhost:8090", "https://evil.example", 403},
		{"dns rebinding", "evil.example:8090", "http://evil.example:8090", 403},
		{"unknown host", "10.0.0.5:8090", "", 403},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "http://"+c.host+"/api/v1/runs", nil)
		req.Host = c.host
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
		if c.want == 200 && c.origin != "" && rec.Header().Get("Access-Control-Allow-Origin") != c.origin {
			t.Errorf("%s: missing CORS header for allowed origin", c.name)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
			t.Errorf("%s: wildcard CORS must never be sent", c.name)
		}
	}

	any := Middleware(SecurityConfig{AllowedHosts: []string{"*"}})(ok)
	req := httptest.NewRequest("GET", "http://perf.example.com/api/v1/plans", nil)
	rec := httptest.NewRecorder()
	any.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("wildcard host should be allowed, got %d", rec.Code)
	}
}
