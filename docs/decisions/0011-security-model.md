# ADR-0011: Security model — request origin checks, authentication, roles, secrets at rest

**Date:** 2026-10-27
**Status:** accepted (amends ADR-0005)
**Authors:** OpenSynapse team

## Context

The control plane can start load tests against arbitrary targets and its playground proxies arbitrary HTTP requests. A review found it sent `Access-Control-Allow-Origin: *`, required no authentication, listened on every interface, skipped the WebSocket origin check (ADR-0005), and stored AI keys and secret environment variables in plaintext. Any web page a user visited while OpenSynapse ran could read their plans and results, start tests, or use the playground as a proxy into their network. The specification calls for bearer-token authentication on every request (install token on desktop, per-user tokens in team mode), argon2id passwords, revocable tokens, admin/editor/viewer roles, and keys kept out of plaintext.

## Decision

**Request origin (all modes).** `internal/httpx` rejects requests whose `Host` is not loopback or listed in `OPENSYNAPSE_ALLOWED_HOSTS` (blocks DNS rebinding) and requests carrying an `Origin` that is neither same-origin nor listed in `OPENSYNAPSE_ALLOWED_ORIGINS` (blocks cross-site requests). CORS headers are sent only for allowed origins. The WebSocket uses the same origin policy (amending ADR-0005). The server binds to `127.0.0.1` unless `OPENSYNAPSE_BIND` says otherwise; container images bind `0.0.0.0` and log a warning in local mode.

**Local mode (default).** Every `/api` request needs the installation token (`Authorization: Bearer osl_…`), generated on first start into `<data dir>/api-token` (or `OPENSYNAPSE_API_TOKEN`). The web UI fetches it from `GET /api/v1/auth/session`, which only same-origin pages can read thanks to the origin checks; CLI users copy it from Settings › About. The single local user has the admin role.

**Team mode (`TEAM_MODE=true`).** Users sign in with email and password (argon2id) and get a 7-day session token (`oss_…`); they can create named personal API tokens (`osk_…`), each revocable on its own. Tokens are stored as SHA-256 hashes. Roles: viewers can read everything and run read-only POSTs (compare, export, compile, AI prompt preview); editors can also change plans, environments, crawls and run tests; admins can also manage users, settings, AI configuration, storage and diagnostics. The first admin comes from `OPENSYNAPSE_ADMIN_EMAIL`/`OPENSYNAPSE_ADMIN_PASSWORD` or a one-time setup screen. The last admin cannot be deleted or demoted. Runs record who started them.

**Public routes.** Health/readiness/version, `/metrics`, the session/login/setup endpoints, the WebSocket (which authenticates with its first message), the run control-state endpoint (authenticated with a per-run token handed only to that run's k6 process), and OAuth callbacks.

**Secrets at rest.** AI provider keys, secret environment variables and webhook URLs are encrypted with AES-256-GCM (`internal/secrets`). The key comes from `OPENSYNAPSE_SECRET_KEY` (the Helm chart supplies it from a Kubernetes Secret) or `<data dir>/secret.key`, created with owner-only permissions. Secret values are redacted (`********`) in every API response and in run snapshots; sending the placeholder back on update keeps the stored value. The k6 subprocess gets a minimal environment so the control plane's own secrets never reach scripts via `__ENV`.

## Alternatives considered

- **OS keychain for the AI key (as specified for desktop).** Not available in containers or Kubernetes and needs cgo or D-Bus on Linux; a key file with owner-only permissions gives equivalent protection against database theft and works everywhere. The desktop shell can move the key into the keychain later without changing the storage format.
- **Cookie sessions.** Would need CSRF tokens; bearer tokens plus strict origin checks avoid that and match the spec's API contract.
- **No token in local mode.** The origin checks alone stop browsers, but the spec's single auth model (every request carries a bearer token) keeps CLI usage and team mode consistent.

## Consequences

- OpenSynapse is no longer usable as an open proxy from other websites.
- Exposing a local-mode install on a network still lets anyone who can reach the port use it; the README tells server operators to enable team mode.
- Deployments reached through a DNS name or IP must list it in `OPENSYNAPSE_ALLOWED_HOSTS` (or `*` behind a trusted proxy).
- Losing `secret.key` makes stored secrets unreadable; it is part of the data directory backup.
