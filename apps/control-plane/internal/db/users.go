package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"` // admin, editor, viewer
	CreatedAt   time.Time `json:"created_at"`
}

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(email, displayName, password, role string) (*User, string, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if role == "" {
		role = "editor"
	}
	if role != "admin" && role != "editor" && role != "viewer" {
		return nil, "", fmt.Errorf("invalid role: %s", role)
	}

	passwordHash := hashPassword(password)
	apiToken := generateToken()
	tokenHash := hashToken(apiToken)
	nowStr := now.Format(time.RFC3339)

	_, err := s.db.Exec(
		`INSERT INTO users (id, email, display_name, role, password_hash, api_token_hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, email, displayName, role, passwordHash, tokenHash, nowStr,
	)
	if err != nil {
		return nil, "", fmt.Errorf("create user: %w", err)
	}

	return &User{
		ID: id, Email: email, DisplayName: displayName,
		Role: role, CreatedAt: now,
	}, apiToken, nil
}

func (s *UserStore) Get(id string) (*User, error) {
	var u User
	var createdAt string

	err := s.db.QueryRow(
		`SELECT id, email, display_name, role, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, nil
}

func (s *UserStore) GetByEmail(email string) (*User, error) {
	var u User
	var createdAt string

	err := s.db.QueryRow(
		`SELECT id, email, display_name, role, created_at FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, nil
}

func (s *UserStore) ValidatePassword(email, password string) (*User, bool) {
	var u User
	var passwordHash, createdAt string

	err := s.db.QueryRow(
		`SELECT id, email, display_name, role, password_hash, created_at FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &passwordHash, &createdAt)
	if err != nil {
		return nil, false
	}

	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, verifyPassword(password, passwordHash)
}

func (s *UserStore) ValidateToken(token string) (*User, bool) {
	tokenHash := hashToken(token)
	var u User
	var createdAt string

	err := s.db.QueryRow(
		`SELECT id, email, display_name, role, created_at FROM users WHERE api_token_hash = ?`, tokenHash,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &createdAt)
	if err != nil {
		return nil, false
	}

	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, true
}

func (s *UserStore) List() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, email, display_name, role, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &createdAt); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		users = append(users, u)
	}
	if users == nil {
		users = []User{}
	}
	return users, nil
}

func (s *UserStore) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// --- Crypto helpers ---

func hashPassword(password string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return hex.EncodeToString(salt) + "$" + hex.EncodeToString(hash)
}

func verifyPassword(password, stored string) bool {
	parts := splitOnce(stored, '$')
	if len(parts) != 2 {
		return false
	}
	salt, _ := hex.DecodeString(parts[0])
	expected, _ := hex.DecodeString(parts[1])
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	if len(hash) != len(expected) {
		return false
	}
	// Constant-time comparison
	result := byte(0)
	for i := range hash {
		result |= hash[i] ^ expected[i]
	}
	return result == 0
}

func generateToken() string {
	b := make([]byte, 40)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func splitOnce(s string, sep byte) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
