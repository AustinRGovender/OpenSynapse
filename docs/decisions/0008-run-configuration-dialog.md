# ADR-0008: Run Configuration Dialog

**Date:** 2026-04-17
**Status:** accepted
**Authors:** OpenSynapse team

## Context

Clicking "Run" in the plan builder sent `POST /api/v1/runs` with only `{ plan_id }`, defaulting to 10 VUs / 30s. Users had no way to select a test type (Smoke, Load, Stress, etc.) or adjust parameters before starting a run. The 9 load-profile templates were only used at plan creation time in the TemplateGallery, but their configurations are equally useful when launching a run.

## Decision

Intercept the Run button in PlanBuilder with a RunConfigDialog modal that:

1. Offers a "Use Plan Defaults" option (selected by default) that reads the plan's scenario node properties for quick-start users.
2. Displays the 9 existing template cards (reusing the shared `template-data.ts` and `LoadCurveSvg` component) for selecting a different load profile.
3. Shows an editable parameter panel below the selection (VUs, duration, stages table, or arrival rate fields depending on executor type).
4. On confirm, sends `POST /api/v1/runs` with `{ plan_id, parameters: { vus_target, duration_seconds, rps_target? } }`.

No backend changes are required — `handlers/runs.go` already accepts the `parameters` field with `vus_target`, `rps_target`, and `duration_seconds`.

## Alternatives considered

- **Inline parameter fields in the toolbar**: Too cramped for the ramping-vus stages editor and doesn't surface test-type choices.
- **Separate "Run Settings" page**: Breaks the flow — users want to go from plan editing to run quickly.
- **Always use plan defaults**: Requires editing the plan tree to change load profile for a single run, which is destructive to the plan.

## Consequences

- Users can adjust load parameters per-run without modifying their plan.
- Template data is now shared between TemplateGallery (plan creation) and RunConfigDialog (run launch), reducing duplication.
- The dialog adds one extra click for users who always want defaults, but "Use Plan Defaults" is pre-selected so they can press Enter/click Run immediately.
