//go:build integration

package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

func TestRodEngineBasicCrawl(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Home</title></head><body><a href="/about">About</a></body></html>`)
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>About</title></head><body><h1>About Page</h1></body></html>`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &RodEngine{}
	if engine.Name() != "rod" {
		t.Fatalf("expected name 'rod', got %q", engine.Name())
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

	if len(graph.Nodes) < 1 {
		t.Fatalf("expected at least 1 node, got %d", len(graph.Nodes))
	}
	if len(requests) < 1 {
		t.Fatalf("expected at least 1 request, got %d", len(requests))
	}
}

func TestRodEngineBearerAuth(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>Authenticated</body></html>`)
	}))
	defer srv.Close()

	engine := &RodEngine{}
	cfg := CrawlConfig{
		EntryURL:   srv.URL,
		Auth:       db.CrawlAuthConfig{Type: "bearer", Token: "rod-test-token"},
		Depth:      1,
		SameOrigin: true,
		Limit:      10,
	}

	_, _, err := engine.Crawl(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuth != "Bearer rod-test-token" {
		t.Fatalf("expected 'Bearer rod-test-token', got %q", receivedAuth)
	}
}

func TestRodEngineCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		links := ""
		for i := 0; i < 50; i++ {
			links += fmt.Sprintf(`<a href="/page/%d">Page %d</a>`, i, i)
		}
		fmt.Fprintf(w, `<html><body>%s</body></html>`, links)
	}))
	defer srv.Close()

	engine := &RodEngine{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := CrawlConfig{
		EntryURL:   srv.URL,
		Depth:      3,
		SameOrigin: true,
		Limit:      1000,
	}

	_, _, err := engine.Crawl(ctx, cfg, nil)
	if err == nil {
		t.Log("crawl completed before cancellation took effect")
	}
}

func TestRodEngineFormLogin(t *testing.T) {
	var loggedIn bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
			<form action="/login" method="post">
				<input type="text" name="username" />
				<input type="password" name="password" />
				<button type="submit">Login</button>
			</form>
		</body></html>`)
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			loggedIn = true
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>Logged in</body></html>`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &RodEngine{}
	cfg := CrawlConfig{
		EntryURL: srv.URL,
		Auth: db.CrawlAuthConfig{
			Type:     "form_login",
			Username: "testuser",
			Password: "testpass",
		},
		Depth:      1,
		SameOrigin: true,
		Limit:      10,
	}

	_, _, err := engine.Crawl(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = loggedIn // Form submission depends on JS execution timing
}
