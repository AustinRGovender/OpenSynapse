package db_test

import (
	"strings"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/secrets"
)

func TestSecretsAreEncryptedAtRest(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	box, _ := secrets.Load(t.TempDir())

	envs := db.NewEnvironmentStore(database)
	envs.UseSecrets(box)
	env, err := envs.Create("prod", map[string]db.Variable{
		"BASE_URL": {Value: "https://api.example.com"},
		"TOKEN":    {Value: "s3cret-token", Secret: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	database.QueryRow(`SELECT variables FROM environments WHERE id = ?`, env.ID).Scan(&raw)
	if strings.Contains(raw, "s3cret-token") || !strings.Contains(raw, "https://api.example.com") {
		t.Fatalf("secret stored in plaintext or public value encrypted: %s", raw)
	}
	got, _ := envs.Get(env.ID)
	if got.Variables["TOKEN"].Value != "s3cret-token" {
		t.Fatalf("decrypted value = %q", got.Variables["TOKEN"].Value)
	}

	// Saving the redacted placeholder keeps the stored secret.
	vars := got.Redacted().Variables
	if _, err := envs.Update(env.ID, "prod", vars); err != nil {
		t.Fatal(err)
	}
	got, _ = envs.Get(env.ID)
	if got.Variables["TOKEN"].Value != "s3cret-token" {
		t.Fatalf("placeholder overwrote the secret: %q", got.Variables["TOKEN"].Value)
	}

	ai := db.NewAIStore(database)
	ai.UseSecrets(box)
	if err := ai.SaveConfig("openai", "sk-abcdefgh1234", "gpt", nil, true, ""); err != nil {
		t.Fatal(err)
	}
	database.QueryRow(`SELECT api_key_encrypted FROM ai_config`).Scan(&raw)
	if strings.Contains(raw, "sk-abcdefgh1234") {
		t.Fatal("AI key stored in plaintext")
	}
	if key, _ := ai.GetRawKey(); key != "sk-abcdefgh1234" {
		t.Fatalf("GetRawKey = %q", key)
	}
	cfg, _ := ai.GetConfig()
	if strings.Contains(cfg.APIKeyMasked, "abcdefgh") || !strings.HasSuffix(cfg.APIKeyMasked, "1234") {
		t.Fatalf("masked key = %q", cfg.APIKeyMasked)
	}
}
