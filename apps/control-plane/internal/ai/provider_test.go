package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeChat serves one chat completions response and records the request.
type fakeChat struct {
	status int
	reply  string
	path   string
	query  string
	header http.Header
	body   map[string]interface{}
}

func (f *fakeChat) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.path, f.query, f.header = r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		json.NewDecoder(r.Body).Decode(&f.body)
		w.WriteHeader(f.status)
		w.Write([]byte(f.reply))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const okReply = `{"choices":[{"message":{"role":"assistant","content":"p95 doubled"},"finish_reason":"stop"}],
	"usage":{"prompt_tokens":1000,"completion_tokens":200}}`

func TestGetProviderNIM(t *testing.T) {
	p, err := GetProvider("nvidia_nim", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "nvidia_nim" {
		t.Fatalf("name = %q", p.Name())
	}
	if _, err := GetProvider("nope", ""); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

func TestNIMChat(t *testing.T) {
	f := &fakeChat{status: 200, reply: okReply}
	srv := f.start(t)

	resp, err := (&NIMProvider{BaseURL: srv.URL + "/v1/"}).Chat("nvapi-test", "z-ai/glm-5.3-flash", "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	if f.path != "/v1/chat/completions" {
		t.Errorf("path = %q", f.path)
	}
	if got := f.header.Get("Authorization"); got != "Bearer nvapi-test" {
		t.Errorf("Authorization = %q", got)
	}
	if f.body["model"] != "z-ai/glm-5.3-flash" {
		t.Errorf("model = %v", f.body["model"])
	}
	msgs, _ := f.body["messages"].([]interface{})
	if len(msgs) != 1 || msgs[0].(map[string]interface{})["content"] != "What changed?" {
		t.Errorf("messages = %v", f.body["messages"])
	}
	if resp.Content != "p95 doubled" || resp.InputTokens != 1000 || resp.OutputTokens != 200 {
		t.Errorf("response = %+v", resp)
	}
	if resp.CostUSD <= 0 {
		t.Errorf("cost = %v, want an estimate above zero so the spend cap applies", resp.CostUSD)
	}
}

func TestNIMErrorStatus(t *testing.T) {
	f := &fakeChat{status: 401, reply: `{"status":401,"title":"Unauthorized"}`}
	srv := f.start(t)

	err := (&NIMProvider{BaseURL: srv.URL}).TestKey("bad", "z-ai/glm-5.3-flash")
	if err == nil || !strings.Contains(err.Error(), "nvidia nim returned 401") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatNoAnswer(t *testing.T) {
	// A reasoning model that ran out of tokens before answering.
	f := &fakeChat{status: 200, reply: `{"choices":[{"message":{"content":null},"finish_reason":"length"}]}`}
	srv := f.start(t)

	_, err := (&NIMProvider{BaseURL: srv.URL}).Chat("k", "m", "q")
	if err == nil || !strings.Contains(err.Error(), "finish reason: length") {
		t.Fatalf("err = %v", err)
	}
}

func TestAzureUsesConfiguredEndpoint(t *testing.T) {
	f := &fakeChat{status: 200, reply: okReply}
	srv := f.start(t)

	p, _ := GetProvider("azure_openai", srv.URL+"/")
	if _, err := p.Chat("azure-key", "my-deployment", "q"); err != nil {
		t.Fatal(err)
	}
	if f.path != "/openai/deployments/my-deployment/chat/completions" || f.query != "api-version=2024-02-01" {
		t.Errorf("url = %s?%s", f.path, f.query)
	}
	if got := f.header.Get("api-key"); got != "azure-key" {
		t.Errorf("api-key = %q", got)
	}
	if _, ok := f.body["model"]; ok {
		t.Error("Azure routes by deployment; the body should not name a model")
	}
}

func TestAzureWithoutEndpoint(t *testing.T) {
	p, _ := GetProvider("azure_openai", "")
	if _, err := p.Chat("k", "d", "q"); err == nil || !strings.Contains(err.Error(), "no endpoint") {
		t.Fatalf("err = %v", err)
	}
}
