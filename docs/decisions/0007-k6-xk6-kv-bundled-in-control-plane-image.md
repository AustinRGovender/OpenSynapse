# ADR-0007: Bundle k6 + xk6-kv inside the control-plane Docker image

**Date:** 2026-04-16
**Status:** accepted
**Authors:** OpenSynapse team

## Context

The control-plane Go service spawns k6 as a subprocess to execute test runs. At startup it resolves the binary once via `exec.LookPath("k6")` (`apps/control-plane/internal/engine/engine.go:72`). If the lookup fails the engine is set to `nil`, `main.go` logs a warning, and every `POST /api/v1/runs` thereafter returns `503 ENGINE_NOT_AVAILABLE`.

Until this ADR the control-plane image (`deploy/docker/Dockerfile.control-plane`) installed only `ca-certificates`, `chromium`, `nss`, `freetype`, and `harfbuzz` for the Rod crawler. k6 was not installed, so every containerised deployment hit `ENGINE_NOT_AVAILABLE` on the first run attempt. Observed in the field on 2026-04-16.

The engine is not vanilla k6. Per `docs/02-architecture.md` §3 and `docs/06-implementation-plan.md` §3.1, live control requires:

- k6's externally-controlled executor, plus
- a token-bucket throttle that reads `rps_target` from **xk6-kv**, plus
- a watchdog that reads `should_stop` and `deadline_ms` from **xk6-kv**.

`CLAUDE.md` §Hard constraints pins this design ("Live control uses the externally-controlled executor + xk6-kv token bucket. No other approach."). The official `grafana/k6` image does **not** include xk6-kv, so it cannot satisfy the constraint on its own.

## Decision

Bundle a custom-built k6 binary — built from `go.k6.io/xk6` with `github.com/oleiade/xk6-kv` — inside the control-plane image via a dedicated builder stage.

`deploy/docker/Dockerfile.control-plane` gains a second builder stage:

```dockerfile
FROM golang:1.25-alpine AS k6-builder
RUN apk add --no-cache git
RUN go install go.k6.io/xk6/cmd/xk6@latest
RUN xk6 build --with github.com/oleiade/xk6-kv --output /k6
```

and the final stage copies the resulting binary:

```dockerfile
COPY --from=k6-builder /k6 /usr/local/bin/k6
```

The binary is placed on the default `PATH`, so `exec.LookPath("k6")` inside the running container resolves it without any configuration.

CI is extended to load the built image and run `k6 version` against it, asserting that `xk6-kv` appears in the extension list. This catches silent regressions (e.g. a future Dockerfile edit that drops the k6-builder stage).

## Alternatives considered

- **Copy from `grafana/k6:latest`.** Smallest diff (one `COPY --from=...` line) and fastest build, but the image does not ship xk6-kv, so live RPS control and the should-stop watchdog would fail at runtime. Violates the hard constraint in `CLAUDE.md`.
- **Separate `opensynapse-worker` image running k6.** `docs/02-architecture.md` §4 alludes to a future worker container. Still required for Phase 10 cluster mode, but not a prerequisite for local (single-machine) Docker Compose runs. Deferred; this ADR covers the single-container path only.
- **Install k6 via `apk add k6`.** Not available in Alpine's package repositories.
- **Expect the operator to mount a k6 binary from the host.** Brittle across host OSes (the user's Windows `k6.exe` will not execute in a Linux container) and contradicts the "clone and compose up" onboarding story in `docs/07-dev-environment.md`.
- **Remove the `engine = nil` fallback and fail startup loudly.** Good hygiene but orthogonal to this fix and would regress the desktop/CLI story where the user may legitimately not have k6 installed yet. Kept for a separate ADR if we want to enforce it in Docker mode only.

## Consequences

- **Runs work out of the box in Docker.** `docker compose up` yields a control-plane container that can execute runs without any operator action.
- **Build time grows.** The k6-builder stage needs to download the xk6 toolchain and compile a custom k6 binary (~30–60 s on a warm cache, 1–2 min cold). Amortised by Docker layer caching.
- **Spec-compliant.** xk6-kv is present, so the live-control token bucket and watchdog behave as specified.
- **Reproducibility risk from unpinned versions.** `xk6@latest` and `xk6-kv@main` float. Tracked as follow-up: pin both to specific versions once the initial rollout stabilises. The CI smoke check (`k6 version` must include `xk6-kv`) gates regressions in the meantime.
- **Image size grows modestly** (~30 MB for the k6 binary). No base image change.
- **A new CI gate exists**: the Docker job now runs the control-plane image and validates k6/xk6-kv. A future change that removes the k6 builder stage will fail CI with a clear message instead of silently shipping a broken image.
- **Desktop and Tauri shells are unaffected.** They continue to rely on a user-installed host k6. A separate ADR should revisit that path if we decide to bundle k6 with the desktop installer.
