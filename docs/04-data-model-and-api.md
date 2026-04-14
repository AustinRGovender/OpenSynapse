# OpenSynapse — Data Model and API

**Version:** 1.0
**Companion to:** 02-architecture.md

---

## 1. Entities

### 1.1 Plan

A test plan document. Stored as JSONB in Postgres, as a JSON blob in SQLite.

```
Plan {
  id: UUID
  name: string
  description: string
  tags: string[]
  created_at: timestamp
  updated_at: timestamp
  version: integer            // incremented on every save
  default_environment_id: UUID?
  root: Node                  // the node tree
}

Node {
  id: UUID                    // stable across saves
  type: string                // "plan" | "scenario" | "http" | "if" | ...
  name: string
  enabled: boolean
  properties: object          // type-specific; validated by schema per type
  children: Node[]
}
```

Each node type has a JSON schema file at `schemas/nodes/{type}.schema.json` that the control plane uses for validation. Adding a new node type is adding a schema file and a transformer function; no core changes required.

### 1.2 Environment

```
Environment {
  id: UUID
  name: string
  variables: Map<string, Variable>
  created_at: timestamp
  updated_at: timestamp
}

Variable {
  value: string
  secret: boolean             // if true, encrypted at rest and redacted in logs
}
```

### 1.3 Run

```
Run {
  id: UUID
  plan_id: UUID
  plan_version: integer       // snapshot of plan.version at start
  plan_snapshot: Plan         // full plan copy for historical fidelity
  environment_snapshot: Environment?
  parameters: {
    vus_target: integer
    rps_target: integer?
    duration_seconds: integer
    worker_count: integer
  }
  status: "queued" | "running" | "completed" | "failed" | "aborted"
  started_at: timestamp
  ended_at: timestamp?
  summary: RunSummary?        // populated on completion
  created_by: UUID?           // user ID in team mode
}

RunSummary {
  total_requests: integer
  failed_requests: integer
  error_rate: float
  throughput_rps: float
  p50_ms: float
  p90_ms: float
  p95_ms: float
  p99_ms: float
  max_ms: float
  bytes_sent: integer
  bytes_received: integer
  assertion_failures: integer
  thresholds_passed: boolean
}
```

### 1.4 Result samples

Time-series samples. Written to Parquet on desktop, Postgres (hypertable via timescaledb) or S3 on cluster.

```
Sample {
  run_id: UUID
  timestamp_ms: integer       // epoch ms
  metric: string              // "http_req_duration", "http_reqs", etc
  value: float
  tags: Map<string, string>   // endpoint, status, scenario, worker_id
}
```

Retention: configurable. Default 90 days for samples, indefinite for summaries.

### 1.5 Event

Events track lifecycle and control changes for a run. They appear on the timeline.

```
Event {
  run_id: UUID
  timestamp_ms: integer
  type: "start" | "stop" | "vu_change" | "rps_change" | "duration_change" | "error" | "threshold_breach" | "user_note"
  payload: object
}
```

### 1.6 Fragment

```
Fragment {
  id: UUID
  name: string
  description: string
  tags: string[]
  node_subtree: Node          // the root of the fragment
  bindings: Binding[]         // variables the user must supply on insert
  built_in: boolean           // true for shipped fragments
}

Binding {
  name: string
  description: string
  default_value: string?
  required: boolean
}
```

### 1.7 Report

A saved comparison configuration.

```
Report {
  id: UUID
  name: string
  run_ids: UUID[]
  metrics: string[]           // which metrics to overlay
  normalisation: "elapsed_time" | "run_progress"
  ai_analysis_cached: string?
  created_at: timestamp
}
```

### 1.8 AI configuration

```
AIConfig {
  provider: "openai" | "anthropic" | "azure_openai"
  api_key_ref: string         // reference into OS keychain / k8s secret
  model: string
  monthly_cap_usd: float?
  enabled: boolean
}
```

### 1.9 User (team mode only)

```
User {
  id: UUID
  email: string
  display_name: string
  role: "admin" | "editor" | "viewer"
  created_at: timestamp
  api_token_hash: string
}
```

Desktop mode has a single implicit local user and no User table.

---

## 2. REST API

Base path: `/api/v1`. All requests authenticate with a bearer token generated at install time (desktop) or per-user (team mode). Responses are JSON unless otherwise noted.

### 2.1 Plans

```
GET    /plans                       List plans
POST   /plans                       Create plan
GET    /plans/{id}                  Get plan
PUT    /plans/{id}                  Update plan (full replacement)
PATCH  /plans/{id}                  Update plan (JSON patch)
DELETE /plans/{id}                  Delete plan
GET    /plans/{id}/versions         List version history
GET    /plans/{id}/versions/{n}     Get a historical version
POST   /plans/{id}/validate         Validate without saving
POST   /plans/{id}/compile          Return the generated k6 script
POST   /plans/import/jmx            Import a JMeter .jmx file
```

### 2.2 Environments

```
GET    /environments
POST   /environments
GET    /environments/{id}
PUT    /environments/{id}
DELETE /environments/{id}
```

### 2.3 Runs

```
GET    /runs                        List runs (filters: plan_id, status, since, until, tag)
POST   /runs                        Start a run. Body: { plan_id, environment_id?, parameters? }
GET    /runs/{id}                   Get run metadata and summary
DELETE /runs/{id}                   Delete run and its samples
GET    /runs/{id}/samples           Query samples (filters: metric, tags, since, until, aggregate)
GET    /runs/{id}/events            List events for a run
POST   /runs/{id}/events            Add a user note
GET    /runs/{id}/report            Get the full report document
POST   /runs/{id}/export            Export report. Body: { format: "pdf" | "html" | "csv" | "json" }
PATCH  /runs/{id}/control           Live-control. Body: { vus?, rps?, duration_seconds?, paused? }
POST   /runs/{id}/stop              Graceful stop
POST   /runs/{id}/kill              Forceful stop
```

### 2.4 Comparison

```
POST   /compare                     Compute a comparison. Body: { run_ids: [...], metrics: [...] }
GET    /reports                     List saved reports
POST   /reports                     Save a comparison configuration
GET    /reports/{id}
DELETE /reports/{id}
POST   /reports/{id}/export         Export the comparison in one of the supported formats
```

### 2.5 Fragments

```
GET    /fragments                   List fragments (includes built-ins)
POST   /fragments                   Save a user fragment
GET    /fragments/{id}
PUT    /fragments/{id}
DELETE /fragments/{id}               // cannot delete built-ins
```

### 2.6 Crawler

```
POST   /crawls                      Start a crawl. Body: { entry_url, auth?, depth, same_origin, blocklist, limit }
GET    /crawls/{id}                 Get crawl status and progress
GET    /crawls/{id}/graph           Get the resulting graph
POST   /crawls/{id}/generate-plan   Produce a draft plan from the crawl
POST   /crawls/{id}/cancel          Cancel a running crawl
```

### 2.7 AI

```
GET    /ai/config                   Get AI configuration (secrets redacted)
PUT    /ai/config                   Update AI configuration
POST   /ai/config/test              Validate the current key with a minimal call
POST   /ai/analyse                  Analyse a run or comparison. Body: { run_id | report_id, question? }
```

### 2.8 Endpoint playground

```
POST   /playground/request          Execute a single request. Body is the full request spec.
GET    /playground/collections      List saved request collections
POST   /playground/collections      Save a collection
```

### 2.9 System

```
GET    /health                      Liveness
GET    /ready                       Readiness (DB, worker connectivity)
GET    /version                     Version info
GET    /metrics                     Prometheus metrics endpoint
GET    /settings                    Get settings
PUT    /settings                    Update settings
POST   /diagnostics                 Build a diagnostic bundle (returns a download URL)
```

---

## 3. WebSocket API

A single WebSocket endpoint at `/api/v1/ws`. The client connects, authenticates with the bearer token in the initial message, and subscribes to channels.

### 3.1 Message format

```
{
  "type": "subscribe" | "unsubscribe" | "event",
  "channel": string,
  "payload": object
}
```

### 3.2 Channels

`runs.{id}.metrics` — live metric snapshots (1 Hz). Payload contains aggregated values for all tracked metrics since the last tick.

`runs.{id}.events` — lifecycle and control events as they happen.

`runs.{id}.errors` — new errors as they are recorded.

`crawls.{id}.progress` — crawler progress updates (pages discovered, requests captured).

`system.notifications` — system-level events (import completed, export ready).

### 3.3 Backpressure

If a client cannot keep up with the 1 Hz metric rate, the server drops frames for that client without disconnecting. Dropped frames are counted and shown in a dev panel. The client can request a catch-up snapshot on demand.

---

## 4. Error handling

Standard error response:

```
{
  "error": {
    "code": "PLAN_NOT_FOUND",
    "message": "Plan with ID ... does not exist",
    "details": { ... }
  }
}
```

HTTP status codes follow conventional usage. Error codes are stable strings suitable for programmatic handling. A full error code catalogue lives in `docs/error-codes.md` (to be generated by the implementer).

---

## 5. Authentication

Desktop mode. At first launch, the installer generates a bearer token and writes it to the OS keychain. The UI reads it and includes it as `Authorization: Bearer ...` on every request. A "reveal token" button in settings lets power users copy it for CLI use.

Team mode. Each user has an email, password, and API token. Passwords are hashed with argon2id. Tokens are random 40-byte strings, hashed in storage. Tokens can be revoked individually.

No OAuth or SSO in v1. Deferred to v1.2.

---

## 6. Pagination

List endpoints use cursor-based pagination. Query parameters: `limit` (default 50, max 500), `cursor` (opaque string). Response includes `next_cursor` if more results exist.

---

## 7. Rate limiting

The control plane rate-limits the AI analysis endpoint to 20 requests per hour per user to avoid runaway costs. Other endpoints are not rate-limited in v1 because they are local or cluster-internal.
