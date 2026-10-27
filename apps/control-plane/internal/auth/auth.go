// Package auth authenticates API requests and enforces roles.
//
// Local mode (the default, for a single user on their own machine): the API
// requires the installation's bearer token, which the UI fetches from
// /api/v1/auth/session (only reachable same-origin, see httpx). Team mode
// (TEAM_MODE=true): users sign in with email and password; requests carry a
// session token or a personal API token, and roles gate what they can do:
// viewers read, editors change plans and run tests, admins also manage users
// and installation settings. See docs/decisions/0011-security-model.md.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// Roles, from least to most privileged.
const (
	RoleViewer = "viewer"
	RoleEditor = "editor"
	RoleAdmin  = "admin"
)

var rank = map[string]int{RoleViewer: 1, RoleEditor: 2, RoleAdmin: 3}

// Principal is the authenticated caller.
type Principal struct {
	UserID string `json:"id,omitempty"`
	Email  string `json:"email,omitempty"`
	Name   string `json:"display_name"`
	Role   string `json:"role"`
}

// Service authenticates tokens.
type Service struct {
	TeamMode   bool
	LocalToken string
	Users      *db.UserStore
}

// LoadLocalToken returns the installation token from OPENSYNAPSE_API_TOKEN
// or <dataDir>/api-token, creating the file on first start.
func LoadLocalToken(dataDir string) (string, error) {
	if t := os.Getenv("OPENSYNAPSE_API_TOKEN"); t != "" {
		return t, nil
	}
	path := filepath.Join(dataDir, "api-token")
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	t := "osl_" + hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(t), 0o600); err != nil {
		return "", err
	}
	return t, nil
}

// LocalPrincipal is the implicit single user of a local install.
var LocalPrincipal = &Principal{Name: "Local user", Role: RoleAdmin}

// Authenticate resolves a bearer token to a principal.
func (s *Service) Authenticate(token string) (*Principal, bool) {
	if token == "" {
		return nil, false
	}
	if !s.TeamMode {
		if s.LocalToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.LocalToken)) == 1 {
			return LocalPrincipal, true
		}
		return nil, false
	}
	u, err := s.Users.UserForToken(token)
	if err != nil || u == nil {
		return nil, false
	}
	return &Principal{UserID: u.ID, Email: u.Email, Name: u.DisplayName, Role: u.Role}, true
}

type ctxKey struct{}

// FromContext returns the request's principal (nil on public routes).
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}

// WithPrincipal attaches a principal to a context (used by tests).
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// BearerToken extracts the token from an Authorization header.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// isPublic lists routes reachable without a token. The WebSocket
// authenticates in its first message, the k6 control scenario with a
// per-run token, and OAuth callbacks arrive as browser redirects.
func isPublic(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case p == "/health" || p == "/ready" || p == "/version" || p == "/metrics" || p == "/openapi.json":
		return true
	case p == "/api/v1/auth/session" || p == "/api/v1/auth/login" || p == "/api/v1/auth/setup":
		return true
	case p == "/api/v1/ws":
		return true
	case strings.HasPrefix(p, "/api/v1/runs/") && strings.HasSuffix(p, "/control-state"):
		return true
	case p == "/api/v1/playground/oauth/callback":
		return true
	case !strings.HasPrefix(p, "/api/"):
		return true
	}
	return false
}

// readOnlyPOSTs are POST endpoints that do not change state.
var readOnlyPOSTs = []string{"/api/v1/compare", "/api/v1/ai/preview"}

// RequiredRole returns the minimum role for a request.
func RequiredRole(r *http.Request) string {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/v1/users"),
		p == "/api/v1/settings" && r.Method != http.MethodGet,
		strings.HasPrefix(p, "/api/v1/storage/"),
		p == "/api/v1/diagnostics",
		p == "/api/v1/system/logs",
		strings.HasPrefix(p, "/api/v1/ai/config") && r.Method != http.MethodGet:
		return RoleAdmin
	case p == "/api/v1/auth/logout", strings.HasPrefix(p, "/api/v1/auth/"):
		return RoleViewer // everyone manages their own session, password and tokens
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return RoleViewer
	}
	if r.Method == http.MethodPost {
		for _, ro := range readOnlyPOSTs {
			if p == ro {
				return RoleViewer
			}
		}
		if strings.HasSuffix(p, "/export") || strings.HasSuffix(p, "/compile") || strings.HasSuffix(p, "/validate") {
			return RoleViewer
		}
	}
	return RoleEditor
}

func deny(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// Middleware authenticates /api requests and enforces roles.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || isPublic(r) {
			// Public routes still learn who is calling if a token was sent.
			if p, ok := s.Authenticate(BearerToken(r)); ok {
				r = r.WithContext(WithPrincipal(r.Context(), p))
			}
			next.ServeHTTP(w, r)
			return
		}
		p, ok := s.Authenticate(BearerToken(r))
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="opensynapse"`)
			deny(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sign in or send a valid API token (Authorization: Bearer ...)")
			return
		}
		if need := RequiredRole(r); rank[p.Role] < rank[need] {
			deny(w, http.StatusForbidden, "FORBIDDEN", "Your role ("+p.Role+") cannot do this; it needs "+need+".")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}
