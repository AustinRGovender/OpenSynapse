package crawler

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	localbrowser "github.com/opensynapse/opensynapse/apps/control-plane/internal/browser"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/ysmood/gson"
)

// RodEngine implements CrawlEngine using headless Chromium via the Rod library.
// It can render JavaScript and capture all network requests.
type RodEngine struct{}

func (e *RodEngine) Name() string { return "rod" }

func (e *RodEngine) Crawl(ctx context.Context, cfg CrawlConfig, onProgress ProgressCallback) (db.CrawlGraph, []db.CapturedRequest, error) {
	browser, cleanup, err := localbrowser.Launch()
	if err != nil {
		return db.CrawlGraph{}, nil, fmt.Errorf("launch browser: %w", err)
	}
	defer cleanup()

	// Chrome drops session cookies, such as a form login's, when its last tab
	// closes; this blank tab keeps the session open between pages.
	keepAlive, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return db.CrawlGraph{}, nil, fmt.Errorf("open browser tab: %w", err)
	}
	defer keepAlive.Close()

	var mu sync.Mutex
	var nodes []db.CrawlGraphNode
	var edges []db.CrawlGraphEdge
	var requests []db.CapturedRequest
	visited := map[string]bool{}

	type queueItem struct {
		url   string
		depth int
	}
	queue := []queueItem{{url: cfg.EntryURL, depth: 0}}

	// Progress reporting
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
	defer close(done)

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return db.CrawlGraph{}, nil, ctx.Err()
		default:
		}

		item := queue[0]
		queue = queue[1:]

		mu.Lock()
		if visited[item.url] {
			mu.Unlock()
			continue
		}
		visited[item.url] = true
		visitCount := len(visited)
		mu.Unlock()

		if cfg.Limit > 0 && visitCount > cfg.Limit {
			break
		}

		pageRequests, links, title, err := e.visitPage(browser, ctx, cfg, item.url, item.depth == 0)
		if err != nil {
			continue
		}

		mu.Lock()
		nodes = append(nodes, db.CrawlGraphNode{URL: item.url, Title: title})
		requests = append(requests, pageRequests...)
		mu.Unlock()

		if item.depth < cfg.Depth {
			for _, href := range links {
				normalized := NormalizeURL(item.url, href)
				if normalized == "" {
					continue
				}

				mu.Lock()
				alreadyVisited := visited[normalized]
				count := len(visited)
				mu.Unlock()

				if alreadyVisited {
					continue
				}
				if !ShouldVisit(cfg, cfg.EntryURL, normalized, count) {
					continue
				}

				mu.Lock()
				edges = append(edges, db.CrawlGraphEdge{Source: item.url, Target: normalized})
				mu.Unlock()

				queue = append(queue, queueItem{url: normalized, depth: item.depth + 1})
			}
		}
	}

	mu.Lock()
	defer mu.Unlock()

	graph := db.CrawlGraph{
		Nodes: ensureNodes(nodes),
		Edges: ensureEdges(edges),
	}
	return graph, ensureRequests(requests), nil
}

// visitPage navigates to a URL, captures network requests, and extracts links.
func (e *RodEngine) visitPage(browser *rod.Browser, ctx context.Context, cfg CrawlConfig, pageURL string, isFirst bool) ([]db.CapturedRequest, []string, string, error) {
	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, nil, "", err
	}
	defer page.Close()

	var mu sync.Mutex
	var captured []db.CapturedRequest

	// Track request methods by request ID
	methodMap := &sync.Map{}

	record := func(requestID proto.NetworkRequestID, resp *proto.NetworkResponse) {
		// The browser fetches /favicon.ico on its own; it isn't the site's traffic.
		if u, err := url.Parse(resp.URL); err == nil && u.Path == "/favicon.ico" {
			return
		}
		method := "GET"
		if m, ok := methodMap.Load(string(requestID)); ok {
			method = m.(string)
		}
		headers := make(map[string]string)
		for k, v := range resp.Headers {
			headers[k] = fmt.Sprintf("%v", v)
		}
		mu.Lock()
		captured = append(captured, db.CapturedRequest{
			Method:     strings.ToUpper(method),
			URL:        resp.URL,
			StatusCode: resp.Status,
			Headers:    headers,
		})
		mu.Unlock()
	}

	// Listen for network events
	go page.EachEvent(
		func(ev *proto.NetworkRequestWillBeSent) {
			// A redirect (such as the 303 after a form post) gets no
			// ResponseReceived; it arrives with the request that follows it.
			if ev.RedirectResponse != nil {
				record(ev.RequestID, ev.RedirectResponse)
			}
			methodMap.Store(string(ev.RequestID), ev.Request.Method)
		},
		func(ev *proto.NetworkResponseReceived) {
			record(ev.RequestID, ev.Response)
		},
	)()

	_ = proto.NetworkEnable{}.Call(page)

	// Set auth headers if needed
	if cfg.Auth.Type == "bearer" {
		_ = proto.NetworkSetExtraHTTPHeaders{
			Headers: proto.NetworkHeaders{
				"Authorization": gson.New("Bearer " + cfg.Auth.Token),
			},
		}.Call(page)
	} else if cfg.Auth.Type == "basic" {
		encoded := base64Encode(cfg.Auth.Username + ":" + cfg.Auth.Password)
		_ = proto.NetworkSetExtraHTTPHeaders{
			Headers: proto.NetworkHeaders{
				"Authorization": gson.New("Basic " + encoded),
			},
		}.Call(page)
	}

	if err := page.Navigate(pageURL); err != nil {
		return nil, nil, "", err
	}
	if err := page.WaitLoad(); err != nil {
		return nil, nil, "", err
	}

	// Form login on first page
	if isFirst && cfg.Auth.Type == "form_login" {
		rodFormLogin(page, cfg)
		time.Sleep(2 * time.Second)
		_ = page.WaitLoad()
	}

	// Get title
	titleStr := pageURL
	if result, err := page.Eval(`() => document.title`); err == nil && result != nil {
		if t := result.Value.Str(); t != "" {
			titleStr = t
		}
	}

	// Extract links
	var links []string
	if result, err := page.Eval(`() => Array.from(document.querySelectorAll('a[href]')).map(a => a.href).filter(h => h && !h.startsWith('javascript:'))`); err == nil && result != nil {
		for _, v := range result.Value.Arr() {
			links = append(links, v.Str())
		}
	}

	// Small delay to let in-flight requests finish
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	return captured, links, titleStr, nil
}

func rodFormLogin(page *rod.Page, cfg CrawlConfig) {
	usernameSelectors := []string{
		"input[name='username']", "input[name='email']",
		"input[type='email']", "input[name='user']", "input[name='login']",
	}
	passwordSelectors := []string{
		"input[type='password']", "input[name='password']",
	}
	submitSelectors := []string{
		"button[type='submit']", "input[type='submit']",
	}

	// Has looks once; Element would wait for a missing field indefinitely.
	first := func(selectors []string) *rod.Element {
		for _, sel := range selectors {
			if ok, el, err := page.Has(sel); err == nil && ok {
				return el
			}
		}
		return nil
	}
	if el := first(usernameSelectors); el != nil {
		el.Input(cfg.Auth.Username)
	}
	if el := first(passwordSelectors); el != nil {
		el.Input(cfg.Auth.Password)
	}
	if el := first(submitSelectors); el != nil {
		el.Click(proto.InputMouseButtonLeft, 1)
	}
}
