# ADR-0009: User Testing Infrastructure

## Status

Accepted

## Date

2026-04-17

## Context

OpenSynapse needs real APIs for user testing and demonstration purposes. Users who install OpenSynapse should be able to immediately run performance tests without needing to set up their own target APIs. Additionally, having built-in test plans gives users concrete examples of how to structure different test types (smoke, load, stress, spike, resilience).

We have 4 purpose-built Go/Gin test APIs:

| API | Port | Purpose |
|-----|------|---------|
| echo-api | 9001 | Basic echo, delay, and status code simulation |
| mock-ecommerce | 9002 | Multi-step e-commerce user journey |
| slow-api | 9003 | Configurable slow responses and flaky behavior |
| error-api | 9004 | Error rate, rate limiting, and degradation simulation |

## Decision

### Test API Hosting

Place test APIs in `user-testing/test-apis/` (outside the main app tree) with a dedicated `docker-compose.yml` that uses the `include` directive to compose with the main OpenSynapse services. This keeps test infrastructure separate from production code while allowing a single `docker compose up` to start everything.

### Built-In Plans

Add a `built_in` column to the `plans` table (migration 0010) and seed 6 plans on startup via `SeedBuiltInPlans()`. Built-in plans are:

1. **Echo API — Smoke Test** (constant-vus, 1 VU, 30s)
2. **E-commerce — Browse & Buy** (ramping-vus, 10→50 VUs, 14m)
3. **Slow API — Stress Test** (ramping-vus, 5→100 VUs, 8m)
4. **Error API — Resilience Test** (constant-vus, 10 VUs, 2m)
5. **Cross-API — Spike Test** (ramping-vus spike, 5→100→5 VUs)
6. **Echo API — Delay Validation** (constant-vus, 5 VUs, 1m)

Built-in plans cannot be modified or deleted (403 response). This follows the same pattern established for built-in fragments (ADR implicit in fragments implementation).

Seeding is idempotent: if any built-in plans exist, seeding is skipped.

## Consequences

- Users get a working demo experience immediately after `docker compose up`
- Built-in plans serve as templates users can reference when creating their own plans
- The `user-testing/` directory is clearly separated from production code
- The `include` directive requires Docker Compose v2.20+ (widely available)
- Built-in plans use Docker service hostnames (e.g., `http://echo-api:9001`) which only resolve inside the Docker network; users running outside Docker will need to adjust URLs
