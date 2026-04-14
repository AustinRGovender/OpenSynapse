# ADR-0002: Use modernc.org/sqlite instead of mattn/go-sqlite3

**Date:** 2026-04-14
**Status:** accepted
**Authors:** Claude Code

## Context

The control plane needs an embedded SQLite driver for Go. The two main options are `mattn/go-sqlite3` (CGO-based, wraps the C SQLite library) and `modernc.org/sqlite` (pure Go, transpiled from C via c2go).

During Phase 1 implementation, `mattn/go-sqlite3` failed to build because CGO requires a C compiler (GCC) which was not available in the development environment (Windows without MinGW).

## Decision

Use `modernc.org/sqlite` as the SQLite driver. It registers as the `"sqlite"` driver (not `"sqlite3"`). Connection string pragmas use `_pragma=key(value)` syntax.

## Alternatives considered

**mattn/go-sqlite3**: The most popular Go SQLite driver. Requires CGO and a C compiler. Faster than the pure-Go option by ~10-30% for write-heavy workloads. Rejected because it adds a build dependency (GCC/MinGW) that complicates cross-platform development and CI.

**Install GCC**: Would fix the mattn driver but adds a prerequisite to every development machine and makes the CI pipeline more complex.

## Consequences

- No C compiler needed to build the control plane on any OS.
- Slightly slower for large batch writes (acceptable for our workload; metric samples go to Parquet, not SQLite).
- The Dockerfile can use a simpler base image since no CGO compilation is needed.
- Binary size is larger (~15MB increase from the transpiled SQLite code).
