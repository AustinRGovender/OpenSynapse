package crawler

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strings"
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
	visitCount := 0 // tracks dispatched visits (not completed responses)

	// Depth counts links followed from the start page; Colly counts the start
	// page itself as depth 1.
	maxDepth := cfg.Depth
	if maxDepth > 0 {
		maxDepth++
	}
	c := colly.NewCollector(
		colly.MaxDepth(maxDepth),
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
		count := visitCount
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
		count := visitCount
		mu.Unlock()

		if !ShouldVisit(cfg, cfg.EntryURL, href, count) {
			return
		}

		mu.Lock()
		visitCount++
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

	// Sign in through the start page's login form before crawling.
	if cfg.Auth.Type == "form_login" {
		if err := collyFormLogin(c, cfg); err != nil {
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

// userFieldNames are input names taken to be the login's username field.
var userFieldNames = []string{"username", "user", "login", "email", "user_name", "userid"}

// collyFormLogin signs in through the login form on the start page: it fills
// the username (or email) and password fields, keeps the form's other fields
// such as CSRF tokens, and posts to the form's action. The crawling collector
// shares the cookie jar, so it crawls as the signed-in user.
func collyFormLogin(c *colly.Collector, cfg CrawlConfig) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	c.SetCookieJar(jar)
	// A separate collector, so fetching the start page here does not mark it
	// visited for the crawl itself.
	login := colly.NewCollector()
	login.SetCookieJar(jar)
	login.SetRequestTimeout(15 * time.Second)

	var action string
	fields := map[string]string{}
	found := false
	login.OnHTML("form", func(e *colly.HTMLElement) {
		if found || e.DOM.Find("input[type=password]").Length() == 0 {
			return
		}
		found = true
		action = e.Request.URL.String()
		if a := strings.TrimSpace(e.Attr("action")); a != "" {
			action = e.Request.AbsoluteURL(a)
		}
		userSet := false
		e.ForEach("input", func(_ int, in *colly.HTMLElement) {
			name := in.Attr("name")
			if name == "" {
				return
			}
			typ := strings.ToLower(in.Attr("type"))
			switch {
			case typ == "password":
				fields[name] = cfg.Auth.Password
			case !userSet && (typ == "email" || slices.Contains(userFieldNames, strings.ToLower(name))):
				fields[name] = cfg.Auth.Username
				userSet = true
			case typ != "submit" && typ != "button" && typ != "checkbox" && typ != "radio":
				fields[name] = in.Attr("value")
			}
		})
	})
	status := 0
	login.OnResponse(func(r *colly.Response) { status = r.StatusCode })
	login.OnError(func(r *colly.Response, _ error) { status = r.StatusCode })

	if err := login.Visit(cfg.EntryURL); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no login form (a form with a password field) on %s", cfg.EntryURL)
	}
	if err := login.Post(action, fields); err != nil && status < 400 {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("the login form answered %d; check the username and password", status)
	}
	return nil
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
