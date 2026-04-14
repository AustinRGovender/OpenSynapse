# OpenSynapse — Technical Architecture

**Version:** 1.0
**Companion to:** 01-PRD.md

---

## 1. High-level shape

OpenSynapse is a three-tier system. The UI tier is a React single-page application. The control plane is a Go service that exposes a REST and WebSocket API, manages test plans and runs, persists results, and orchestrates workers. The worker tier is a pool of k6 processes, run locally as subprocesses on desktop installs and as Kubernetes pods via the k6-operator on cluster installs. The same control plane code drives all three deployment modes; only the worker launcher differs.

Go is chosen for the control plane because k6 is already written in Go and its execution primitives, metric types, and script runtime can be embedded or invoked natively. This eliminates translation layers and version drift between the control plane and the load engine.

## 2. Why k6 as the engine

k6 was selected after evaluating JMeter, Gatling, Locust, and a custom engine. The deciding factors are runtime VU control via the externally-controlled executor (which exposes a REST API to change VUs and pause/resume during a live test, the single most important technical requirement from the PRD), goroutine-per-VU concurrency which outperforms thread-based tools like JMeter on modest hardware, JavaScript scripting which lowers the barrier for users compared to Scala (Gatling) or XML (JMeter), Kubernetes-native distributed execution via the official k6-operator, and a healthy xk6 extension system for adding custom protocols without forking the engine.

The trade-offs accepted: k6 reports response time slightly differently from JMeter and Gatling (typically 10 to 20 percent lower, depending on protocol specifics) because of how it accounts for connection reuse. This is documented for users in the run report with a note explaining how timings are measured. k6's externally-controlled executor does not work with k6 Cloud, which is irrelevant for us because we are building our own orchestration.

## 3. Live control implementation

This is the architecturally tricky part so it gets its own section.

k6 accepts one of several executors per scenario. To enable live control, OpenSynapse generates test scripts that use the `externally-controlled` executor. This executor starts k6 with a configured `vus` and `maxVUs` and a default `duration`. k6 then starts its local REST API on port 6565 (the port is randomised per run in OpenSynapse to avoid collisions). The control plane sends PATCH requests to `/v1/status` on that port to change the active VU count and to pause or resume.

For RPS control, k6's built-in `--rps` flag is a per-instance cap that cannot be changed at runtime. OpenSynapse instead implements RPS throttling inside the generated test script using a shared token bucket backed by an xk6-kv store. The control plane updates a `rps_target` key in the store; the VU script reads it at the start of each iteration and sleeps to maintain the target. Changing the key from the control plane effectively changes the test rate without restarting.

For duration changes, shortening is implemented by calling the k6 REST API to pause and then graceful-stop. Lengthening requires a different approach because k6 cannot extend a scenario past its configured duration. OpenSynapse solves this by starting every run with a generous maxDuration (the user-requested duration plus a 50 percent buffer, capped) and tracking the "effective end time" in the control plane. A small watchdog goroutine inside the generated script reads a `should_stop` flag from the kv store each iteration and calls `exec.test.abort()` when it flips. The user's "extend" control simply pushes the effective end time later; the "shorten" control flips the stop flag.

All three controls (VUs, RPS, duration) appear to the user as live sliders. The underlying mechanics are hidden.

## 4. Deployment modes

### 4.1 Desktop (Mac, Windows, Linux)

The desktop build is a single binary produced with Wails or Tauri. The choice is Tauri for smaller binary size (20 to 30 MB vs 80+ MB for Electron) and because it lets us embed the Go control plane as a sidecar binary rather than recompiling it into a JavaScript runtime. The front-end is the same React SPA served from the embedded binary. k6 is bundled as a sidecar executable, patched with the required xk6 extensions (xk6-kv for the token bucket, xk6-browser for the crawler if not using a separate Playwright install). The installer is a signed .dmg for Mac, .msi for Windows, and .deb plus .rpm plus .AppImage for Linux.

First-run experience: the app launches, the control plane starts on localhost, a random port is chosen, the UI opens in a window (Tauri's webview, not a browser). The user sees a welcome screen with three paths: "new test", "import JMeter .jmx", or "start a crawl". No account creation, no configuration.

### 4.2 Docker

A single `docker-compose.yml` brings up three containers: `opensynapse-control`, `opensynapse-ui` (nginx serving static assets), and `opensynapse-worker` (a k6 image with xk6 extensions baked in). A fourth optional container is `opensynapse-db` running SQLite on a persistent volume. The user runs `docker compose up -d` and the UI is available at `http://localhost:8080`. Distributed workers are added by scaling the worker service.

### 4.3 Kubernetes

A Helm chart deploys the control plane, a Postgres StatefulSet (for cluster mode; SQLite is desktop-only), an nginx ingress, and the k6-operator. When a test runs, the control plane creates a `TestRun` custom resource that the k6-operator fulfills by spawning worker pods with the configured parallelism. Metrics stream back to the control plane via the k6 remote-write Prometheus endpoint, which the control plane terminates and re-emits over its WebSocket API to the UI.

A single command `helm install opensynapse ./charts/opensynapse` sets everything up. The chart includes sensible defaults and a `values.yaml` for customisation. Target Kubernetes versions: 1.28 and up.

## 5. Distributed load generation

On desktop, parallelism is a single k6 process with many goroutines. On Docker compose, parallelism is multiple worker containers, each running its own k6 process. On Kubernetes, parallelism is multiple worker pods managed by the k6-operator.

In all cases, the control plane splits the workload using k6's execution segment feature: each worker is told to run a fraction of the total VUs and iterations (for example, worker 1 runs segment 0/4 to 1/4, worker 2 runs 1/4 to 2/4, etc.). This is the native, well-tested k6 sharding mechanism and we use it unmodified.

Worker discovery in cluster mode uses Kubernetes service discovery. Worker discovery in Docker Compose uses the compose network's DNS. Worker discovery in desktop mode is trivial because there is only one worker.

Recommendation for v1 scaling targets. Desktop: one worker, up to 2,000 VUs. Docker Compose: up to 5 workers on a single host, 10,000 VUs. Kubernetes: up to 100 workers, 100,000+ VUs. Beyond that is Phase 2.

## 6. Data flow

### 6.1 Starting a run

The user clicks Run on a test plan. The UI sends `POST /api/runs` with the plan ID and parameters. The control plane validates the plan, compiles it to a k6 JavaScript file using the plan-to-script transformer (see section 9), resolves the environment, writes the script to a temp location, and launches workers. Each worker starts k6 with the externally-controlled executor pointing at the generated script. k6 starts its REST API on a random local port; the control plane records these endpoints.

### 6.2 Metric ingestion

Each k6 worker is configured with `--out=experimental-prometheus-rw` pointed at the control plane's ingestion endpoint. The control plane accepts the remote-write stream, parses the Prometheus payload, tags each sample with the run ID and worker ID, and buffers samples in a ring buffer sized for one minute of data at peak ingestion rate. Every one second, the control plane aggregates the ring buffer (sum counters, merge histograms, compute percentiles) and broadcasts a snapshot over WebSocket to any connected UI clients subscribed to that run. Every 30 seconds, the full buffer is flushed to the persistent store (SQLite plus Parquet on desktop, Postgres plus S3-compatible object store on cluster).

### 6.3 Live control path

The user drags a VU slider and releases. The UI sends `PATCH /api/runs/{id}/control` with the new target. The control plane fans out to all k6 worker REST APIs, updating VUs proportionally across execution segments. For RPS and duration it writes to the xk6-kv store that workers are polling. The control plane logs a control event to the run timeline. The UI confirms on the next tick.

### 6.4 Run completion

When all workers exit, the control plane finalises the result set, computes run-level summary statistics, persists the final artefacts, and emits a `run.completed` WebSocket event. The UI transitions from live dashboard to historical report view, which reads from the persistent store rather than the ring buffer.

## 7. Persistence

### 7.1 Desktop mode

SQLite holds plans, runs, run metadata, environments, fragments, user preferences, and AI keys (keys go through the OS keychain first, with only a reference in SQLite). Raw metric samples for completed runs are written to Parquet files in a `runs/{run_id}/` directory. Parquet is chosen because it compresses time-series data well (typical reduction 8 to 10 times versus raw JSON), is queryable from both DuckDB and Go, and survives being copied to a USB stick for sharing.

### 7.2 Cluster mode

Postgres replaces SQLite. Parquet files are written to an S3-compatible object store (MinIO ships with the Helm chart for out-of-box use). The schemas are otherwise identical.

### 7.3 Schema overview

Plans, runs, results, environments, fragments, reports, users, and audit entries. Full DDL is in 04-data-model.md.

## 8. The front-end

A single React 18 SPA built with Vite, written in TypeScript. State management is Zustand for local UI state and TanStack Query for server state and cache invalidation. Routing is TanStack Router for type-safe routes. The component library is shadcn/ui on top of Tailwind, which gives us accessible primitives we fully own rather than a locked-in library. Charts are Recharts. The visual plan builder uses React Flow for the node tree because it handles large graphs well and supports keyboard navigation out of the box.

Design tokens and theming are defined in 06-ui-ux.md. The short version: neutral slate palette, one accent colour (a muted teal) for interactive elements, one semantic colour family (red for errors, amber for warnings, green for success, blue for informational), no other colour elsewhere in the product. Typography is Inter for UI and JetBrains Mono for code. The product aims for the visual language of Linear, Vercel, or modern Grafana rather than the heavy saturation of JMeter or older enterprise tools.

## 9. Plan-to-script transformer

The plan builder produces a JSON document. The transformer walks this document and emits a k6 JavaScript file. Each node type has a corresponding emit function. Controllers become `if` blocks or `for` loops. Transaction controllers become `group()` calls. Assertions become `check()` calls. Data sources become `SharedArray` imports. The xk6-kv token bucket helper and the should-stop watchdog are prepended to every generated script.

The transformer is stateless and deterministic: the same plan always produces the same script bytes, which makes debugging and diffing easier. It is also reversible for a documented subset (a round-trip from plan to script to plan preserves plan semantics), which allows users to hand-edit a generated script and re-import it into the builder. Non-round-trippable constructs (arbitrary JavaScript expressions the user writes in a code block node) are preserved verbatim and marked as opaque in the builder.

## 10. Crawler subsystem

The crawler runs as a separate Go service that spawns Playwright (via playwright-go). It consumes a crawl job spec (entry URL, auth config, depth, blocklist) and produces a crawl result document (graph of pages, list of network requests with request/response pairs, correlation hints). The result is fed to a "plan generator" which turns the graph into a draft test plan using heuristics: group requests by endpoint prefix, detect CSRF tokens by pattern, parameterise query strings that contain values seen elsewhere in the graph.

Playwright is the right choice here over Puppeteer because it has better network interception APIs and first-class support across Chromium, Firefox, and WebKit, which matters for users whose targets are WebKit-specific.

The crawler is bandwidth-bounded by the target and network-bounded by Playwright's single-page instrumentation, so it does not need distributed execution in v1.

## 11. AI integration

A small service layer in the control plane exposes `POST /api/ai/analyse` with a run ID or a comparison ID. It builds a structured prompt with run metadata, summary statistics, threshold results, and a handful of concrete anomalies (for example, percentile jumps above a threshold). It sends the prompt to the configured provider using the provider's official SDK where possible, or HTTP otherwise. Responses are cached per run to avoid double-billing if the user clicks Analyse twice.

Three providers are supported in v1: OpenAI (chat completions), Anthropic (messages), Azure OpenAI (chat completions with deployment routing). The provider abstraction is a Go interface with one implementation per provider. Adding a fourth is a file, not a refactor.

## 12. Packaging and distribution

Each desktop build is a GitHub Actions matrix job that produces signed artefacts. The Docker build is a multi-arch image (amd64 and arm64) published to GHCR. The Helm chart is published to a Helm repository served from GitHub Pages.

Release cadence is monthly for minor versions and immediately for security fixes. Auto-update is opt-in on desktop using the Tauri updater, disabled by default in enterprise settings for change-control reasons.

## 13. Observability of OpenSynapse itself

OpenSynapse is a load tester; ironically it should be boringly reliable. The control plane emits its own Prometheus metrics on `/metrics` (request rates, queue depths, worker health), exposes health and readiness probes for Kubernetes, and writes structured logs to stdout. Desktop installs surface logs in a "Developer tools" drawer accessible from settings.

## 14. Technology stack summary

| Layer | Choice | Reason |
|-|-|-|
| Load engine | k6 (Grafana) | Externally-controlled executor, Go-native, extension system |
| Control plane | Go 1.22+ | Same language as k6, fast, easy to ship as a single binary |
| UI framework | React 18 + TypeScript + Vite | Requested by product owner; mature ecosystem |
| UI routing | TanStack Router | Type-safe routes |
| UI state | Zustand + TanStack Query | Minimal boilerplate, correct cache semantics |
| UI components | shadcn/ui + Tailwind | Owned code, accessible, no lock-in |
| UI charts | Recharts | Good-enough performance, simple API |
| Plan graph | React Flow | Best-in-class node editor |
| Desktop shell | Tauri 2 | Small binary, Go sidecar support |
| Crawler | Playwright (playwright-go) | Best network interception |
| Cluster orchestration | k6-operator | Official, proven, TestRun CRD |
| Desktop DB | SQLite + Parquet | Embedded, queryable, portable |
| Cluster DB | Postgres + S3-compatible | Standard cloud-native stack |
| Packaging | Tauri bundler, Docker, Helm | One per deployment target |
| CI | GitHub Actions | Matrix builds for all OSes |
