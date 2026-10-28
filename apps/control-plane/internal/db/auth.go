package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Token prefixes make credentials recognisable in logs and secret scanners.
const (
	SessionTokenPrefix = "oss_"
	APITokenPrefix     = "osk_"
)

// APIToken is a personal access token (the secret itself is only shown once).
type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// ErrLastAdmin protects against removing the only administrator.
var ErrLastAdmin = errors.New("cannot remove or demote the last admin")

// Count returns the number of users.
func (s *UserStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *UserStore) countAdmins() int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&n)
	return n
}

// Update changes a user's display name and role.
func (s *UserStore) Update(id, displayName, role string) (*User, error) {
	u, err := s.Get(id)
	if err != nil || u == nil {
		return u, err
	}
	if role != "admin" && role != "editor" && role != "viewer" {
		return nil, fmt.Errorf("invalid role: %s", role)
	}
	if u.Role == "admin" && role != "admin" && s.countAdmins() <= 1 {
		return nil, ErrLastAdmin
	}
	if _, err := s.db.Exec(`UPDATE users SET display_name = ?, role = ? WHERE id = ?`, displayName, role, id); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	u.DisplayName, u.Role = displayName, role
	return u, nil
}

// SetPassword replaces a user's password and signs out their sessions.
func (s *UserStore) SetPassword(id, password string) error {
	if _, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hashPassword(password), id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	return err
}

// DeleteUser removes a user unless they are the last admin.
func (s *UserStore) DeleteUser(id string) error {
	u, err := s.Get(id)
	if err != nil {
		return err
	}
	if u == nil {
		return fmt.Errorf("user not found")
	}
	if u.Role == "admin" && s.countAdmins() <= 1 {
		return ErrLastAdmin
	}
	return s.Delete(id)
}

func newSecret(prefix string) string { return prefix + generateToken() }

// CreateSession issues a login session token for a user.
func (s *UserStore) CreateSession(userID string, ttl time.Duration) (string, time.Time, error) {
	token := newSecret(SessionTokenPrefix)
	now := time.Now().UTC()
	exp := now.Add(ttl)
	_, err := s.db.Exec(`INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		uuid.New().String(), userID, hashToken(token), now.Format(time.RFC3339), exp.Format(time.RFC3339))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return token, exp, nil
}

// DeleteSession signs a session out.
func (s *UserStore) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

func (s *UserStore) userBy(query string, args ...any) (*User, error) {
	var u User
	var createdAt string
	err := s.db.QueryRow(query, args...).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, nil
}

// UserForToken resolves a session or API token to its user.
func (s *UserStore) UserForToken(token string) (*User, error) {
	h := hashToken(token)
	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case strings.HasPrefix(token, SessionTokenPrefix):
		return s.userBy(`SELECT u.id, u.email, u.display_name, u.role, u.created_at
			FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > ?`, h, now)
	case strings.HasPrefix(token, APITokenPrefix):
		u, err := s.userBy(`SELECT u.id, u.email, u.display_name, u.role, u.created_at
			FROM api_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ?`, h)
		if u != nil {
			s.db.Exec(`UPDATE api_tokens SET last_used_at = ? WHERE token_hash = ?`, now, h)
		}
		return u, err
	}
	return nil, nil
}

// CreateAPIToken issues a personal access token; the secret is returned once.
func (s *UserStore) CreateAPIToken(userID, name string) (string, *APIToken, error) {
	token := newSecret(APITokenPrefix)
	t := &APIToken{ID: uuid.New().String(), Name: name, CreatedAt: time.Now().UTC()}
	_, err := s.db.Exec(`INSERT INTO api_tokens (id, user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.ID, userID, name, hashToken(token), t.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return "", nil, fmt.Errorf("create api token: %w", err)
	}
	return token, t, nil
}

// ListAPITokens lists a user's tokens (without secrets).
func (s *UserStore) ListAPITokens(userID string) ([]APIToken, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at, last_used_at FROM api_tokens WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		var created string
		var used sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &created, &used); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if used.Valid {
			u, _ := time.Parse(time.RFC3339, used.String)
			t.LastUsedAt = &u
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteAPIToken revokes one of a user's tokens.
func (s *UserStore) DeleteAPIToken(userID, id string) error {
	res, err := s.db.Exec(`DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("token not found")
	}
	return nil
}
