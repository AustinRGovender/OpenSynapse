# ADR-0003: Multi-engine crawler with Rod, Colly, and ZAP

**Date:** 2026-04-15
**Status:** accepted
**Authors:** OpenSynapse team

## Context

The PRD specifies Playwright for crawling, but the Phase 7 implementation only stubbed the crawl execution — the handler immediately marks crawls as completed with 0 pages and 0 requests. Only the OpenAPI import path was functional. We need a real crawler that can:

- Navigate JavaScript-rendered SPAs and capture all network calls
- Log into web applications with test credentials before crawling
- Support fast HTTP-only crawling for traditional server-rendered sites
- Optionally integrate with security testing tools

Different use cases call for different tools: SPAs need a real browser, static sites benefit from pure-HTTP speed, and security teams want integration with established tools like OWASP ZAP.

## Decision

Implement three crawl engines behind a common `CrawlEngine` Go interface, selectable by the user at crawl time:

1. **Rod** (go-rod/rod) — headless Chromium via DevTools Protocol. Renders JavaScript, intercepts all network requests (XHR, fetch, document loads), and supports form login by filling and submitting login forms. This replaces the PRD's Playwright specification because Rod has more mature Go bindings and a simpler API.

2. **Colly** (gocolly/colly) — pure Go HTTP crawler. Fast, lightweight, zero browser dependencies. Discovers links via HTML parsing. Supports bearer/basic auth via request headers and form login via POST. Cannot render JavaScript.

3. **OWASP ZAP** (REST API sidecar) — communicates with a ZAP instance via its REST API. ZAP runs as a separate Docker container. Provides security-focused crawling with built-in vulnerability scanning capabilities.

The handler dispatches to the selected engine in a background goroutine, and each engine reports progress via a callback so the frontend's 2-second polling cycle works.

## Alternatives considered

- **Playwright-go** — The PRD's original choice. Rejected because Go bindings are less mature than Rod's, and the Node.js dependency adds complexity to the Docker image.
- **chromedp** — Lower-level Chrome DevTools Protocol library. Requires significantly more boilerplate for common operations like form filling and network interception.
- **Katana** (ProjectDiscovery) — CLI-only tool, not embeddable as a Go library. Would require subprocess management.
- **Single-engine approach** — Using only Rod would cover most cases but penalizes simple static sites with unnecessary browser overhead, and doesn't serve security testing workflows.

## Consequences

- Users can select the right tool for their target application (SPA → Rod, static → Colly, security → ZAP).
- Rod replaces Playwright as the browser-based engine, deviating from the PRD. This ADR documents the deviation.
- ZAP requires a separate sidecar container, adding Docker Compose complexity. The ZAP service is behind a `profiles: [security]` gate so it doesn't start by default.
- Colly cannot handle SPAs — this limitation is documented in the UI with clear engine descriptions.
- The `CrawlEngine` interface makes it straightforward to add future engines (e.g., a raw HTTP client for API-only targets).
- Rod requires Chromium packages in the Docker image, adding ~200MB to the control plane container.
