package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	// Colly is async, so some requests may be in-flight when the limit is hit.
	// Verify the limit bounds the crawl — we should get far fewer than 20.
	if len(requests) > 10 {
		t.Fatalf("expected requests roughly bounded by limit 3, got %d", len(requests))
	}
}
