---
name: update-openapi-client
description: How to regenerate the TypeScript API client after REST API changes
tools:
  - Bash
  - Read
  - Glob
---

# Regenerating the TypeScript API Client

Run this procedure whenever a handler, route, or OpenAPI definition file is modified.

## Steps

1. Ensure the OpenAPI spec is up to date:
   ```bash
   # Check that the spec file reflects your handler changes
   cat apps/control-plane/api/openapi.yaml
   ```

2. Regenerate the client:
   ```bash
   pnpm --filter api-client generate
   ```

3. Verify compilation:
   ```bash
   pnpm --filter api-client build
   ```

4. Check for breakages in consumers:
   ```bash
   pnpm --filter web build
   ```

5. Fix any type errors in the web app that result from the updated client.

6. **Do not commit a partial update.** The handler change, OpenAPI spec update, client regeneration, and consumer fixes must all be in the same commit.

## Troubleshooting

- If `generate` fails, check that the OpenAPI spec is valid YAML and conforms to OpenAPI 3.0+.
- If the web app has type errors after regeneration, the handler's response shape likely changed. Update the consuming code to match.

## Reference

- OpenAPI spec location: `apps/control-plane/api/openapi.yaml`
- Generated client: `packages/api-client/src/generated.ts`
- The OpenAPI drift guard hook (`scripts/dev/hooks/check-openapi-drift.sh`) will warn if you forget this step.
