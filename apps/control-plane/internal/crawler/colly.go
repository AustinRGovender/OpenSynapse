package crawler

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// CollyEngine implements CrawlEngine using the Colly HTTP crawler.
// It is fast and lightweight but cannot render JavaScript.
type CollyEngine struct{}

func (e *CollyEngine) Name() string { return "colly" }

func (e *CollyEngine) Crawl(ctx context.Context, cfg CrawlConfig, onProgress ProgressCallback) (db.CrawlGraph, []db.CapturedRequest, error) {
	var mu sync.Mutex
	var nodes []db.CrawlGraphNode
	var edges []db.CrawlGraphEdge
	var requests []db.CapturedRequest
	visited := map[string]bool{}

	c := colly.NewCollector(
		colly.MaxDepth(cfg.Depth),
		colly.Async(true),
	)
	c.SetRequestTimeout(15 * time.Second)

	if cfg.SameOrigin {
		if host := extractHost(cfg.EntryURL); host != "" {
			c.AllowedDomains = []string{host}
		}
	}

	// Auth: set headers on every request
	c.OnRequest(func(r *colly.Request) {
		select {
		case <-ctx.Done():
			r.Abort()
			return
		default:
		}

		mu.Lock()
		count := len(requests)
		mu.Unlock()
		if cfg.Limit > 0 && count >= cfg.Limit {
			r.Abort()
			return
		}

		switch cfg.Auth.Type {
		case "bearer":
			r.Headers.Set("Authorization", "Bearer "+cfg.Auth.Token)
		case "basic":
			r.Headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cfg.Auth.Username+":"+cfg.Auth.Password)))
		}
	})

	// Capture responses
	c.OnResponse(func(r *colly.Response) {
		mu.Lock()
		defer mu.Unlock()

		reqURL := r.Request.URL.String()
		requests = append(requests, db.CapturedRequest{
			Method:     r.Request.Method,
			URL:        reqURL,
			Headers:    flattenHeaders(r.Headers),
			StatusCode: r.StatusCode,
		})

		if !visited[reqURL] {
			visited[reqURL] = true
			nodes = append(nodes, db.CrawlGraphNode{URL: reqURL, Title: reqURL})
		}
	})

	// Discover links
	c.OnHTML("a[href]", func(el *colly.HTMLElement) {
		href := NormalizeURL(el.Request.URL.String(), el.Attr("href"))
		if href == "" {
			return
		}

		mu.Lock()
		count := len(requests)
		mu.Unlock()

		if !ShouldVisit(cfg, cfg.EntryURL, href, count) {
			return
		}

		mu.Lock()
		edges = append(edges, db.CrawlGraphEdge{
			Source: el.Request.URL.String(),
			Target: href,
		})
		mu.Unlock()

		el.Request.Visit(href)
	})

	// Progress reporting goroutine
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				if onProgress != nil {
					onProgress(
						db.CrawlProgress{PagesDiscovered: len(nodes), RequestsCaptured: len(requests)},
						db.CrawlGraph{Nodes: copyNodes(nodes), Edges: copyEdges(edges)},
						copyRequests(requests),
					)
				}
				mu.Unlock()
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Handle form login before crawling
	if cfg.Auth.Type == "form_login" {
		err := c.Post(cfg.EntryURL, map[string]string{
			"username": cfg.Auth.Username,
			"password": cfg.Auth.Password,
		})
		if err != nil {
			close(done)
			return db.CrawlGraph{}, nil, fmt.Errorf("form login failed: %w", err)
		}
	}

	c.Visit(cfg.EntryURL)
	c.Wait()
	close(done)

	if ctx.Err() != nil {
		return db.CrawlGraph{}, nil, ctx.Err()
	}

	mu.Lock()
	defer mu.Unlock()

	graph := db.CrawlGraph{
		Nodes: ensureNodes(nodes),
		Edges: ensureEdges(edges),
	}
	return graph, ensureRequests(requests), nil
}

func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func flattenHeaders(h *http.Header) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string)
	for k, v := range *h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

func copyNodes(n []db.CrawlGraphNode) []db.CrawlGraphNode {
	c := make([]db.CrawlGraphNode, len(n))
	copy(c, n)
	return c
}

func copyEdges(e []db.CrawlGraphEdge) []db.CrawlGraphEdge {
	c := make([]db.CrawlGraphEdge, len(e))
	copy(c, e)
	return c
}

func copyRequests(r []db.CapturedRequest) []db.CapturedRequest {
	c := make([]db.CapturedRequest, len(r))
	copy(c, r)
	return c
}

func ensureNodes(n []db.CrawlGraphNode) []db.CrawlGraphNode {
	if n == nil {
		return []db.CrawlGraphNode{}
	}
	return n
}

func ensureEdges(e []db.CrawlGraphEdge) []db.CrawlGraphEdge {
	if e == nil {
		return []db.CrawlGraphEdge{}
	}
	return e
}

func ensureRequests(r []db.CapturedRequest) []db.CapturedRequest {
	if r == nil {
		return []db.CapturedRequest{}
	}
	return r
}
