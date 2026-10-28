# OpenSynapse — Implementation Progress

Status as verified end to end on 2026-10-28 (branch `review/fixes-2026-10`). An earlier
version of this table marked every phase complete in April 2026 on the strength of unit
tests alone; a full review on 2026-10-27 found the core run loop, live metrics and live
control did not work, and cluster mode, Storybook and the generated API client had not
been built. "Complete" below means the phase's acceptance criteria were exercised against
a running system, not just unit-tested.

| Phase | Description | Status | Verified | Notes |
| ----- | ----------- | ------ | -------- | ----- |
| -1 | Development environment setup | complete | 2026-04-14 | |
| 0 | Project scaffold | partial | 2026-10-28 | Web, control plane, Docker and Helm work. The Tauri desktop shell is a scaffold only: no sidecar wiring, no build. |
| 1 | Control plane core and data model | complete | 2026-10-28 | SQLite and Postgres; every route documented in `/openapi.json` and covered by tests. |
| 2 | Visual plan builder | complete | 2026-10-28 | Tree with drag-and-drop, canvas, all node property editors, thresholds, undo/redo, shortcuts. |
| 3 | Local execution engine | complete | 2026-10-28 | Real k6 metrics (ADR-0010); all built-in plans run and pass `k6 inspect`. |
| 4 | Live control | complete | 2026-10-28 | VUs, RPS limit (xk6-kv), duration, pause, stop, kill — verified locally and on 4 cluster workers. |
| 5 | Reports, history, comparison | complete | 2026-10-28 | Welch's t-test comparison; JSON, CSV, HTML and PDF exports. |
| 6 | Endpoint playground | complete | 2026-10-28 | Seven auth methods incl. OAuth 2.0 popup and client credentials, collections, micro-tests, add to plan. |
| 7 | Crawler | complete | 2026-10-28 | OpenAPI import (JSON and YAML, examples) of all four test APIs; Colly/Rod/ZAP crawls. Colly and Rod verified against the test shop (`/shop`), including form login and Basic auth. |
| 8 | AI analysis | complete | 2026-10-28 | Opt-in, user keys, prompt preview, rate limit and spend cap. Not exercised against a live provider (needs a real key). |
| 9 | Fragments and template gallery | complete | 2026-10-28 | |
| 10 | Cluster mode and distributed execution | complete | 2026-10-28 | Verified on kind (Kubernetes 1.34): 1 vs 4 workers within 0.6%, live control reaches all workers in 2 s, 30,780 req/s recorded exactly. Team mode and roles. See ADR-0012. |
| 11 | JMeter import, Docker polish, release engineering | partial | 2026-10-28 | JMeter import, Docker Compose and quick-start, README and how-to guide done. Not done: signed desktop installers and auto-update, multi-arch/cosign-signed images, a published Helm chart repository; the release workflow has not been run. |
| 12 | Multi-engine crawler | complete | 2026-10-28 | |
| 13 | User testing infrastructure and seed plans | complete | 2026-10-28 | Test APIs (Compose and Kubernetes manifests), six built-in plans. |

## Quality gates in CI

- Go: build, vet, tests on SQLite **and** Postgres, govulncheck, the k6 output extension's tests.
- Web: build, lint, Vitest component tests, Storybook build (every component has stories), dependency audit.
- API client: regenerated from the OpenAPI document and checked for drift.
- Helm: dependency fetch, lint, template. Docker: images build, and the bundled k6 has xk6-kv and the opensynapse output.

## Known limitations

- **Desktop app**: not built (see Phase 0 and 11).
- **AI analysis** has not been tried against a live provider in this review.
- **Cluster runs** keep no raw-sample archive; CSV export gives per-second data.
- Cluster nodes on Kubernetes 1.35+ need cgroup v2.
