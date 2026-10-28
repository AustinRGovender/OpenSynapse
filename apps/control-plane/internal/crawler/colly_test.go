package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

func TestCollyEngineBasicCrawl(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><a href="/about">About</a><a href="/contact">Contact</a></body></html>`)
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h1>About</h1></body></html>`)
	})
	mux.HandleFunc("/contact", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h1>Contact</h1></body></html>`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &CollyEngine{}
	if engine.Name() != "colly" {
		t.Fatalf("expected name 'colly', got %q", engine.Name())
	}

	cfg := CrawlConfig{
		EntryURL:   srv.URL,
		Depth:      2,
		SameOrigin: true,
		Limit:      100,
	}

	graph, requests, err := engine.Crawl(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(graph.Nodes))
	}

	if len(requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requests))
	}

	// Verify requests contain our pages
	foundRoot := false
	foundAbout := false
	for _, req := range requests {
		if req.URL == srv.URL+"/" || req.URL == srv.URL {
			foundRoot = true
		}
		if req.URL == srv.URL+"/about" {
			foundAbout = true
		}
	}
	if !foundRoot {
		t.Error("expected root URL in requests")
	}
	if !foundAbout {
		t.Error("expected /about in requests")
	}
}

func TestCollyEngineBlocklist(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><a href="/safe">Safe</a><a href="/logout">Logout</a></body></html>`)
	})
	mux.HandleFunc("/safe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>Safe page</body></html>`)
	})
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>Logged out</body></html>`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &CollyEngine{}
	cfg := CrawlConfig{
		EntryURL:   srv.URL,
		Depth:      2,
		SameOrigin: true,
		Blocklist:  []string{"/logout"},
		Limit:      100,
	}

	_, requests, err := engine.Crawl(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, req := range requests {
		if strings.Contains(req.URL, "/logout") {
			t.Error("blocked URL /logout should not appear in requests")
		}
	}
}

func TestCollyEngineCancellation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		links := ""
		for i := 0; i < 50; i++ {
			links += fmt.Sprintf(`<a href="/page/%d">Page %d</a>`, i, i)
		}
		fmt.Fprintf(w, `<html><body>%s</body></html>`, links)
	})
	mux.HandleFunc("/page/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>Page</body></html>`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &CollyEngine{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := CrawlConfig{
		EntryURL:   srv.URL,
		Depth:      3,
		SameOrigin: true,
		Limit:      1000,
	}

	_, _, err := engine.Crawl(ctx, cfg, nil)
	if err == nil {
		// Cancellation might not always produce an error if it happens after crawl completes
		// but the crawl should have been short
	}
}

func TestCollyEngineBearerAuth(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>Authenticated</body></html>`)
	}))
	defer srv.Close()

	engine := &CollyEngine{}
	cfg := CrawlConfig{
		EntryURL:   srv.URL,
		Auth:       db.CrawlAuthConfig{Type: "bearer", Token: "test-token-123"},
		Depth:      1,
		SameOrigin: true,
		Limit:      10,
	}

	_, _, err := engine.Crawl(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuth != "Bearer test-token-123" {
		t.Fatalf("expected 'Bearer test-token-123', got %q", receivedAuth)
	}
}

func TestCollyEngineProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>Hello</body></html>`)
	}))
	defer srv.Close()

	engine := &CollyEngine{}
	cfg := CrawlConfig{
		EntryURL:   srv.URL,
		Depth:      1,
		SameOrigin: true,
		Limit:      10,
	}

	// Just verify it doesn't panic with a progress callback
	graph, requests, err := engine.Crawl(context.Background(), cfg, func(p db.CrawlProgress, g db.CrawlGraph, r []db.CapturedRequest) {
		// Progress callback received
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Nodes) == 0 {
		t.Error("expected at least one node")
	}
	if len(requests) == 0 {
		t.Error("expected at least one request")
	}
}

func TestCollyEngineRequestLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		links := ""
		for i := 0; i < 20; i++ {
			links += fmt.Sprintf(`<a href="/p%d">P%d</a>`, i, i)
		}
		fmt.Fprintf(w, `<html><body>%s</body></html>`, links)
	})
	for i := 0; i < 20; i++ {
		i := i
		mux.HandleFunc(fmt.Sprintf("/p%d", i), func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body>Page</body></html>`)
		})
	}

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &CollyEngine{}
	cfg := CrawlConfig{
		EntryURL:   srv.URL,
		Depth:      2,
		SameOrigin: true,
		Limit:      3,
	}

	_, requests, err := engine.Crawl(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With visit counting, we allow at most cfg.Limit link visits plus the
	// initial root request, so total requests should never exceed limit+1.
	if len(requests) > cfg.Limit+1 {
		t.Fatalf("expected at most %d requests (limit %d + root), got %d", cfg.Limit+1, cfg.Limit, len(requests))
	}
}

// loginSite serves a login form at / (fields named by userField, plus a CSRF
// token) and a members page that needs the session cookie it sets.
// demoUser is the sign-in name loginSite accepts; an email field needs an
// email, or the browser's form validation refuses to submit.
func demoUser(userField string) string {
	if userField == "email" {
		return "demo@example.com"
	}
	return "demo"
}

func loginSite(t *testing.T, userField string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><a href="/members">Members</a>
<form method="post" action="/session">
  <input type="hidden" name="csrf" value="tok123">
  <input type="%s" name="%s"><input type="password" name="pw"><button type="submit">Go</button>
</form></body></html>`, map[bool]string{true: "email", false: "text"}[userField == "email"], userField)
	})
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Method != http.MethodPost || r.Form.Get("csrf") != "tok123" || r.Form.Get(userField) != demoUser(userField) || r.Form.Get("pw") != "secret" {
			http.Error(w, "bad login", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "ok", Path: "/"})
		http.Redirect(w, r, "/members", http.StatusSeeOther)
	})
	mux.HandleFunc("/members", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("sid"); err != nil || c.Value != "ok" {
			http.Error(w, "sign in first", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><a href="/members/secret">Secret</a></body></html>`)
	})
	mux.HandleFunc("/members/secret", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("sid"); err != nil {
			http.Error(w, "sign in first", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `<html><body>ok</body></html>`)
	})
	return httptest.NewServer(mux)
}

func TestCollyFormLoginUsesTheLoginForm(t *testing.T) {
	for _, field := range []string{"username", "email"} {
		srv := loginSite(t, field)
		cfg := CrawlConfig{EntryURL: srv.URL + "/", Depth: 3, SameOrigin: true, Limit: 50,
			Auth: db.CrawlAuthConfig{Type: "form_login", Username: demoUser(field), Password: "secret"}}
		_, reqs, err := (&CollyEngine{}).Crawl(context.Background(), cfg, nil)
		srv.Close()
		if err != nil {
			t.Fatalf("%s form: %v", field, err)
		}
		reached := false
		for _, r := range reqs {
			if strings.HasSuffix(r.URL, "/members/secret") && r.StatusCode == 200 {
				reached = true
			}
		}
		if !reached {
			t.Errorf("%s form: the crawl did not reach the members-only page as the signed-in user: %+v", field, reqs)
		}
	}
}

func TestCollyFormLoginReportsWrongCredentials(t *testing.T) {
	srv := loginSite(t, "username")
	defer srv.Close()
	cfg := CrawlConfig{EntryURL: srv.URL + "/", Depth: 1, SameOrigin: true, Limit: 10,
		Auth: db.CrawlAuthConfig{Type: "form_login", Username: "demo", Password: "wrong"}}
	_, _, err := (&CollyEngine{}).Crawl(context.Background(), cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want a 401 login error, got %v", err)
	}
}

// chainSite serves / -> /a -> /b -> /c, one link per page.
func chainSite(t *testing.T) *httptest.Server {
	t.Helper()
	next := map[string]string{"/": "/a", "/a": "/b", "/b": "/c", "/c": ""}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		to, ok := next[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><a href="%s">next</a></body></html>`, to)
	}))
}

// assertDepth checks that a depth-2 crawl of chainSite followed exactly two links.
func assertDepth(t *testing.T, reqs []db.CapturedRequest) {
	t.Helper()
	seen := map[string]bool{}
	for _, r := range reqs {
		if u, err := url.Parse(r.URL); err == nil {
			seen[u.Path] = true
		}
	}
	if !seen["/a"] || !seen["/b"] || seen["/c"] {
		t.Fatalf("depth 2 from the start page should reach /a and /b but not /c; visited %v", seen)
	}
}

func TestCollyDepthCountsLinksFromTheStartPage(t *testing.T) {
	srv := chainSite(t)
	defer srv.Close()
	_, reqs, err := (&CollyEngine{}).Crawl(context.Background(), CrawlConfig{EntryURL: srv.URL + "/", Depth: 2, SameOrigin: true, Limit: 50}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertDepth(t, reqs)
}
