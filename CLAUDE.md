# OpenSynapse

OpenSynapse is a self-hosted performance testing platform combining k6 scripting, visual test planning, and live test control. See `docs/01-PRD.md` for the full vision.

## Tech stack

| Layer            | Choice                                    |
| ---------------- | ----------------------------------------- |
| Control plane    | Go 1.22+                                  |
| Web app          | React 18 + TypeScript + Vite              |
| Desktop shell    | Tauri 2                                   |
| Load engine      | k6 with xk6-kv (do not substitute)        |
| Desktop DB       | SQLite + Parquet                           |
| Cluster DB       | Postgres + S3-compatible (MinIO)           |
| UI components    | shadcn/ui + Tailwind                       |
| Charts           | Recharts                                   |
| Plan graph       | React Flow                                 |
| State            | Zustand + TanStack Query                   |
| Routing          | TanStack Router                            |
| Crawler          | Rod, Colly, OWASP ZAP (see ADR-0003)       |
| Orchestration    | k6-operator                                |
| CI               | GitHub Actions                             |

## Where to find things

- Product requirements: `docs/01-PRD.md`
- Architecture and live-control mechanics: `docs/02-architecture.md`
- Feature specifications: `docs/03-feature-spec.md`
- Data model and API contracts: `docs/04-data-model-and-api.md`
- UI/UX design language and tokens: `docs/05-ui-ux-spec.md`
- Implementation plan (phases): `docs/06-implementation-plan.md`
- Dev environment setup: `docs/07-dev-environment.md`
- Progress tracker: `docs/progress.md`
- Architecture decisions: `docs/decisions/`

## Hard constraints

- No hex colours outside `packages/ui/src/tokens.ts`. The hex colour guard hook enforces this.
- No commits of `.env`, `.key`, `.pem`, or `creds*` files.
- No phone-home code. The product never contacts external services without explicit user action.
- k6 is the load engine. Do not substitute another engine.
- Every REST endpoint must have tests. Every UI component must have a Storybook entry.
- Live control uses the externally-controlled executor + xk6-kv token bucket. No other approach.
- AI features are opt-in and use keys the user supplies. No data sent without explicit user click.

## Current phase

See `docs/progress.md` for the current implementation phase and status.

## How to work

Work phase by phase per `docs/06-implementation-plan.md`. Stop at phase boundaries for user confirmation. Commit at the end of each phase with the message format `Phase N: <short description>`. Run acceptance tests before committing.

When making decisions not covered by the specs, use `/adr` to record them in `docs/decisions/`. When unsure about a spec, use `/spec <question>` to consult the spec-consultant subagent. When you need to understand existing code, use `/explore <question>`. When you want a code review before committing, use `/review`.

## Common commands

```bash
pnpm dev              # Start control plane + web app in dev mode
pnpm test             # Run all tests
pnpm lint             # Lint all packages
pnpm build            # Build all packages
go test ./...         # Run Go tests (from apps/control-plane/)
cargo tauri dev       # Start desktop app in dev mode
k6 inspect script.js  # Validate a generated k6 script
```

## Repository layout

```
apps/control-plane/   Go service (REST + WebSocket API)
apps/web/             React SPA
apps/desktop/         Tauri 2 shell
packages/ui/          Shared React components and design tokens
packages/plan-schema/ JSON schemas for plan node types
packages/api-client/  Generated TypeScript API client
deploy/docker/        Dockerfiles and docker-compose
deploy/helm/          Helm chart for Kubernetes
docs/                 Specs, decisions, progress
scripts/              Build and dev tooling
```

## Never do

- Never commit secrets (API keys, tokens, passwords, .env files)
- Never use `rm -rf /` or destructive commands against the repo root
- Never bypass hooks with `--no-verify`
- Never write hex colours in component files (add to tokens.ts first)
- Never modify spec documents in `docs/01-*` through `docs/07-*` without asking
- Never substitute k6 with another load engine
- Never add phone-home or telemetry code
