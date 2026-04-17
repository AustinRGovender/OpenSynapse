# User Testing Infrastructure

This directory contains everything needed to run OpenSynapse with real test APIs for demonstration and user testing.

## Quick Start

```bash
docker compose up --build
```

This starts:
- **OpenSynapse control plane** on `localhost:8090`
- **OpenSynapse web UI** on `localhost:8080`
- **Echo API** on `localhost:9001`
- **Mock E-commerce** on `localhost:9002`
- **Slow API** on `localhost:9003`
- **Error API** on `localhost:9004`

Open `http://localhost:8080` to access the web UI. Six built-in test plans are pre-loaded and ready to run.

## Test APIs

| API | Port | Purpose | Key Endpoints |
|-----|------|---------|---------------|
| echo-api | 9001 | Basic echo, delay, and status code simulation | `GET /health`, `POST /echo`, `GET /delay/:ms`, `GET /status/:code` |
| mock-ecommerce | 9002 | Multi-step e-commerce user journey | `GET /products`, `GET /products/:id`, `POST /orders`, `GET /orders/:id` |
| slow-api | 9003 | Configurable slow responses and flaky behavior | `GET /slow/:seconds`, `GET /timeout`, `GET /flaky` |
| error-api | 9004 | Error rate, rate limiting, and degradation simulation | `GET /random-error`, `GET /rate-limit`, `GET /degradation` |

Each API includes an OpenAPI spec at `test-apis/<api>/openapi.yaml`.

## Built-In Plans

The following plans are seeded on startup and appear in the plan list:

1. **Echo API -- Smoke Test** - Quick verification (1 VU, 30s, constant-vus)
2. **E-commerce -- Browse & Buy** - Realistic multi-step journey (ramping 10-50 VUs, 14m)
3. **Slow API -- Stress Test** - Find the breaking point (ramping 5-100 VUs, 8m)
4. **Error API -- Resilience Test** - Observe error behavior (10 VUs, 2m)
5. **Cross-API -- Spike Test** - Sudden burst across all APIs (spike 5-100-5 VUs)
6. **Echo API -- Delay Validation** - Validate response time accuracy (5 VUs, 1m)

Built-in plans cannot be modified or deleted. They use Docker service hostnames (e.g., `http://echo-api:9001`), which resolve inside the Docker network.

## Running Without Docker

To run the test APIs directly (requires Go 1.22+):

```bash
cd test-apis/echo-api && go run main.go &
cd test-apis/mock-ecommerce && go run main.go &
cd test-apis/slow-api && go run main.go &
cd test-apis/error-api && go run main.go &
```

When running outside Docker, the built-in plans reference Docker hostnames. Create your own plans with `localhost` URLs, or modify the plan URLs before running.

## Verifying Connectivity

```bash
curl localhost:9001/health
curl localhost:9002/products
curl localhost:9003/slow/1
curl localhost:9004/random-error
```
