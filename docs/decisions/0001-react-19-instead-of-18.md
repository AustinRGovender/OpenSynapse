# ADR-0001: Use React 19 instead of React 18

**Date:** 2026-04-14
**Status:** accepted
**Authors:** Claude Code

## Context

The specification documents (02-architecture.md, CLAUDE.md) reference React 18 as the UI framework. However, the current Vite template (`create-vite@latest` with `react-ts` template) scaffolds with React 19 as the default. React 19 was released as stable in December 2024 and is now the default version in all major tooling.

## Decision

Use React 19 as scaffolded by the Vite template. React 19 is fully backwards-compatible with React 18 component patterns. All code written will be compatible with both versions. The spec's intent — a modern React SPA with TypeScript — is preserved.

## Alternatives considered

**Downgrade to React 18**: Would require pinning `react@18` and `react-dom@18`, and potentially older versions of `@vitejs/plugin-react` and other dependencies that now default to React 19 peer dependencies. This creates maintenance overhead for no functional benefit.

**Keep React 18 and pin all tooling versions**: Feasible but fragile. Future dependency updates would continuously fight the pinned versions.

## Consequences

- All React code uses standard patterns compatible with React 18 and 19.
- We get React 19 improvements (Actions, use(), improved Suspense) for free if needed later.
- Third-party libraries that only support React 18 may need their React 19-compatible versions, but all libraries in our stack (Recharts, React Flow, shadcn/ui) already support React 19.
