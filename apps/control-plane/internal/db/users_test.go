package db

import (
	"testing"
)

func TestUserCreateAndValidate(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := NewUserStore(database)

	// Create user
	user, token, err := store.Create("test@example.com", "Test User", "secureP@ss123", "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if user.Email != "test@example.com" {
		t.Fatalf("expected email test@example.com, got %s", user.Email)
	}
	if user.Role != "admin" {
		t.Fatalf("expected role admin, got %s", user.Role)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Validate password
	validated, ok := store.ValidatePassword("test@example.com", "secureP@ss123")
	if !ok {
		t.Fatal("expected password validation to succeed")
	}
	if validated.ID != user.ID {
		t.Fatal("expected same user ID")
	}

	// Wrong password
	_, ok = store.ValidatePassword("test@example.com", "wrongpassword")
	if ok {
		t.Fatal("expected password validation to fail with wrong password")
	}

	// Validate token
	validated, ok = store.ValidateToken(token)
	if !ok {
		t.Fatal("expected token validation to succeed")
	}
	if validated.ID != user.ID {
		t.Fatal("expected same user ID from token")
	}

	// Wrong token
	_, ok = store.ValidateToken("invalid-token")
	if ok {
		t.Fatal("expected token validation to fail")
	}
}

func TestUserGetByEmail(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := NewUserStore(database)
	store.Create("alice@example.com", "Alice", "pass123", "editor")

	user, err := store.GetByEmail("alice@example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if user == nil {
		t.Fatal("expected user")
	}
	if user.DisplayName != "Alice" {
		t.Fatalf("expected Alice, got %s", user.DisplayName)
	}

	// Non-existent
	user, err = store.GetByEmail("nobody@example.com")
	if err != nil {
		t.Fatalf("get non-existent: %v", err)
	}
	if user != nil {
		t.Fatal("expected nil for non-existent email")
	}
}

func TestUserList(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := NewUserStore(database)
	store.Create("a@example.com", "A", "pass", "admin")
	store.Create("b@example.com", "B", "pass", "editor")

	users, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestUserInvalidRole(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := NewUserStore(database)
	_, _, err = store.Create("bad@example.com", "Bad", "pass", "superadmin")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}
