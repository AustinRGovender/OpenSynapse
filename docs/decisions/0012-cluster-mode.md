# ADR-0012: Cluster mode — Postgres, object storage and k6-operator workers

**Date:** 2026-10-28
**Status:** accepted
**Authors:** OpenSynapse team

## Context

Phase 10 asks for a Helm install on Kubernetes where runs execute on the
k6-operator, metrics from distributed workers aggregate correctly, live control
reaches every worker within five seconds, data lives in Postgres and raw
samples in MinIO. The review found that the chart deployed a control plane that
could not work (it listened on loopback, kept the database password in a
ConfigMap, had no persistent encryption key, no RBAC and no operator), and
that nothing in the control plane used Postgres, object storage or the
operator.

## Decision

**Execution.** With `OPENSYNAPSE_MODE=cluster` the engine submits each run as a
k6-operator `TestRun` with `parallelism` equal to the run's worker count. The
script goes in a ConfigMap; the run's environment (variables, secrets,
live-control URL and token) goes in a Secret the runner pods load as
environment variables. Runner pods use the control-plane image, which carries
k6 with xk6-kv. The control plane needs RBAC for TestRuns, ConfigMaps, Secrets,
Pods and `pods/log` in its namespace, and talks to the API server with a small
REST client (`internal/kube`) rather than client-go.

**Metrics.** The spec suggested Prometheus remote-write. Instead each runner
writes k6's JSON samples to stdout (`K6_OUT=json=/dev/stdout`) and the control
plane follows every runner pod's log, feeding the samples to the same
aggregator a local run uses (ADR-0010). Aggregation is therefore exact: totals,
per-second points and percentiles are computed from every sample of every
worker, not merged from per-worker summaries. The VU gauge is tracked per
worker and summed. Log streams that the API server closes are resumed from
`sinceTime`, skipping lines already seen (lines from one container write share
a timestamp, so the count at the boundary timestamp is remembered).

**Live control.** VU, vus-max and stop changes are sent to every runner's k6
REST API (pod IP, port 6565) in parallel; k6 scales run-wide VU numbers by each
runner's execution segment. The internal control scenario gets one VU per
worker so every worker receives RPS, pause and stop state, and each worker
paces `rps_target / OPENSYNAPSE_INSTANCES`. Exit codes are combined (a failure
outranks failed thresholds, which outrank a requested stop). On start-up the
control plane deletes TestRuns left by a previous instance, since their runs
are marked interrupted.

**Database.** Stores keep SQLite's SQL dialect. On Postgres a database/sql
driver wrapper rewrites `?` placeholders to `$n` and `json_extract(col, '$.k')`
to `col::jsonb ->> 'k'`, and stores booleans as 0/1. Each migration has a
`NNNN_name.up.postgres.sql` twin with the same version, per the migration
skill's convention. Setting `OPENSYNAPSE_TEST_POSTGRES_DSN` runs the whole Go
test suite against Postgres.

**Object storage.** Raw sample archives go through an `artifacts.Store`: the
data directory locally, or an S3-compatible bucket (`MINIO_*` settings) in a
cluster. MinIO no longer publishes community container images (the Docker Hub
repository is gone and Quay requires authentication), so the chart bundles
RustFS 1.0.4 (Apache-2.0, S3-compatible) instead; any S3 service works via
`externalObjectStorage`. The S3 code was verified against RustFS with a 16 MiB
streaming multipart upload, listing, batch delete and missing-object handling.

**Chart.** One control-plane replica (live runs are tracked in memory).
Credentials (database, object storage, the secret-encryption key) are
generated on install and preserved on upgrade. The web image's nginx upstream
is configurable (`OPENSYNAPSE_API_UPSTREAM`) so it can reach the control plane's
Service. `/health` and `/ready` skip the Host check because kubelet probes
connect by pod IP. The grafana/k6-operator chart is a dependency
(`installK6Operator`), pulled from Grafana's OCI registry.

## Consequences

- Verified on kind (Kubernetes 1.34): a 4-worker run at 8 VUs sustained
  80.3 req/s against 78.3 req/s for a single worker (2.6% apart, identical
  p50/p95); a live VU change reached all four workers in 2 s; an RPS limit of
  40 held at 40.0 req/s across four workers; kill aborted within 2 s; raw
  samples exported from the bucket; no TestRun, ConfigMap or Secret was left
  behind.
- Runner pods need network access to the control plane's Service (live
  control) and the control plane to the runners' pod IPs; restrictive
  NetworkPolicies must allow both.
- Following pod logs puts metric traffic through the API server. That is fine
  for the worker counts and sample rates OpenSynapse targets; very large tests
  may need a different transport later.
- Kubernetes 1.35+ needs cgroup v2 nodes; this is a cluster requirement, not
  an OpenSynapse one.
