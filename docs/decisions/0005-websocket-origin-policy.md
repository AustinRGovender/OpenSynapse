# ADR-0005: WebSocket origin policy and `InsecureSkipVerify` scope

**Date:** 2026-04-16
**Status:** accepted
**Authors:** OpenSynapse team

## Context

`apps/control-plane/internal/wsserver/wsserver.go:41` accepts every WebSocket upgrade with:

```go
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    InsecureSkipVerify: true, // Allow any origin in dev
})
```

`InsecureSkipVerify: true` in `nhooyr.io/websocket` disables the library's built-in `Origin` header check. Without that check the server is vulnerable to Cross-Site WebSocket Hijacking (CSWSH): a malicious page the user happens to have open in another browser tab can open a WS to `ws://localhost:8090/api/v1/ws`, subscribe to `runs.<id>.metrics`, and read live metric streams.

The comment claims "dev" but the setting is unconditional and ships in all three deployment modes (desktop, Docker, Kubernetes).

The WebSocket endpoint currently carries no authentication (there is no user model in v1) and no sensitive payloads beyond live run metrics and event names. It does not accept state-changing commands — all mutation goes through REST, which is subject to CORS.

## Decision

Accept `InsecureSkipVerify: true` for v1 with the following scope and caveats documented here and in follow-up work:

1. **Desktop mode (Tauri).** The SPA is served from a Tauri custom scheme (`tauri://localhost` or similar) and the control plane binds to `127.0.0.1`. A cross-origin browser attack from an unrelated tab targeting `ws://127.0.0.1:<port>` is the realistic attack vector. The exposure is read-only metric data for runs the attacker does not know the IDs of; they must guess UUIDs. Accepted risk for v1.
2. **Docker Compose.** The nginx front-end proxies `/api/v1/ws`; the control plane is not directly exposed to the host by default. Same-origin policy applies at the nginx layer. Accepted risk for v1.
3. **Kubernetes.** Behind an ingress that terminates TLS and enforces its own origin policy at the proxy layer. Accepted risk for v1.

Follow-up work (tracked outside this ADR):

- Replace `InsecureSkipVerify: true` with `OriginPatterns: []string{…}` populated from a runtime config value (`OPENSYNAPSE_ALLOWED_ORIGINS`), defaulting to `http://localhost:*` and `tauri://*`.
- When auth is introduced (Phase 2+), require a short-lived bearer token in the subprotocol or first message, which makes origin enforcement less load-bearing.

## Alternatives considered

- **Ship proper origin enforcement today.** The right long-term answer but requires wiring configuration through `main.go`, docker-compose, the Helm chart, and the Tauri sidecar. Disproportionate work for v1 threat model — deferred to follow-up.
- **Hard-code origin allowlist.** Rejected because the allowed origins differ per deployment mode (Tauri vs. nginx vs. ingress hostname) and hard-coding would break cluster installs on arbitrary domains.
- **Remove the WebSocket endpoint and poll over REST.** Rejected — live metrics at 1 Hz over polling is wasteful and contradicts the live-dashboard UX from the PRD.

## Consequences

- **Known security gap:** any browser page the user visits can read live metric streams from a running OpenSynapse instance if it knows (or guesses) a run ID. Run IDs are UUIDs, which limits but does not eliminate risk.
- The comment in `wsserver.go:41` is misleading ("dev") — it should be updated to point at this ADR.
- Follow-up work to add origin enforcement is pre-scoped and well-understood.
- This ADR must be revisited before any deployment mode exposes the control plane to untrusted networks or introduces write-capable WebSocket messages.
