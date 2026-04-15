# OpenSynapse — Claude Code Implementation Plan

**Version:** 1.0
**Audience:** Claude Code
**Related:** All other documents in this bundle

This plan breaks the project into phases that Claude Code can execute in order. Each phase is independently testable and produces something runnable. Do not proceed to the next phase until the current one is working. When you hit a decision point not covered by the specs, make the choice that keeps options open and document it in `docs/decisions/`.

---

## How to use this document

Claude Code should read the documents in this order before starting: 01-PRD.md for the vision, 02-architecture.md for the shape, 05-ui-ux-spec.md for the design language, 03-feature-spec.md for the behaviours, 04-data-model-and-api.md for the contracts, and this plan for sequencing.

Each phase has an objective, a list of deliverables, a short acceptance test, and a "stop if" condition that says what should halt progress.

Do not try to implement the entire system in one pass. Work phase by phase. At the end of each phase, run the acceptance test and commit.

---

## Phase 0 — Project scaffold

**Objective.** Stand up the repository structure, tooling, CI, and a hello-world on all three deployment targets.

**Deliverables.**

A monorepo using pnpm workspaces at the root. Top-level packages:

```
/
├── apps/
│   ├── control-plane/      Go service
│   ├── web/                React SPA
│   └── desktop/            Tauri shell
├── packages/
│   ├── ui/                 shared React components and tokens
│   ├── plan-schema/        JSON schemas for node types
│   └── api-client/         generated TypeScript client for the REST API
├── deploy/
│   ├── docker/             Dockerfiles and compose
│   └── helm/               Helm chart for Kubernetes
├── docs/                   architecture and decision records
├── scripts/                build and release tooling
└── .github/workflows/      CI
```

Go module for the control plane with a minimal HTTP server that returns `{"status":"ok"}` on `/health`. React app bootstrapped with Vite that renders a single page showing "OpenSynapse" and the Tailwind setup. Tauri shell that opens a window and loads the React app, with the Go control plane as a sidecar. Dockerfile for the control plane. A compose file with control-plane plus nginx-serving-the-web-build. A skeleton Helm chart that deploys control-plane as a Deployment.

GitHub Actions workflow that runs lint, tests, and builds for all packages on every push.

**Acceptance.** `pnpm dev` starts the control plane and web app together. `cargo tauri dev` starts the desktop app and it opens a window showing the hello-world. `docker compose up` in `deploy/docker/` starts the services. `helm template ./deploy/helm` produces valid Kubernetes manifests.

**Stop if.** The build does not reproduce on a clean checkout, or any of the three deployment modes fails to start the hello-world.

---

## Phase 1 — Control plane core and data model

**Objective.** Implement the core REST API with the data model, plan storage, and environment management. No test execution yet.

**Deliverables.**

A Go control plane with routes for plans and environments matching section 2.1 and 2.2 of 04-data-model-and-api.md. SQLite backend for desktop (embedded, no daemon). The schema is expressed as migrations in `control-plane/internal/db/migrations/`. Plans are stored as JSONB (or TEXT for SQLite) with a version counter.

JSON schema files for every node type under `packages/plan-schema/schemas/`. The control plane validates plans against these schemas on save. A small test plan fixture that uses every node type once.

A TypeScript client generated from an OpenAPI spec that the Go server emits at `/openapi.json`. The web app imports this client.

Unit tests for every handler. Integration tests that exercise the happy path of CRUD on plans and environments.

**Acceptance.** The web app can create, list, open, edit, and delete plans and environments via the generated client. All data persists across restarts. Validation errors produce the structured error response from section 4 of the data model doc.

**Stop if.** Plan validation passes invalid plans or fails on the fixture, or if the generated TypeScript client drifts from the Go handlers.

---

## Phase 2 — Visual plan builder

**Objective.** Ship the three-pane plan builder with the full set of node types.

**Deliverables.**

The plan builder page at `/plans/{id}` in the web app. Left pane is a node tree built on a library-neutral tree primitive (not React Flow yet; React Flow is for the canvas). Centre pane is a React Flow canvas showing the currently selected branch. Right pane is a form driven by the node's JSON schema (use `react-jsonschema-form` or `@rjsf/core`).

Drag-and-drop for adding and moving nodes. Keyboard shortcuts for copy, paste, cut, duplicate, delete, add child. Undo and redo via a command stack in Zustand.

Debounced auto-save. Version history view.

Show-script toggle that calls `POST /plans/{id}/compile` on the control plane. The control plane has a minimal plan-to-script transformer that handles HTTP samplers, groups, ifs, loops, and scenarios. Advanced node types can emit TODO comments in the generated script for now.

**Acceptance.** A user can build a 20-node plan using the visual builder without touching code, save it, reload the page, and see the same plan. The show-script view compiles a plan with scenarios and controllers into a valid k6 script (validated by running `k6 inspect` on it in CI).

**Stop if.** The builder lags at 100 nodes, or the generated script is rejected by `k6 inspect`.

---

## Phase 3 — Local execution engine

**Objective.** Run a plan locally using a bundled k6 binary and stream metrics to the control plane.

**Deliverables.**

A bundled k6 binary built with xk6 including the xk6-kv extension. The control plane starts this binary as a subprocess when a run is requested. The run lifecycle endpoints (POST /runs, GET /runs/{id}, GET /runs/{id}/samples, etc.) are implemented as in section 2.3 of the data model doc.

The control plane ingests metrics via k6's experimental Prometheus remote-write output pointed at a local endpoint on the control plane. The ingested samples are parsed, aggregated, and stored. Samples go to Parquet files under a runs directory; metadata goes to SQLite.

The WebSocket channel `runs.{id}.metrics` broadcasts 1 Hz snapshots to subscribed clients.

A minimal run view in the web app that shows live RPS, p95 response time, error rate, and active VUs as line charts (Recharts). Charts update from the WebSocket stream.

**Acceptance.** A user creates a plan in phase 2's builder, clicks Run, sees the live charts populate, and after the run completes, the run is listed in the runs table with correct summary statistics.

**Stop if.** Metric ingestion drops samples under load (test at 10,000 samples per second), or the live chart lag exceeds 2 seconds.

---

## Phase 4 — Live control

**Objective.** Implement runtime VU, RPS, and duration changes using the techniques described in section 3 of 02-architecture.md.

**Deliverables.**

The plan-to-script transformer is updated to always wrap the user's scenario in an externally-controlled executor. The generated script includes a token-bucket RPS throttle that reads `rps_target` from xk6-kv each iteration, and a watchdog that reads `should_stop` and a `deadline_ms` from xk6-kv and aborts when either is triggered.

The `PATCH /runs/{id}/control` endpoint fans out VU changes to the k6 REST API of each worker and writes RPS and duration changes to the shared kv store.

The live control panel UI: three sliders for VUs, RPS, and duration, an Apply button, pause and resume buttons, stop and kill buttons. Control events are logged to the run event stream and visible as markers on the live charts.

**Acceptance.** During a running test, moving the VU slider from 10 to 50 takes effect within 2 seconds and is visible in the active VU chart. Moving the RPS slider changes the measured rate within the next tick. Extending the duration by 5 minutes pushes the end time back without restarting. Pausing stops new iterations; resume continues.

**Stop if.** VU changes exceed 3 seconds of latency, or RPS changes produce visible oscillation (that is, the throttle is unstable).

---

## Phase 5 — Reports, history, and comparison

**Objective.** Run history, the full post-run report view, and multi-run comparison.

**Deliverables.**

The runs list view with filters and sort. The run report view that shows all six default charts (section F-06), the summary cards, the per-endpoint breakdown, the assertion results table, and the event timeline.

Export endpoints for PDF, HTML, CSV, and JSON. PDF is generated by a headless Chromium render of a print stylesheet. HTML is self-contained.

The comparison view takes 2 to 5 runs and overlays their metrics. The summary block highlights improvements and degradations. Statistical significance check (simple two-sample test) avoids flagging noise.

**Acceptance.** A user can select three runs from history, click Compare, and see an overlay chart for p95 response time with clearly labelled lines. Exporting to HTML produces a file that opens in a browser offline and renders the same charts.

**Stop if.** PDF generation takes longer than 30 seconds for a typical run, or the HTML export requires an internet connection to render.

---

## Phase 6 — Endpoint playground

**Objective.** Ship a Postman-style request builder and collections.

**Deliverables.**

A new route `/playground` in the web app. The request builder and response viewer are described in section F-03 of the feature spec. The server-side for playground requests is `POST /playground/request`, which the control plane executes using Go's net/http (not k6, because this is single-shot, not a load test). All seven auth methods work.

A micro-test action that runs the current request N times (capped at 50 VUs, 1 minute) via the same load engine as normal runs, showing a mini dashboard.

Save-to-plan inserts an HTTP sampler into a user-selected plan.

**Acceptance.** The user can set up a request with bearer auth against a test API, send it, see the timing breakdown, and save it to a plan.

**Stop if.** OAuth flow does not complete for at least one real provider (pick one with a sandbox, for example Auth0's test tenant).

---

## Phase 7 — Crawler

**Objective.** Ship the multi-engine crawler and the plan generator.

**Deliverables.**

A crawler package inside the control plane implementing the `CrawlEngine` interface with three engines: Rod (headless Chromium), Colly (HTTP), and OWASP ZAP (sidecar). See ADR-0003 for engine selection rationale. The crawl configuration, graph output, and correlation logic are described in section F-04.

The crawl result is stored in the database. The plan generator converts it into a draft plan using the heuristics described.

The UI: a new route `/crawler` with a configuration form, progress view, and graph visualisation. On completion, the user clicks "Generate plan" and is redirected to the plan builder with the draft loaded.

OpenAPI ingestion path: if the target exposes `/openapi.json`, `/swagger.json`, or `/v3/api-docs`, use it directly instead of crawling the UI.

**Acceptance.** Crawling a demo SPA (ship one with the project: a small React app with login, list, and detail pages) produces a plan that runs without manual edits. Crawling an OpenAPI-backed API produces a plan with one sampler per operation.

**Stop if.** The crawler cannot handle a form login, or the correlation step misses obvious CSRF tokens on the demo app.

---

## Phase 8 — AI analysis

**Objective.** Add opt-in AI analysis with BYO provider keys.

**Deliverables.**

Settings UI for AI configuration. Storage of keys in the OS keychain (desktop) or Kubernetes secret (cluster). Provider abstraction in Go with three implementations: OpenAI, Anthropic, Azure OpenAI. Test-key endpoint.

The `/ai/analyse` endpoint that takes a run ID or report ID and a question, builds a structured prompt from run summary data (not raw samples), sends it to the configured provider, and returns the response. Prompts are cached per run per question.

A UI button on the run view and comparison view. A prompt-preview modal shown before sending. The response renders as markdown in a panel.

Cost tracking per call, monthly cap in settings.

**Acceptance.** With an OpenAI key configured, clicking Analyse on a run produces a sensible paragraph about the run within 10 seconds. No data is sent without explicit user click.

**Stop if.** Any test sends data to a provider without user action, or the key is visible anywhere in logs or diagnostic bundles.

---

## Phase 9 — Fragments and template gallery

**Objective.** Ship the template gallery and reusable fragments library.

**Deliverables.**

The template gallery as described in section F-01, with the nine templates and their SVG animations. Animations are pre-built SVG files in `packages/ui/src/assets/templates/`.

The fragments library (F-10) with the ten shipped fragments. User fragment save, edit, delete. Variable binding dialog on insert.

**Acceptance.** A new user clicks "New test", picks "Load test" from the gallery, and lands on a builder with a ready-to-run plan. A user selects a subtree and saves it as a fragment, then inserts it into another plan.

**Stop if.** Any shipped fragment fails to run against its intended target.

---

## Phase 10 — Cluster mode and distributed execution

**Objective.** Make everything work on Kubernetes with the k6-operator.

**Deliverables.**

A complete Helm chart for OpenSynapse including the control plane, Postgres (or a BYO-Postgres option), nginx ingress, and a dependency on the k6-operator chart. MinIO for object storage of Parquet files in a cluster.

The control plane detects cluster mode at startup (env var) and switches from subprocess k6 to creating TestRun custom resources. Metrics still stream back via Prometheus remote-write; the control plane hosts the endpoint and workers are configured to push to it.

Live control works across distributed workers by fanning out to each worker pod's k6 REST API. VU changes are split using execution segments.

Team mode authentication: user accounts, argon2id passwords, per-user API tokens. Role-based permissions (admin, editor, viewer).

**Acceptance.** A fresh `helm install opensynapse ./deploy/helm` on a local kind cluster produces a working OpenSynapse accessible via port-forward. A run with parallelism 4 produces correct aggregated metrics that match a single-worker run with 4x the VUs.

**Stop if.** Metric aggregation across workers is inconsistent, or live control does not reach all workers within 5 seconds.

---

## Phase 11 — JMeter import, Docker polish, and release engineering

**Objective.** Ship the final v1 pieces and make releases reproducible.

**Deliverables.**

JMeter .jmx importer as described in F-12. Mapping table in a dedicated file. Test fixtures covering real-world JMX files.

Docker image polish: multi-arch builds, minimal base image, signed with cosign. Docker compose with sensible defaults and a quick-start script.

Desktop installer signing for all three OSes. Auto-update via Tauri updater (opt-in).

Helm chart published to a chart repository. Release workflow that tags, builds all artefacts, and publishes.

Comprehensive README. Getting-started guide. A demo target application shipped in the repo for tutorials.

**Acceptance.** A user on a fresh Mac can download the signed installer, double-click, and be in a running app in under 2 minutes. The same for Windows and Linux. `docker compose up` in the repo works out of the box. `helm install` on kind works out of the box.

**Stop if.** Any installer is rejected by its OS (Gatekeeper, SmartScreen, AppImage sandbox), or the Helm chart fails on a vanilla kind cluster.

---

## Cross-cutting concerns

These apply to every phase.

Testing. Every new endpoint has a unit test and an integration test. Every new UI component has a Storybook entry and a rendering test. Integration tests use a seeded test database.

Error handling. Every user-facing error has a clear message, a code, and a suggested action. No stack traces surface in the UI.

Logging. Structured logs (JSON) from the control plane. The UI has a developer log drawer accessible from settings.

Metrics on OpenSynapse itself. The control plane exposes its own Prometheus `/metrics` with request rates, error rates, queue depths, and worker health.

Documentation. Every phase's deliverables include updating the user-facing docs in `docs/user/`. Architecture decisions are recorded as ADRs under `docs/decisions/`.

Decision records. When the implementer chooses between options that the specs leave open, record the decision, the alternatives considered, and the reason chosen. One file per decision, numbered.

---

## What "done" looks like for v1

A user downloads the installer for their OS. Double-clicks. The app opens. They click "Crawl application", point it at their staging environment, log in, and wait. The crawler produces a plan. They pick the "Stress" template to reshape the load, adjust the target VUs, and click Run. Live charts populate. Mid-run, they drag the VU slider from 100 to 200 and watch the response time react. The run completes. They click "Analyse with AI" (if configured) and get a plain-language explanation of the bottleneck. They share the HTML export with a developer. The developer ships a fix. The user re-runs the same plan, compares the new run to the previous one, and sees the improvement.

That full flow should be achievable end-to-end by the time phase 11 is complete.
