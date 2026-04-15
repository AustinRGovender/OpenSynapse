# OpenSynapse — Product Requirements Document

**Version:** 1.0
**Status:** Draft for implementation
**Owner:** Austin Govender
**Target audience:** Claude Code (implementation), performance engineers (users)

---

## 1. Vision

OpenSynapse is a self-hosted performance testing platform that blends the scripting power of k6, the visual test planning of JMeter, the approachable UX of BlazeMeter, and the endpoint-exploration ergonomics of Postman into a single installable product. It runs on a laptop, on a server via Docker, or on a Kubernetes cluster with the same codebase and the same user interface. It ships as a single installer per platform and requires no external SaaS account.

The product exists because current tools force a trade-off. JMeter gives you visual test building but a dated UI and heavy resource footprint. k6 is efficient and modern but code-only and cannot be modified mid-run without restart. BlazeMeter is polished but cloud-locked and expensive. Postman is ergonomic for endpoints but not a load tool. OpenSynapse takes the best of each without inheriting their limitations.

## 2. Target users

The primary user is a QA engineer, SRE, or performance specialist who needs to run realistic load tests against web applications and APIs without building infrastructure from scratch. The secondary user is a developer who wants a Postman-like experience for validating endpoints and occasionally running small load tests locally. The tertiary user is a performance lead who needs to compare runs over time to prove regressions or improvements to stakeholders.

Users are assumed to be technically competent but not necessarily fluent in JavaScript, Go, or any specific DSL. The UI must be usable without writing code for the majority of tasks, with code available as an advanced option.

## 3. Guiding principles

OpenSynapse prioritises correctness of measurement over feature breadth. A test that reports wrong numbers is worse than a test that cannot be built. The product should refuse to run a test it cannot measure accurately rather than quietly produce misleading results.

Everything visible in the UI must be achievable via the REST API, and everything in the API must be scriptable. No feature is UI-only.

Installation is one command. Setup is one click. If a dependency cannot be bundled, the installer fetches it automatically with a progress indicator and a clearly stated fallback if offline.

The product is self-contained. It does not phone home, does not require an account, and does not degrade when offline. AI features are opt-in and use keys the user supplies.

## 4. Scope summary

In scope for v1: HTTP and HTTPS load generation, WebSocket support, visual test plan builder, pre-built test templates, live test control, real-time dashboards, run history and comparison, crawler-based test generation, endpoint playground, if/else logic, thresholds and assertions, local and distributed execution, AI-assisted analysis with bring-your-own key, export to PDF/HTML/CSV/JSON, single-installer deployment for Mac, Windows, Linux, Docker, and Kubernetes.

Out of scope for v1: gRPC (planned v1.1), mobile app testing, SAP or Citrix protocol support, LoadRunner script import, multi-tenant SaaS hosting, user authentication beyond single-user or basic team mode, commercial licensing.

## 5. Core features

### 5.1 Test template gallery

When a user clicks "New test", they see a gallery of test archetypes rather than a blank canvas. Each card shows the name, a short description, a small animated load-curve graphic, and a "Use template" button. The archetypes are: smoke (minimal traffic to verify the test works), load (sustained typical traffic), stress (push beyond normal to find the knee), spike (sudden burst and drop), soak (long-duration stability, hours), breakpoint (slowly ramp until failure), trickle feed (very low constant rate for endurance), ramp-up (staged linear increase), and step-load (increase in discrete plateaus). Each template seeds the test plan with sensible defaults and an explanatory note on what the test is trying to answer. The user can then modify anything.

The graphics are SVG animations of the load profile over time: a flat line for soak, a stair for step-load, a single tall pulse for spike, and so on. They are generated once and cached.

### 5.2 Visual test plan builder

A test plan is a tree of nodes. The root is the plan itself. Children include scenarios, thread groups, controllers (if, else, loop, transaction, once-only), samplers (HTTP request, WebSocket, custom code block), timers, assertions, and data sources (CSV, JSON, inline). The builder is drag-and-drop, with a properties panel on the right for the selected node. The tree mirrors JMeter's mental model but with modern ergonomics: inline validation, search across all nodes, multi-select, copy-paste between plans.

Every visual change produces a serialised plan file (JSON schema defined in section 7). The plan file is the source of truth. The visual builder and the code view are two projections of the same file; editing either updates the other.

### 5.3 Endpoint playground

A Postman-style pane lets the user construct and execute a single request without running a full test. It supports all HTTP methods, custom headers, multiple body types, authentication helpers (Basic, Bearer, API key, OAuth 2 code flow), and environment variables. Responses are shown with pretty-printing, header inspection, and timing breakdown (DNS, connect, TLS, send, wait, receive). A "Save to plan" button inserts the request as a sampler node in the active test plan. A "Run as micro-test" button executes the request N times with configurable concurrency for quick smoke-testing before committing to a full run.

### 5.4 Crawler

The crawler is the feature that distinguishes OpenSynapse from every competitor. The user provides an entry URL, selects a crawl engine, and optionally provides credentials (form login, HTTP basic, or a bearer token). Three engines are available: Rod (headless Chromium via DevTools Protocol for SPAs and JS-heavy apps), Colly (fast pure-Go HTTP crawler for static sites), and OWASP ZAP (security-focused sidecar). The selected engine performs the login and then walks the application. It records every network request in the background and every link it can reach from the landing page. It respects a configurable depth limit, a same-origin restriction by default, and a path blocklist for destructive actions (delete, logout, DELETE verbs on REST by default). See ADR-0003 for the engine selection rationale.

The crawler produces a directed graph of pages and endpoints. It performs automatic correlation by watching for tokens, session IDs, CSRF values, and IDs that appear in one response and are consumed by a subsequent request. It annotates these in the generated plan as parameterised values. The output is a draft test plan that the user reviews, edits, and saves. The user can re-run the crawler on an updated version of the application and OpenSynapse will diff the two graphs to show what changed.

API crawling: if the target exposes an OpenAPI or Swagger document, the crawler ingests it directly and generates requests for every operation, using example values from the schema. If no spec is available, the crawler intercepts network traffic during the UI crawl and reconstructs API call sequences from that.

### 5.5 Live test control

While a test is running, the user can modify active VUs, requests per second, and duration without stopping the test. The underlying engine is k6 configured with the externally-controlled executor, which exposes a REST API for runtime VU changes. OpenSynapse's control plane wraps this API and adds runtime RPS adjustment by maintaining a throttling layer between VUs and the target. Duration changes are handled by extending or truncating the configured run window; extending past the original maxVUs triggers a warning because it requires pre-allocation.

The live control UI shows three sliders (VUs, RPS, remaining duration), each with current and target values, and an "Apply" button. Changes are logged to the run's event stream so they appear on the report timeline alongside metrics, making it clear which metric shifts are caused by user intervention.

### 5.6 Real-time dashboards

During a run, the user sees live charts for requests per second, response time percentiles (p50, p90, p95, p99), error rate, active VUs, data sent and received, and a rolling error log. Charts update at one-second intervals. The user can add custom charts for any metric the script emits, including tagged subsets (for example, only the login endpoint). Charts are built with Recharts for consistency and low bundle impact.

### 5.7 Post-run reports and history

When a run finishes, the full result set is persisted to the local database. The run view shows all the live charts now populated with the full dataset, plus summary statistics, a per-endpoint breakdown, an assertion results table, and the event timeline including any live-control changes. Reports can be exported as PDF, HTML (standalone, includes embedded data for offline viewing), CSV (raw metrics), and JSON (the complete result set for programmatic use).

Runs are listed in a history view with filters for date, test plan, tags, and outcome. The user selects two or more runs and clicks "Compare" to open an overlay view. The overlay shows each selected metric with one line per run, colour-coded, with a difference summary at the top (for example: p95 improved by 180ms between run 3 and run 5). The comparison view supports the same export options as individual runs.

### 5.8 AI analysis (opt-in, BYO key)

In settings, the user configures an AI provider (OpenAI, Anthropic, Azure OpenAI) and pastes an API key. OpenSynapse stores the key in the OS keychain on desktop installs and in a Kubernetes secret on cluster installs. When AI is enabled, a "Analyse" button appears on run and comparison views. Clicking it sends a structured summary of the run (metrics, thresholds, any anomalies) to the configured provider and displays the response as annotated insights: likely bottlenecks, suggested next tests, interpretation of percentile shifts between runs. The raw prompt and response are shown in a collapsible panel so users can audit what was sent.

No metrics are sent to any AI provider without explicit user action. AI is never the default path for any insight.

### 5.9 Controllers and logic

The plan builder includes if controllers (evaluate a JavaScript expression against the previous response or shared state), else branches, loop controllers, transaction controllers (group requests and report aggregated metrics), once-only controllers (for setup steps that run per VU), and random controllers (pick one child at random). Assertions verify status codes, response body content (substring, regex, JSONPath, XPath), headers, and response time thresholds. Assertion failures are recorded against the run and count towards the error rate.

### 5.10 Reusable components

A library of pre-built plan fragments ships with the product: generic login (form-based, OAuth, SAML stub), CSRF token extraction, pagination walker, search-then-select, cart checkout, file upload, file download with hash verification. Users can save their own fragments to a personal library and share them via export or a team shared folder if deployed on a server.

## 6. Non-functional requirements

Performance. A single local instance on a 2023-era laptop must sustain 2,000 virtual users or 5,000 HTTP requests per second for API tests, whichever limit is reached first. Distributed mode on Kubernetes must scale linearly to at least 100,000 VUs across 10 worker pods, bounded by cluster capacity.

Measurement accuracy. Response time is measured from the moment the first byte of the request is written to the socket until the last byte of the response is read. This matches k6's native measurement and must be documented clearly so users comparing OpenSynapse runs to browser timings understand what is included and excluded.

Startup time. Cold start to usable UI on desktop must be under 5 seconds. Test plan load for a plan with 500 nodes must be under 1 second.

Reliability. The control plane must survive a worker crash without losing in-progress metrics already reported. Runs must be resumable if the control plane itself crashes, up to the point of the last checkpoint (checkpoints every 30 seconds).

Security. All installer artefacts are signed. The REST API requires a token generated at install time and stored locally. Multi-user mode uses per-user tokens with role-based permissions. No telemetry leaves the machine without explicit opt-in.

Accessibility. WCAG 2.1 AA compliance for the UI. All charts have accessible table equivalents. Keyboard navigation for all workflows.

## 7. Data model (high level)

The four top-level entities are Test Plan, Run, Result, and Environment. A Test Plan is a JSON document representing the node tree; its schema is defined in 03-architecture.md. A Run is an execution of a Test Plan with specific parameters (VUs, duration, environment bindings). A Result is the time-series and summary data produced by a Run. An Environment is a named set of variables (base URL, credentials, feature flags) that plans reference.

Secondary entities include Fragments (reusable plan subtrees), Reports (saved comparison configurations), Keys (AI provider credentials), and Users (only in team mode).

Full schema definitions live in 04-data-model.md.

## 8. Success metrics

The product succeeds if a new user can go from installer to a running load test in under 10 minutes without reading documentation. A user migrating from JMeter should be able to rebuild a typical test plan in OpenSynapse in half the time it took them in JMeter. Comparison reports should reduce the time to explain a performance regression to developers by an order of magnitude compared to exporting JMeter JTL files.

These are measured via built-in anonymous usage timing (opt-in, never transmitted), not via remote telemetry.

## 9. Assumptions

The user has Docker available if they want to run the Docker deployment option. They have kubectl and cluster access if they want the Kubernetes option. Desktop installs require no prerequisites beyond what the installer bundles. The target applications being tested use HTTP, HTTPS, or WebSocket; other protocols are v1.1.

## 10. Open questions for the build

These are flagged for the implementer to resolve during Phase 1 and do not block starting work.

Which lightweight embedded database to use for run storage. Candidates: SQLite with a time-series extension, DuckDB, or a pair (SQLite for metadata, Parquet files for raw metric samples). Recommendation in 03-architecture.md: SQLite plus Parquet.

Whether to ship a built-in reverse proxy for recording browser traffic as an alternative to the browser-based crawler. Nice-to-have for v1.1, not v1.

How to handle long-running soak tests (8+ hours) in desktop mode when the machine sleeps. Recommendation: refuse to start if power settings allow sleep, with a one-click "prevent sleep" helper.
