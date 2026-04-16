# ADR-0006: Parse k6 run summary from stdout (not `handleSummary` / `--summary-export`)

**Date:** 2026-04-16
**Status:** accepted
**Authors:** OpenSynapse team

## Context

After a run finishes, the control plane needs the aggregate summary (total requests, p50/p90/p95/p99, error rate, RPS) to persist as a `db.RunSummary`. The current implementation in `apps/control-plane/internal/engine/engine.go:398` (`parseK6Summary`) joins captured stdout lines into one string and applies a set of regex patterns to scrape numbers out of k6's default end-of-test summary text:

```go
patterns := map[string]*float64{
    `http_reqs[.\s]+(\d+)`:                   nil,
    `http_req_duration.*p\(95\)=([0-9.]+)ms`: &summary.P95MS,
    ...
}
```

k6 offers two structured alternatives that do not depend on the stability of its text output:

1. `--summary-export=<path>` writes a JSON file on exit with the same metric tree as the text summary.
2. `handleSummary(data)` inside the test script lets us emit arbitrary JSON or write to any destination.

The stdout-regex approach was a pragmatic shortcut during Phase 3 and was never revisited. It breaks silently when a k6 release tweaks the text format (spacing, symbols, the `…` ellipsis in 0.50+ releases, sub-metric indentation). `grep`-style matches can also cross-match: the `http_req_duration.*p(95)=…ms` pattern is anchored to whatever k6 prints on the `http_req_duration` line and would match the wrong sub-metric if the text layout ever changes.

## Decision

**Keep the stdout regex parser for now.** It works against the k6 versions we currently bundle, and the failure mode is graceful: unmatched patterns leave zero values in `RunSummary` rather than crashing. Runs continue to persist; the dashboard simply shows zeros in fields the regex missed.

Plan a follow-up migration to `--summary-export` that is triggered on any of:

- a pinned k6 version upgrade (CI catches summary regressions via compiler/engine integration tests);
- a user report of blank summary stats;
- any change that requires a summary field not present in the current regex table (for example, per-group or per-tag breakdowns).

Migration path is well-defined and cheap:

1. Pass `--summary-export=<tmp>.json` in the args at `engine.go:111`.
2. On process exit, read the JSON instead of scanning stdout.
3. Map the k6 `metrics` tree to `db.RunSummary` fields.
4. Delete `parseK6Summary` and the `regexp` import.

## Alternatives considered

- **Migrate to `--summary-export` now.** The cleanest answer technically but unneeded churn: all Phase 3–12 tests pass against the current parser, and a spec-compliant structured parser is not on any user-facing critical path today. Marked as follow-up instead.
- **Inject a `handleSummary` function in the generated script.** Gives us full control of the summary shape but spreads summary logic across the compiler and the engine. `--summary-export` keeps the summary concern in the engine.
- **Remote-write only; no summary.** k6's Prometheus remote-write output already delivers every sample in real time. We could compute the summary from the ring buffer on run completion. Rejected because Phase 10 cluster mode depends on remote-write but desktop mode does not always have it wired; keeping a single summary path that works in both modes is simpler.

## Consequences

- **Fragility acknowledged.** A k6 text-format change between versions will blank out summary fields. The dashboard degrades gracefully (zeros, not a crash), but the failure is silent until a user notices.
- No additional dependencies, no changes to the generated script template, no extra temp-file management.
- Migration is a single-file change in `engine.go` and a small test update. Deferred cost is low.
- The `parseK6Summary` function should be covered by a regression test that pins the expected text format against a committed sample of k6 stdout; without that, version drift is invisible. (Tracked as follow-up tech debt.)
