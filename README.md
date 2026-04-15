# OpenSynapse

Self-hosted performance testing platform combining k6 scripting, visual test planning, and live test control.

## What is OpenSynapse

OpenSynapse is a self-hosted performance testing platform that blends the scripting power of k6, the visual test planning of JMeter, the approachable UX of BlazeMeter, and the endpoint-exploration ergonomics of Postman into a single installable product. It runs on a laptop, on a server via Docker, or on a Kubernetes cluster with the same codebase and the same user interface. It ships as a single installer per platform and requires no external SaaS account.

Current tools force a trade-off. JMeter gives you visual test building but a dated UI and heavy resource footprint. k6 is efficient and modern but code-only and cannot be modified mid-run without restart. BlazeMeter is polished but cloud-locked and expensive. Postman is ergonomic for endpoints but not a load tool. OpenSynapse takes the best of each without inheriting their limitations.

The product is self-contained. It does not phone home, does not require an account, and does not degrade when offline. AI features are opt-in and use keys the user supplies.

## Features

- **Visual plan builder** -- drag-and-drop test plan construction with a tree of scenarios, controllers, samplers, timers, and assertions
- **Live test control** -- modify active VUs, requests per second, and duration while a test is running, without restart
- **Test template gallery** -- pre-built archetypes (smoke, load, stress, spike, soak, breakpoint, ramp-up, step-load) with sensible defaults
- **Endpoint playground** -- Postman-style pane for constructing and executing individual requests with timing breakdown
- **Multi-engine crawler** -- crawl target applications to generate draft test plans with three selectable engines: Rod (headless Chromium for SPAs), Colly (fast HTTP for static sites), and OWASP ZAP (security-focused sidecar). Supports form login, bearer, and basic auth
- **AI analysis** -- opt-in, bring-your-own-key integration with OpenAI, Anthropic, or Azure OpenAI for run insights and bottleneck identification
- **Multi-run comparison** -- overlay view comparing metrics across selected runs with difference summaries
- **Export** -- reports in PDF, HTML, CSV, and JSON formats
- **Reusable fragments** -- library of pre-built plan components (login flows, CSRF extraction, pagination, cart checkout) plus user-defined fragments
- **JMeter import** -- import existing JMeter `.jmx` test plans and convert them to OpenSynapse plans
- **Distributed execution** -- scale from a single laptop to 100,000+ VUs on Kubernetes via the k6-operator

## Quick Start

### Desktop

Download the installer for your platform from the [Releases page](https://github.com/opensynapse/opensynapse/releases). Run the installer and open the application. No configuration required.

### Docker

```bash
# Clone the repository
git clone https://github.com/opensynapse/opensynapse.git
cd opensynapse

# Start services
docker compose -f deploy/docker/docker-compose.yml up -d

# Open the UI
open http://localhost:8080

# Optional: start with OWASP ZAP sidecar for security-focused crawling
docker compose -f deploy/docker/docker-compose.yml --profile security up -d
```

Or use the quick-start script:

```bash
./scripts/quick-start.sh
```

### Kubernetes

```bash
helm install opensynapse ./deploy/helm
kubectl port-forward svc/opensynapse 8080:8080
open http://localhost:8080
```

## Development

### Prerequisites

- Node.js 20+
- Go 1.22+
- pnpm
- k6 (for test execution)

### Setup

```bash
# Clone
git clone https://github.com/opensynapse/opensynapse.git
cd opensynapse

# Install frontend dependencies
pnpm install

# Start control plane + web app in dev mode
pnpm dev

# Run Go tests (from the control plane directory)
cd apps/control-plane
go test ./... -v

# Run frontend lint
pnpm lint
```

### Common commands

```bash
pnpm dev              # Start control plane + web app in dev mode
pnpm test             # Run all tests
pnpm lint             # Lint all packages
pnpm build            # Build all packages
go test ./...         # Run Go tests (from apps/control-plane/)
cargo tauri dev       # Start desktop app in dev mode
k6 inspect script.js  # Validate a generated k6 script
```

## Architecture

OpenSynapse is a three-tier system. The UI tier is a React single-page application. The control plane is a Go service that exposes a REST and WebSocket API, manages test plans and runs, persists results, and orchestrates workers. The worker tier is a pool of k6 processes, run locally as subprocesses on desktop installs and as Kubernetes pods via the k6-operator on cluster installs.

### Tech stack

| Layer            | Choice                                    |
| ---------------- | ----------------------------------------- |
| Control plane    | Go 1.22+                                  |
| Web app          | React 18 + TypeScript + Vite              |
| Desktop shell    | Tauri 2                                   |
| Load engine      | k6 with xk6-kv                            |
| Desktop DB       | SQLite + Parquet                           |
| Cluster DB       | Postgres + S3-compatible (MinIO)           |
| UI components    | shadcn/ui + Tailwind                       |
| Charts           | Recharts                                   |
| Plan graph       | React Flow                                 |
| State            | Zustand + TanStack Query                   |
| Routing          | TanStack Router                            |
| Crawler          | Rod, Colly, OWASP ZAP                      |
| Orchestration    | k6-operator                                |
| CI               | GitHub Actions                             |

## Project Structure

```
opensynapse/
├── apps/
│   ├── control-plane/      Go service (REST + WebSocket API)
│   ├── web/                React SPA
│   └── desktop/            Tauri 2 shell
├── packages/
│   ├── ui/                 Shared React components and design tokens
│   ├── plan-schema/        JSON schemas for plan node types
│   └── api-client/         Generated TypeScript API client
├── deploy/
│   ├── docker/             Dockerfiles and docker-compose
│   └── helm/               Helm chart for Kubernetes
├── demos/
│   └── target-app/         Sample HTTP server for testing against
├── docs/                   Specs, architecture, decisions
├── scripts/                Build, release, and dev tooling
└── .github/workflows/      CI and release pipelines
```

## Contributing

1. Fork the repository and create a feature branch from `main`.
2. Follow the existing code style and conventions.
3. Every REST endpoint must have tests. Every UI component must have a Storybook entry.
4. Do not commit secrets, `.env` files, or credentials.
5. Do not introduce hex colour values outside `packages/ui/src/tokens.ts`.
6. Run `pnpm lint` and `go vet ./...` before submitting a pull request.
7. Write a clear commit message describing the change.

See the spec documents in `docs/` for detailed architectural and design guidance.

## License

TBD
