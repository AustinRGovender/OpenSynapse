---
name: add-api-endpoint
description: How to add a new REST endpoint to the Go control plane and regenerate the TypeScript client
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
---

# Adding a REST API Endpoint

Follow these steps in order when adding, modifying, or removing a REST API endpoint.

## Step 1: Update the OpenAPI spec

Edit `apps/control-plane/api/openapi.yaml` to add or modify the endpoint definition. Include request/response schemas, parameters, and error responses.

## Step 2: Add the handler

Create or update the handler function in `apps/control-plane/internal/handlers/`. Follow existing handler patterns:
- Accept `http.ResponseWriter` and `*http.Request`
- Parse and validate input
- Call the appropriate service layer function
- Return JSON responses with correct status codes
- Return structured errors matching `docs/04-data-model-and-api.md` section 4

## Step 3: Register the route

Add the route in `apps/control-plane/internal/router/router.go`. Group routes by resource. Apply middleware (auth, logging) consistently.

## Step 4: Write tests

- Unit test for the handler with mocked dependencies
- Integration test against a seeded test database (see `write-integration-test` skill)
- Test both success and error paths
- Use table-driven tests for multiple scenarios

## Step 5: Regenerate the TypeScript client

Run: `pnpm --filter api-client generate`

Verify the generated file compiles: `pnpm --filter api-client build`

## Step 6: Update consumers

If the web app or any other package uses the endpoint, update them to match the new contract. Fix any type errors from the regenerated client.

## Step 7: Verify at runtime

Start the dev server and confirm the endpoint appears in `/openapi.json`. Test with curl or the endpoint playground.

## Reference

- API contract: `docs/04-data-model-and-api.md`
- Error format: `docs/04-data-model-and-api.md` section 4
