package secrets

import (
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	b, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sealed := b.Seal("sk-live-123")
	if !IsSealed(sealed) || strings.Contains(sealed, "sk-live") {
		t.Fatalf("value not encrypted: %q", sealed)
	}
	if got, err := b.Open(sealed); err != nil || got != "sk-live-123" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if got, _ := b.Open("legacy-plaintext"); got != "legacy-plaintext" {
		t.Error("unsealed legacy values must pass through")
	}
	if b.Seal("") != "" {
		t.Error("empty stays empty")
	}
}

func TestKeyFilePersists(t *testing.T) {
	dir := t.TempDir()
	a, _ := Load(dir)
	b, _ := Load(dir)
	if got, err := b.Open(a.Seal("x")); err != nil || got != "x" {
		t.Fatal("a second Load must reuse the key file")
	}
	other, _ := Load(t.TempDir())
	if _, err := other.Open(a.Seal("x")); err == nil {
		t.Error("a different key must not decrypt")
	}
}
