package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func TestFragmentListIncludesBuiltIns(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL()+"/api/v1/fragments", nil)
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	items := result["items"].([]interface{})
	if len(items) < 10 {
		t.Fatalf("expected at least 10 built-in fragments, got %d", len(items))
	}

	// Verify first fragment is built-in
	first := items[0].(map[string]interface{})
	if first["built_in"] != true {
		t.Fatal("expected first fragment to be built-in")
	}
}

func TestFragmentUserCRUD(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create user fragment
	body := `{"name":"My Login","description":"Custom login flow","tags":["custom"],"node_subtree":{"id":"n1","type":"http","name":"Login","enabled":true,"properties":{},"children":[]},"bindings":[{"name":"USER","description":"Username","required":true}]}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/fragments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	fragID := created["id"].(string)
	if created["name"] != "My Login" {
		t.Fatalf("expected name 'My Login', got %v", created["name"])
	}
	if created["built_in"] != false {
		t.Fatal("user fragment should not be built-in")
	}

	// Get
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/fragments/"+fragID, nil)
	resp = ts.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete
	req, _ = http.NewRequest("DELETE", ts.URL()+"/api/v1/fragments/"+fragID, nil)
	resp = ts.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}
}

func TestFragmentCannotDeleteBuiltIn(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// List to get a built-in ID
	req, _ := http.NewRequest("GET", ts.URL()+"/api/v1/fragments", nil)
	resp := ts.Do(req)
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	items := result["items"].([]interface{})
	builtInID := ""
	for _, item := range items {
		frag := item.(map[string]interface{})
		if frag["built_in"] == true {
			builtInID = frag["id"].(string)
			break
		}
	}

	if builtInID == "" {
		t.Fatal("no built-in fragment found")
	}

	// Try to delete
	req, _ = http.NewRequest("DELETE", ts.URL()+"/api/v1/fragments/"+builtInID, nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("delete built-in: expected 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
