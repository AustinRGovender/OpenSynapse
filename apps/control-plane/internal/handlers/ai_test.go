package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func TestAIGetConfigDefault(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL()+"/api/v1/ai/config", nil)
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get config: expected 200, got %d", resp.StatusCode)
	}

	var config map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&config)
	resp.Body.Close()

	// Default should be empty/disabled
	if config["enabled"] == true {
		t.Fatal("expected AI disabled by default")
	}
}

func TestAIUpdateConfig(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"provider":"openai","api_key":"sk-test1234567890","model":"gpt-4o","monthly_cap_usd":10.0,"enabled":true}`
	req, _ := http.NewRequest("PUT", ts.URL()+"/api/v1/ai/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update config: expected 200, got %d", resp.StatusCode)
	}

	var config map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&config)
	resp.Body.Close()

	if config["provider"] != "openai" {
		t.Fatalf("expected provider 'openai', got %v", config["provider"])
	}
	if config["api_key_masked"] != "****7890" {
		t.Fatalf("expected masked key '****7890', got %v", config["api_key_masked"])
	}
	if config["enabled"] != true {
		t.Fatal("expected enabled=true")
	}
}

func TestAIKeyMasking(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Save a key
	body := `{"provider":"anthropic","api_key":"sk-ant-abcdefghijklmnop","model":"claude-sonnet-4-20250514","enabled":true}`
	req, _ := http.NewRequest("PUT", ts.URL()+"/api/v1/ai/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	resp.Body.Close()

	// Read it back — key should be masked
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/ai/config", nil)
	resp = ts.Do(req)

	var config map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&config)
	resp.Body.Close()

	masked := config["api_key_masked"].(string)
	if masked != "****mnop" {
		t.Fatalf("expected masked key '****mnop', got %q", masked)
	}

	// Full key should never appear in the GET response
	respBytes, _ := json.Marshal(config)
	if bytes.Contains(respBytes, []byte("sk-ant-abcdefghijklmnop")) {
		t.Fatal("full API key leaked in GET response")
	}
}

func TestAITestConfigNotConfigured(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/ai/config/test", nil)
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("test config: expected 400, got %d", resp.StatusCode)
	}

	var errResp struct {
		Error struct{ Code string } `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&errResp)
	resp.Body.Close()

	if errResp.Error.Code != "AI_NOT_CONFIGURED" {
		t.Fatalf("expected AI_NOT_CONFIGURED, got %q", errResp.Error.Code)
	}
}

func TestAIAnalyseNotEnabled(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"run_id":"some-run","question":"What happened?"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/ai/analyse", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("analyse: expected 400, got %d", resp.StatusCode)
	}

	var errResp struct {
		Error struct{ Code string } `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&errResp)
	resp.Body.Close()

	if errResp.Error.Code != "AI_NOT_ENABLED" {
		t.Fatalf("expected AI_NOT_ENABLED, got %q", errResp.Error.Code)
	}
}

func TestAIAnalyseMissingIDs(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"question":"What happened?"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/ai/analyse", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("analyse: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
