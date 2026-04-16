# ADR-0004: Hash-based client router instead of TanStack Router

**Date:** 2026-04-16
**Status:** accepted
**Authors:** OpenSynapse team

## Context

`docs/02-architecture.md` §8 and the tech-stack tables in `CLAUDE.md` and `README.md` specify TanStack Router for the SPA. The current implementation in `apps/web/src/App.tsx:15` uses a hand-rolled `useHash` hook plus regex dispatch over `window.location.hash`. Neither `@tanstack/react-router` nor `@tanstack/react-query` appears in `apps/web/package.json`.

The deviation was introduced during scaffolding and never recorded. The current SPA has seven routes (plans list, plan builder, runs list, run view, comparison, playground, crawler, settings) and all are terminal pages — no nested layouts, no route-level data loaders, no programmatic type-safe navigation beyond string hashes.

## Decision

Keep the hand-rolled hash router. Record it as a deliberate deviation from the original tech-stack spec.

The router lives entirely in `App.tsx` as:

- a `useHash` hook that subscribes to `hashchange`,
- a sequence of regex matches that map `#/runs/:id`, `#/compare?ids=…`, `#/plans/:id`, etc. to page components,
- `window.location.hash = "…"` for navigation.

Migrate to TanStack Router only if one of these triggers fires:

- route count exceeds ~15, or nested layouts become genuinely useful;
- we need route-level data loaders that TanStack Query + `useEffect` cannot cleanly express;
- we need type-safe search params or pending states surfaced to the router layer.

## Alternatives considered

- **TanStack Router (spec default).** Rejected for now: ~30 kB added to the bundle and a non-trivial migration surface for zero current payoff. Every existing page self-fetches via `useEffect` or Zustand stores; the router does not need to orchestrate that.
- **React Router v7.** Same bundle cost, lower type-safety payoff than TanStack. Not an improvement over the status quo.
- **Retrofit TanStack Router now.** Rejected as over-engineering per CLAUDE.md's "avoid over-engineering" guidance. Simple hash routing is the minimum that meets current requirements.

## Consequences

- Zero router bundle cost; navigation is `window.location.hash = …`.
- No type safety on route params — every page parses its own ID out of a match group.
- No route-level data loaders; each page manages its own fetching via `useEffect`.
- Deep-linking works (URLs are shareable), browser back/forward works (via `hashchange`).
- The spec documents (`docs/02-architecture.md` §8, §14; `CLAUDE.md` tech stack; `README.md` tech stack) still list TanStack Router. They should be updated to reference this ADR in the same manner ADR-0001 (React 19) and ADR-0003 (crawler) are referenced, but the spec documents themselves are protected from modification per CLAUDE.md's "Never do" list without explicit user approval.
- TanStack Query is also absent; server-state caching is done ad hoc in each page. That deviation is a separate concern tracked in project tech debt, not in this ADR.
