package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/auth"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// sessionTTL is how long a sign-in lasts.
const sessionTTL = 7 * 24 * time.Hour

// minPasswordLength for team-mode accounts.
const minPasswordLength = 8

// AuthHandlers serve sign-in, sessions, API tokens and (admin) user management.
type AuthHandlers struct {
	svc   *auth.Service
	users *db.UserStore
}

func NewAuthHandlers(svc *auth.Service, users *db.UserStore) *AuthHandlers {
	return &AuthHandlers{svc: svc, users: users}
}

type sessionResponse struct {
	Mode          string          `json:"mode"` // local | team
	Authenticated bool            `json:"authenticated"`
	SetupRequired bool            `json:"setup_required"`
	Token         string          `json:"token,omitempty"`
	User          *auth.Principal `json:"user,omitempty"`
}

// Session tells the UI how to authenticate. In local mode it hands out the
// installation token; httpx guarantees only same-origin pages can read it.
func (h *AuthHandlers) Session(w http.ResponseWriter, r *http.Request) {
	if !h.svc.TeamMode {
		writeJSON(w, http.StatusOK, sessionResponse{Mode: "local", Authenticated: true, Token: h.svc.LocalToken, User: auth.LocalPrincipal})
		return
	}
	n, err := h.users.Count()
	if err != nil {
		internalError(w, err)
		return
	}
	resp := sessionResponse{Mode: "team", SetupRequired: n == 0}
	if p := auth.FromContext(r.Context()); p != nil {
		resp.Authenticated, resp.User = true, p
	}
	writeJSON(w, http.StatusOK, resp)
}

type credentials struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name,omitempty"`
}

func validEmail(s string) bool {
	_, err := mail.ParseAddress(s)
	return err == nil && strings.Contains(s, "@")
}

func (h *AuthHandlers) issue(w http.ResponseWriter, status int, u *db.User) {
	token, exp, err := h.users.CreateSession(u.ID, sessionTTL)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, status, map[string]any{
		"token":      token,
		"expires_at": exp,
		"user":       auth.Principal{UserID: u.ID, Email: u.Email, Name: u.DisplayName, Role: u.Role},
	})
}

// Setup creates the first admin account (team mode, only while no users exist).
func (h *AuthHandlers) Setup(w http.ResponseWriter, r *http.Request) {
	if !h.svc.TeamMode {
		writeError(w, http.StatusConflict, "NOT_TEAM_MODE", "Accounts are only used in team mode (TEAM_MODE=true)", nil)
		return
	}
	if n, _ := h.users.Count(); n > 0 {
		writeError(w, http.StatusConflict, "ALREADY_SET_UP", "An admin account already exists; sign in instead", nil)
		return
	}
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if !validEmail(c.Email) || len(c.Password) < minPasswordLength {
		badRequest(w, "VALIDATION_ERROR", "A valid email and a password of at least 8 characters are required", nil)
		return
	}
	if c.DisplayName == "" {
		c.DisplayName = c.Email
	}
	u, _, err := h.users.Create(strings.ToLower(c.Email), c.DisplayName, c.Password, auth.RoleAdmin)
	if err != nil {
		internalError(w, err)
		return
	}
	h.issue(w, http.StatusCreated, u)
}

// Login exchanges email and password for a session token.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	if !h.svc.TeamMode {
		writeError(w, http.StatusConflict, "NOT_TEAM_MODE", "Sign-in is only used in team mode", nil)
		return
	}
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	u, ok := h.users.ValidatePassword(strings.ToLower(strings.TrimSpace(c.Email)), c.Password)
	if !ok || u == nil {
		time.Sleep(300 * time.Millisecond) // slow down guessing
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect", nil)
		return
	}
	h.issue(w, http.StatusOK, u)
}

// Logout ends the current session.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if t := auth.BearerToken(r); strings.HasPrefix(t, db.SessionTokenPrefix) {
		h.users.DeleteSession(t)
	}
	w.WriteHeader(http.StatusNoContent)
}

// Me returns the signed-in principal.
func (h *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, auth.FromContext(r.Context()))
}

func (h *AuthHandlers) requireUser(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p := auth.FromContext(r.Context())
	if p == nil || p.UserID == "" {
		writeError(w, http.StatusConflict, "NOT_TEAM_MODE", "Personal accounts and tokens exist only in team mode; in local mode use the installation token from Settings > About", nil)
		return nil
	}
	return p
}

// ChangePassword lets a user change their own password.
func (h *AuthHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	p := h.requireUser(w, r)
	if p == nil {
		return
	}
	var req struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if _, ok := h.users.ValidatePassword(p.Email, req.Current); !ok {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Current password is incorrect", nil)
		return
	}
	if len(req.New) < minPasswordLength {
		badRequest(w, "VALIDATION_ERROR", "New password must be at least 8 characters", nil)
		return
	}
	if err := h.users.SetPassword(p.UserID, req.New); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	p := h.requireUser(w, r)
	if p == nil {
		return
	}
	tokens, err := h.users.ListAPITokens(p.UserID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tokens})
}

func (h *AuthHandlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	p := h.requireUser(w, r)
	if p == nil {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.Name) == "" {
		badRequest(w, "VALIDATION_ERROR", "Give the token a name so you can recognise it later", nil)
		return
	}
	secret, t, err := h.users.CreateAPIToken(p.UserID, req.Name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": secret, "api_token": t})
}

func (h *AuthHandlers) DeleteToken(w http.ResponseWriter, r *http.Request) {
	p := h.requireUser(w, r)
	if p == nil {
		return
	}
	if err := h.users.DeleteAPIToken(p.UserID, r.PathValue("id")); err != nil {
		notFound(w, "TOKEN", r.PathValue("id"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Users (admin) ---

func (h *AuthHandlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

type userRequest struct {
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Password    string `json:"password,omitempty"`
	Role        string `json:"role"`
}

func validRole(r string) bool {
	return r == auth.RoleAdmin || r == auth.RoleEditor || r == auth.RoleViewer
}

func (h *AuthHandlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if !validEmail(req.Email) || len(req.Password) < minPasswordLength || !validRole(req.Role) {
		badRequest(w, "VALIDATION_ERROR", "A valid email, a password of at least 8 characters, and a role (admin, editor or viewer) are required", nil)
		return
	}
	if existing, _ := h.users.GetByEmail(strings.ToLower(req.Email)); existing != nil {
		writeError(w, http.StatusConflict, "EMAIL_TAKEN", "A user with that email already exists", nil)
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Email
	}
	u, _, err := h.users.Create(strings.ToLower(req.Email), req.DisplayName, req.Password, req.Role)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (h *AuthHandlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req userRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if !validRole(req.Role) {
		badRequest(w, "VALIDATION_ERROR", "role must be admin, editor or viewer", nil)
		return
	}
	u, err := h.users.Update(id, req.DisplayName, req.Role)
	if errors.Is(err, db.ErrLastAdmin) {
		writeError(w, http.StatusConflict, "LAST_ADMIN", err.Error(), nil)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	if u == nil {
		notFound(w, "USER", id)
		return
	}
	if req.Password != "" {
		if len(req.Password) < minPasswordLength {
			badRequest(w, "VALIDATION_ERROR", "Password must be at least 8 characters", nil)
			return
		}
		if err := h.users.SetPassword(id, req.Password); err != nil {
			internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *AuthHandlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if p := auth.FromContext(r.Context()); p != nil && p.UserID == id {
		writeError(w, http.StatusConflict, "CANNOT_DELETE_SELF", "You cannot delete your own account", nil)
		return
	}
	err := h.users.DeleteUser(id)
	if errors.Is(err, db.ErrLastAdmin) {
		writeError(w, http.StatusConflict, "LAST_ADMIN", err.Error(), nil)
		return
	}
	if err != nil {
		notFound(w, "USER", id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
