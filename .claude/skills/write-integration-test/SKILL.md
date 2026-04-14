---
name: write-integration-test
description: How to write an integration test for the Go control plane against a seeded test database
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
---

# Writing Integration Tests

## File location

Test files live alongside the code they test, following Go conventions:
```
apps/control-plane/internal/handlers/plans_test.go
apps/control-plane/internal/handlers/runs_test.go
```

## Test database setup

Use the test helper to get a clean, seeded database for each test:

```go
func TestPlanCreate(t *testing.T) {
    db := testutil.NewTestDB(t)  // creates an in-memory SQLite, runs migrations
    defer db.Close()

    srv := testutil.NewTestServer(t, db)  // spins up the HTTP server

    // ... test code ...
}
```

## Patterns

- **Table-driven tests** for multiple input/output scenarios:
  ```go
  tests := []struct {
      name       string
      input      CreatePlanRequest
      wantStatus int
      wantErr    string
  }{...}
  ```

- **HTTP client helper** for making requests:
  ```go
  resp := srv.Post("/api/v1/plans", body)
  assert.Equal(t, http.StatusCreated, resp.StatusCode)
  ```

- **Assert structured errors** matching the format in `docs/04-data-model-and-api.md` section 4:
  ```go
  var errResp ErrorResponse
  json.Decode(resp.Body, &errResp)
  assert.Equal(t, "PLAN_NOT_FOUND", errResp.Error.Code)
  ```

## Rules

- Clean up after every test. Do not share state between tests.
- Test both success and error paths.
- Use real database operations, not mocks. The test database is SQLite in-memory.
- Prefer `require` for preconditions, `assert` for the thing being tested.
- Set a timeout: `t.Parallel()` where safe, `-timeout=60s` on the test command.

## Reference

- Error response format: `docs/04-data-model-and-api.md` section 4
- API contracts: `docs/04-data-model-and-api.md` section 2
