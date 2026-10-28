# OpenSynapse

A self-hosted load-testing tool. Build test plans visually, run them with
[k6](https://k6.io), watch the results live, and change the load — virtual users,
requests per second, duration — while the test is running. It runs on your laptop,
on a server with Docker, or on Kubernetes, and never contacts outside services unless
you ask it to.

![A run with live control](docs/user/images/08-live-control.png)

**New here? Start with the [how-to guide](docs/user/HOW-TO.md)** — it walks through
everything with screenshots.

---

## Quick start (5 minutes)

You need [Docker](https://docs.docker.com/get-docker/) with the Compose plugin.

```bash
git clone https://github.com/opensynapse/opensynapse.git
cd opensynapse
docker compose -f user-testing/docker-compose.yml up -d --build
```

Open **http://localhost:8080**. Then:

1. Click the plan **E-commerce — Browse & Buy**.
2. Click **Run**, then **Start run**.
3. Watch the live dashboard. Try typing a new number of virtual users in **Live control**
   and clicking **Apply**.

This also starts four small **test APIs** (echo, e-commerce, slow, error) that the six
built-in plans use, so nothing you do touches a real system. They are described in
[section 15 of the guide](docs/user/HOW-TO.md#15-the-bundled-test-apis).

| Address | What it is |
| --- | --- |
| http://localhost:8080 | The web app |
| http://localhost:8090 | The API (`/openapi.json` describes it) |
| http://localhost:9001 – 9004 | The test APIs |

Stop it with `docker compose -f user-testing/docker-compose.yml down`. Your plans and
results stay in a Docker volume; add `-v` to delete them.

## What you can do

- **Build plans visually** — scenarios, requests, transactions, conditions, loops,
  assertions, think times, test data, and code blocks — or import JMeter `.jmx` files.
  Every plan is also a plain k6 script you can copy out.
- **Start from a template** — smoke, load, stress, spike, soak, breakpoint, ramp-up and
  step-load profiles.
- **Control a test live** — change VUs, cap requests per second, extend or shorten the
  run, pause, stop or kill it; every change is marked on the charts.
- **Read and compare results** — per-second charts, per-endpoint and per-assertion
  tables, an error log, pass/fail thresholds, statistically tested comparisons between
  runs, and exports to JSON, CSV, HTML and PDF.
- **Explore endpoints** in a Postman-style playground with seven auth methods (including
  OAuth 2.0), collections, and one-click micro-tests.
- **Generate plans** by crawling a site or importing an OpenAPI/Swagger document.
- **Work as a team** with sign-in, viewer/editor/admin roles and personal API tokens.
- **Scale out** on Kubernetes: one run spread over many k6 workers.
- **Ask an AI** about a run, optionally, with your own API key — nothing is sent until
  you click Send.

## Ways to run it

### 1. Docker (recommended)

| Command | Starts |
| --- | --- |
| `docker compose -f user-testing/docker-compose.yml up -d --build` | OpenSynapse **and** the test APIs |
| `docker compose -f deploy/docker/docker-compose.yml up -d --build` | OpenSynapse only |
| `docker compose -f deploy/docker/docker-compose.yml --profile security up -d --build` | OpenSynapse plus an OWASP ZAP daemon for the crawler |

The image bundles k6 (v1.8.2) with the extensions live control needs, and Chromium for
the browser-based crawler. Data lives in the `opensynapse-data` volume.

**Testing something on your own machine from Docker?** Requests are sent from inside
the container, so use `http://host.docker.internal:<port>` rather than `localhost`.

### 2. From source (for development)

Prerequisites: Go 1.26+, Node.js 22.12+ with pnpm 9+, and k6. For live RPS limits and
pause, k6 must include [xk6-kv](https://github.com/oleiade/xk6-kv); plain k6 works for
everything else.

```bash
pnpm install
pnpm dev            # control plane on :8090 and web app on http://localhost:5173
```

To run the test APIs too: `docker compose -f user-testing/docker-compose.yml up -d
echo-api mock-ecommerce slow-api error-api` and use the **Test APIs (localhost)**
environment.

Optional: build the same k6 the Docker image uses —

```bash
go install go.k6.io/xk6/cmd/xk6@v1.4.15
xk6 build v1.8.2 --with github.com/oleiade/xk6-kv@v1.3.0 \
  --with github.com/opensynapse/opensynapse/apps/k6-output=./apps/k6-output
export OPENSYNAPSE_K6_PATH=$PWD/k6
```

### 3. Kubernetes (cluster mode)

The Helm chart installs OpenSynapse with Postgres, S3-compatible object storage
(RustFS) and the [k6-operator](https://github.com/grafana/k6-operator). Each run becomes a
k6-operator `TestRun` whose workers share the load; you choose the number of workers in
the Run dialog.

```bash
helm dependency update deploy/helm
helm install opensynapse deploy/helm --namespace opensynapse --create-namespace \
  --set controlPlane.adminEmail=you@example.com \
  --set controlPlane.adminPassword='a-long-password'
kubectl -n opensynapse port-forward svc/opensynapse-web 8080:8080
```

Then open http://localhost:8080 and sign in. Useful values (see
[`deploy/helm/values.yaml`](deploy/helm/values.yaml)):

| Value | Default | Purpose |
| --- | --- | --- |
| `controlPlane.image`, `web.image` | `ghcr.io/opensynapse/…:0.2.0` | Images to run |
| `controlPlane.teamMode` | `true` | Require sign-in |
| `workers.maxPerRun` | `16` | Most workers one run may use |
| `workers.resources` | 250m CPU / 256Mi | Resources for each worker pod |
| `postgres.enabled` / `externalDatabase.dsn` | bundled | Use your own Postgres |
| `objectStorage.enabled` / `externalObjectStorage.*` | bundled | Use S3, MinIO or another S3 service |
| `installK6Operator` | `true` | Set `false` if the cluster already runs the operator |
| `ingress.*` | off | Expose it through an ingress |

**Trying it on kind:** build and load the images, deploy the test APIs, then install
with your local images:

```bash
kind create cluster --name opensynapse   # Docker on cgroup v1? add --image kindest/node:v1.34.11
docker build -f deploy/docker/Dockerfile.control-plane -t opensynapse/control-plane:dev .
docker build -f deploy/docker/Dockerfile.web -t opensynapse/web:dev .
docker build -t opensynapse/echo-api:dev user-testing/test-apis/echo-api
docker build -t opensynapse/mock-ecommerce:dev user-testing/test-apis/mock-ecommerce
kind load docker-image opensynapse/control-plane:dev opensynapse/web:dev \
  opensynapse/echo-api:dev opensynapse/mock-ecommerce:dev --name opensynapse
kubectl create namespace opensynapse
kubectl -n opensynapse apply -f user-testing/kubernetes/test-apis.yaml
helm dependency update deploy/helm
helm install opensynapse deploy/helm -n opensynapse \
  --set controlPlane.image.repository=opensynapse/control-plane --set controlPlane.image.tag=dev \
  --set web.image.repository=opensynapse/web --set web.image.tag=dev \
  --set controlPlane.adminEmail=admin@example.com --set controlPlane.adminPassword=change-me-please
```

How cluster mode works, and what was measured: [ADR-0012](docs/decisions/0012-cluster-mode.md).

### Desktop app

Not available yet. `apps/desktop` holds an early Tauri scaffold, but there is no desktop
build or installer; use Docker on your laptop instead.

## Configuration

The control plane is configured with environment variables. In Docker, set them under
`environment:` in the Compose file.

**Network and security**

| Variable | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8090` | API port |
| `OPENSYNAPSE_BIND` | `127.0.0.1` (`0.0.0.0` in Docker) | Listen address |
| `TEAM_MODE` | `false` | `true` requires sign-in, with users and roles. Turn it on before exposing OpenSynapse to anyone else. |
| `OPENSYNAPSE_ADMIN_EMAIL`, `OPENSYNAPSE_ADMIN_PASSWORD` | — | Create the first admin at start-up (team mode). Otherwise the first visitor creates one. |
| `OPENSYNAPSE_ALLOWED_HOSTS` | loopback only | Extra host names the API answers to (comma-separated), e.g. `perf.example.com`. Protects against DNS rebinding. |
| `OPENSYNAPSE_ALLOWED_ORIGINS` | `http://localhost:5173` | Extra browser origins allowed to call the API (the dev server's by default). |
| `OPENSYNAPSE_API_TOKEN` | generated | The local-mode access token (stored in the data directory when not set). |
| `OPENSYNAPSE_SECRET_KEY` | generated | Base64 32-byte key encrypting stored secrets. Keep it safe: without it, stored secrets can't be read. |
| `OPENSYNAPSE_PUBLIC_URL` | — | Your OpenSynapse address, used for links in webhook and Slack messages. |

**Data**

| Variable | Default | Meaning |
| --- | --- | --- |
| `OPENSYNAPSE_DATA_DIR` | `~/.opensynapse` (`/data` in Docker) | SQLite database, secret key and raw run samples |
| `DB_PATH` | `<data dir>/opensynapse.db` | SQLite file |
| `DB_DSN` | — | Use Postgres instead, e.g. `postgres://user:pass@host:5432/opensynapse?sslmode=require` |
| `MINIO_ENDPOINT`, `MINIO_BUCKET`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_USE_SSL` | — | Store raw run samples in any S3-compatible service instead of the data directory (`host:port`, no scheme) |

**Running tests**

| Variable | Default | Meaning |
| --- | --- | --- |
| `OPENSYNAPSE_K6_PATH` | `k6` on `PATH` | The k6 binary |
| `OPENSYNAPSE_CONTROL_URL` | `http://127.0.0.1:<PORT>` | How k6 reaches the control plane for live control |
| `OPENSYNAPSE_TEST_APIS` | auto | `docker` or `localhost`: which test-API environment the built-in plans use |
| `ROD_BROWSER_BIN` | auto | Chrome/Chromium for the browser crawler |
| `ZAP_API_URL` | `http://zap:8080` | OWASP ZAP daemon for the ZAP crawler |
| `OPENSYNAPSE_LOG_FORMAT` | `json` | `text` for human-readable logs |

**Cluster mode** (set by the Helm chart)

| Variable | Meaning |
| --- | --- |
| `OPENSYNAPSE_MODE=cluster` | Run tests as k6-operator TestRuns in the control plane's namespace |
| `OPENSYNAPSE_RUNNER_IMAGE` | Image for worker pods (k6 with xk6-kv and the opensynapse output; the control-plane image) |
| `OPENSYNAPSE_RUNNER_PULL_POLICY` | Worker image pull policy (default `IfNotPresent`) |
| `OPENSYNAPSE_RUNNER_RESOURCES` | Worker pod resources as JSON |
| `OPENSYNAPSE_MAX_WORKERS` | Most workers per run (default 16) |

## Security in brief

- A **local install** listens on loopback and only answers requests addressed to
  `localhost`; the browser gets an install token automatically. Anyone who can reach it
  on your machine can use it — turn on **team mode** before sharing it.
- Cross-site requests are refused, so other websites you visit can't drive it.
- Secrets in environments and AI keys are encrypted at rest (AES-256-GCM) and never
  returned by the API.
- No telemetry: k6 runs with usage reports and automatic extension downloads off, and
  OpenSynapse only contacts the systems you test, the webhooks you configure and the AI
  provider you choose — and the AI only when you click Send.

Details: [ADR-0011](docs/decisions/0011-security-model.md).

## Development

```bash
pnpm dev                     # control plane + web app with hot reload
pnpm test                    # all workspace tests
pnpm lint                    # all linters
pnpm --filter web storybook  # component explorer on http://localhost:6006
cd apps/control-plane && go test ./...
```

| What | How |
| --- | --- |
| Go tests on Postgres | `OPENSYNAPSE_TEST_POSTGRES_DSN=postgres://postgres:pw@localhost:5432/postgres?sslmode=disable go test ./...` |
| k6 output extension | `cd apps/k6-output && go test ./...` |
| Web unit and component tests | `pnpm --filter web test` (Vitest + Testing Library) |
| Storybook build (every component has stories) | `pnpm --filter web build-storybook` |
| Regenerate the TypeScript API client after API changes | `pnpm --filter @opensynapse/api-client generate` (CI fails if it is out of date) |
| Check a generated script | `k6 inspect script.js` |

### Project layout

```
apps/control-plane/   Go service: REST + WebSocket API, plan compiler, run engine
apps/k6-output/       k6 output extension used by cluster workers
apps/web/             React web app (Vite, Tailwind, Zustand, Recharts, React Flow)
apps/desktop/         Tauri scaffold (not yet built)
packages/api-client/  TypeScript client generated from the OpenAPI document
packages/plan-schema/ JSON schemas for plan nodes
packages/ui/          Design tokens (the only place colours are defined)
deploy/docker/        Dockerfiles and Compose file
deploy/helm/          Helm chart
user-testing/         Test APIs, their Compose file and Kubernetes manifests
docs/                 Specs, decisions (ADRs), user guide, progress
```

### Documentation

- [How-to guide](docs/user/HOW-TO.md) — using OpenSynapse
- [API error codes](docs/error-codes.md)
- [Architecture decisions](docs/decisions/)
- [Product requirements](docs/01-PRD.md), [architecture](docs/02-architecture.md),
  [feature spec](docs/03-feature-spec.md), [data model and API](docs/04-data-model-and-api.md),
  [UI/UX spec](docs/05-ui-ux-spec.md), [implementation plan](docs/06-implementation-plan.md),
  [dev environment](docs/07-dev-environment.md)
- [Progress](docs/progress.md)

## Contributing

1. Branch from `main`.
2. Every REST endpoint needs tests and every UI component a Storybook story.
3. Colours come from `packages/ui/src/tokens.ts` only.
4. Never commit secrets, `.env` files or credentials.
5. Run `pnpm lint`, `pnpm test` and `go vet ./...` before opening a pull request.

## License

TBD
