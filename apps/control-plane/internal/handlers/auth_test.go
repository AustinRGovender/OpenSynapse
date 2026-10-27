package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func call(t *testing.T, ts *testutil.TestServer, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ts.URL()+path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("Authorization", "none") // suppress the test server's automatic token
	}
	resp, err := ts.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestLocalModeRequiresInstallToken(t *testing.T) {
	ts := testutil.NewTestServer(t)

	if code, _ := call(t, ts, "GET", "/api/v1/plans", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("no token: expected 401, got %d", code)
	}
	if code, _ := call(t, ts, "GET", "/api/v1/plans", "wrong", nil); code != http.StatusUnauthorized {
		t.Fatalf("wrong token: expected 401, got %d", code)
	}
	code, session := call(t, ts, "GET", "/api/v1/auth/session", "", nil)
	if code != 200 || session["mode"] != "local" || session["token"] != testutil.LocalToken {
		t.Fatalf("session should hand the UI the install token: %d %v", code, session)
	}
	if code, _ := call(t, ts, "GET", "/api/v1/plans", testutil.LocalToken, nil); code != 200 {
		t.Fatalf("install token: expected 200, got %d", code)
	}
	// Health stays public for probes.
	if code, _ := call(t, ts, "GET", "/health", "", nil); code != 200 {
		t.Fatalf("health: expected 200, got %d", code)
	}
}

func TestTeamModeSetupLoginAndRoles(t *testing.T) {
	ts := testutil.NewTestServerWith(t, testutil.Options{TeamMode: true})

	_, session := call(t, ts, "GET", "/api/v1/auth/session", "", nil)
	if session["mode"] != "team" || session["setup_required"] != true || session["token"] != nil {
		t.Fatalf("fresh team install should require setup and expose no token: %v", session)
	}
	if code, _ := call(t, ts, "POST", "/api/v1/auth/setup", "", map[string]any{"email": "admin@example.com", "password": "short"}); code != 400 {
		t.Fatalf("weak password: expected 400, got %d", code)
	}
	code, created := call(t, ts, "POST", "/api/v1/auth/setup", "", map[string]any{"email": "Admin@Example.com", "password": "correct-horse", "display_name": "Ada"})
	if code != 201 {
		t.Fatalf("setup: %d %v", code, created)
	}
	admin := created["token"].(string)
	if code, _ := call(t, ts, "POST", "/api/v1/auth/setup", "", map[string]any{"email": "x@example.com", "password": "another-pass"}); code != 409 {
		t.Fatalf("second setup must be refused, got %d", code)
	}

	// Login works case-insensitively on email; bad passwords are refused.
	if code, _ := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"email": "admin@example.com", "password": "nope-nope"}); code != 401 {
		t.Fatalf("bad password: expected 401, got %d", code)
	}
	code, login := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"email": "ADMIN@example.com", "password": "correct-horse"})
	if code != 200 || login["token"] == nil {
		t.Fatalf("login: %d %v", code, login)
	}

	// Admin creates a viewer and an editor.
	for _, u := range []map[string]any{
		{"email": "viewer@example.com", "password": "viewer-pass", "role": "viewer"},
		{"email": "editor@example.com", "password": "editor-pass", "role": "editor"},
	} {
		if code, out := call(t, ts, "POST", "/api/v1/users", admin, u); code != 201 {
			t.Fatalf("create user: %d %v", code, out)
		}
	}
	login1 := func(email, pw string) string {
		_, out := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]any{"email": email, "password": pw})
		return out["token"].(string)
	}
	viewer := login1("viewer@example.com", "viewer-pass")
	editor := login1("editor@example.com", "editor-pass")

	plan := map[string]any{"name": "p", "root": map[string]any{"id": "r", "type": "plan", "name": "r", "enabled": true, "properties": map[string]any{}, "children": []any{}}}
	cases := []struct {
		who, method, path string
		token             string
		body              any
		want              int
	}{
		{"viewer reads plans", "GET", "/api/v1/plans", viewer, nil, 200},
		{"viewer cannot create plans", "POST", "/api/v1/plans", viewer, plan, 403},
		{"viewer can compare", "POST", "/api/v1/compare", viewer, map[string]any{"run_ids": []string{}}, 400},
		{"editor creates plans", "POST", "/api/v1/plans", editor, plan, 201},
		{"editor cannot list users", "GET", "/api/v1/users", editor, nil, 403},
		{"editor cannot change settings", "PUT", "/api/v1/settings", editor, map[string]any{}, 403},
		{"admin lists users", "GET", "/api/v1/users", admin, nil, 200},
	}
	for _, c := range cases {
		if code, out := call(t, ts, c.method, c.path, c.token, c.body); code != c.want {
			t.Errorf("%s: got %d, want %d (%v)", c.who, code, c.want, out)
		}
	}

	// Personal API tokens work until revoked.
	code, tok := call(t, ts, "POST", "/api/v1/auth/tokens", editor, map[string]any{"name": "ci"})
	if code != 201 || !strings.HasPrefix(tok["token"].(string), "osk_") {
		t.Fatalf("create token: %d %v", code, tok)
	}
	apiToken := tok["token"].(string)
	if code, _ := call(t, ts, "GET", "/api/v1/runs", apiToken, nil); code != 200 {
		t.Fatalf("API token should authenticate, got %d", code)
	}
	id := tok["api_token"].(map[string]any)["id"].(string)
	if code, _ := call(t, ts, "DELETE", "/api/v1/auth/tokens/"+id, editor, nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := call(t, ts, "GET", "/api/v1/runs", apiToken, nil); code != 401 {
		t.Fatalf("revoked token must fail, got %d", code)
	}

	// Logout ends the session.
	call(t, ts, "POST", "/api/v1/auth/logout", viewer, nil)
	if code, _ := call(t, ts, "GET", "/api/v1/plans", viewer, nil); code != 401 {
		t.Fatalf("logged-out session must fail, got %d", code)
	}

	// The last admin cannot be demoted.
	_, me := call(t, ts, "GET", "/api/v1/auth/me", admin, nil)
	if code, _ := call(t, ts, "PUT", "/api/v1/users/"+me["id"].(string), admin, map[string]any{"display_name": "Ada", "role": "viewer"}); code != 409 {
		t.Fatalf("demoting the last admin: expected 409, got %d", code)
	}
}

func TestSettingsRoundTripAndMasking(t *testing.T) {
	ts := testutil.NewTestServer(t)
	tok := testutil.LocalToken
	_, st := call(t, ts, "GET", "/api/v1/settings", tok, nil)
	if st["appearance"].(map[string]any)["theme"] != "dark" {
		t.Fatalf("defaults: %v", st)
	}
	st["integrations"] = map[string]any{"webhook_url": "https://hooks.example.com/services/T000/B000/XXXXsecret", "slack_webhook_url": ""}
	st["storage"] = map[string]any{"retention_mode": "keep_last_n", "retention_count": 5, "retention_days": 30}
	code, saved := call(t, ts, "PUT", "/api/v1/settings", tok, st)
	if code != 200 {
		t.Fatalf("save: %d %v", code, saved)
	}
	masked := saved["integrations"].(map[string]any)["webhook_url"].(string)
	if strings.Contains(masked, "XXXXsecret") || !strings.HasSuffix(masked, "cret") {
		t.Fatalf("webhook URL should be masked, got %q", masked)
	}
	// Sending the masked value back keeps the real URL.
	if code, _ := call(t, ts, "PUT", "/api/v1/settings", tok, saved); code != 200 {
		t.Fatalf("resave: %d", code)
	}
	_, again := call(t, ts, "GET", "/api/v1/settings", tok, nil)
	if again["integrations"].(map[string]any)["webhook_url"] != masked {
		t.Error("masked round-trip changed the stored URL")
	}
	bad := saved
	bad["appearance"] = map[string]any{"theme": "neon", "density": "comfortable", "palette": "default"}
	if code, _ := call(t, ts, "PUT", "/api/v1/settings", tok, bad); code != 400 {
		t.Errorf("invalid theme: expected 400, got %d", code)
	}
	if code, storage := call(t, ts, "GET", "/api/v1/storage", tok, nil); code != 200 || storage["runs"] == nil {
		t.Errorf("storage: %d %v", code, storage)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	ts := testutil.NewTestServer(t)
	call(t, ts, "GET", "/api/v1/plans", testutil.LocalToken, nil)
	resp, err := ts.Server.Client().Get(ts.URL() + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `opensynapse_http_requests_total{route="GET /api/v1/plans",method="GET",code="200"}`) {
		t.Errorf("metrics missing request counter:\n%s", body)
	}
}
