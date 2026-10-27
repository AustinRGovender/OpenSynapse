# ADR-0010: Run metrics and live control

**Date:** 2026-10-27
**Status:** accepted (supersedes ADR-0006)
**Authors:** OpenSynapse team

## Context

A review of the shipped engine found that the core run loop did not work as specified:

- Live charts only carried a real VU count; RPS, p95 and error rate were hard-coded to zero.
- The final summary came from regexes over k6's text summary (ADR-0006). They did not match k6's actual output (`http_reqs....: 25261`), so every run reported 0 requests and 0% errors.
- The engine always passed `--vus`/`--duration` on the command line, which in k6 replaces the script's `scenarios` entirely: ramping, spike and arrival-rate profiles were silently flattened into a constant 10-VU, 30-second run, and the externally-controlled executor was discarded, so live VU changes were rejected by k6 (while the API reported success).
- The "xk6-kv token bucket" never imported xk6-kv; RPS and stop flags were read from `__ENV` once at start-up and could not change.

The architecture document specifies Prometheus remote-write ingestion into a ring buffer, Parquet sample files, a control-plane-written xk6-kv store, and the externally-controlled executor for every run.

## Decision

**Metrics.** k6 runs with `--out json=<file>`. The engine tails the file and aggregates samples into one point per second (requests, failures, error %, p50/p90/p95/p99 from that second's exact durations, active VUs from the `vus` gauge, bytes, iterations, checks) plus whole-run histograms (log-bucketed, ±0.25%), a per-endpoint breakdown, per-assertion tallies, and a recent-error log. Points are broadcast on `runs.{id}.metrics` and stored in `run_metrics`; errors go to `run_errors` and `runs.{id}.errors`. The run summary is computed from the same data, so live and historical numbers always agree. Raw samples are archived as gzip JSON lines in `<data dir>/runs/{id}/samples.jsonl.gz`, which feeds the "raw samples" CSV export.

**Load profile and live VUs.** In managed mode the compiler converts the plan's single VU-based scenario (constant-vus, ramping-vus, externally-controlled) to `externally-controlled` with `maxVUs` at the profile's peak and `duration` at the planned length plus a 50% buffer (at least 5 minutes). The engine drives the plan's stages itself through k6's REST API (`PATCH /v1/status {vus}`), reproducing ramping-vus' linear ramps. A live VU change switches the run to manual control (the UI can hand control back to the profile); raising VUs above `maxVUs` raises `vus-max` first. Arrival-rate and iteration-based scenarios keep their native executors; live VU changes are rejected with a clear error for them. Run-dialog overrides are compiled into the script; no execution options are passed on the command line.

**Duration and stop.** The engine owns the run's end time. When it passes (excluding paused time) or the user clicks Stop, the engine sends `PATCH /v1/status {stopped: true}`; k6 exits with code 103, which the engine treats as a normal completion. Extending is allowed up to the buffered maximum.

**RPS and the kv store.** The control plane cannot write into k6's in-process xk6-kv store, so the managed script includes an internal `__opensynapse_control` scenario (one VU) that polls `GET /api/v1/runs/{id}/control-state` once a second (authenticated with a per-run token) and writes `rps_target` and `should_stop` into an xk6-kv memory store. Every iteration reads them: when `should_stop` is set the iteration is skipped (the watchdog), and `__osPace()` spaces each VU's requests at `active VUs / rps_target` seconds so all VUs together stay at or under the target without contending on a shared counter. The control scenario's samples are excluded from all results. When the k6 binary lacks xk6-kv (a stock k6 on a developer machine), runs still work and VU, pause, duration and stop control still apply; RPS control is reported as unavailable.

**k6 version.** k6 v2 removed the externally-controlled executor, and xk6-kv v1.4+ only builds against k6 v2. The bundled engine is therefore pinned to the latest k6 1.x (v1.8.2) with xk6-kv v1.3.0. The engine passes `--no-usage-report` and `K6_AUTO_EXTENSION_RESOLUTION=false` so k6 never contacts Grafana services.

**Standalone scripts.** "Show Script" and the compile endpoint still produce a portable script that runs with stock k6 exactly as the plan describes (native executors, no kv). `?mode=managed` shows the script OpenSynapse actually runs.

## Alternatives considered

- **Prometheus remote-write ingestion (as specified).** Needs protobuf/snappy decoding and only carries cumulative trend statistics unless native histograms are enabled; per-second percentiles would be approximate. JSON output gives exact per-sample data at desktop scale with no new dependencies. Remote-write remains the right transport for distributed cluster runs, where workers cannot share a file.
- **Parquet sample files.** Adds a large dependency for a file nobody reads directly; per-second aggregates in the database serve every chart and comparison, and the gzip archive keeps raw samples for CSV export.
- **Polling k6's REST `/v1/metrics`.** Only exposes cumulative trend values, so live p95 would lag badly.
- **Keeping native ramping executors.** Would make live VU control impossible for the Load, Stress, Spike and Step templates, which is the product's headline feature.
- **Upgrading to k6 v2.** Removes the executor live control depends on.

## Consequences

- Live charts, run summaries, reports and comparisons now show real numbers, and they agree with each other.
- Every VU-based profile can be adjusted mid-run; arrival-rate profiles cannot (k6 limitation).
- k6's JSON output adds CPU and disk overhead at very high request rates; desktop targets (≤2,000 VUs) are comfortably within budget. Cluster mode should move to remote-write.
- The bundled k6 is held on the 1.x line until an equivalent of the externally-controlled executor exists in k6 v2.
- Supersedes ADR-0006 (stdout summary parsing).
