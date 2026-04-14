---
name: verify-k6-script
description: How to validate a generated k6 script after plan-to-script transformer changes
tools:
  - Bash
  - Read
  - Write
---

# Verifying a Generated k6 Script

Run this procedure after any change to `apps/control-plane/internal/compiler/`.

## Steps

1. **Generate a test script** from a fixture plan:
   ```bash
   cd apps/control-plane
   go run ./cmd/compile --plan fixtures/test-plan.json --output /tmp/test-script.js
   ```

2. **Validate with k6 inspect**:
   ```bash
   k6 inspect /tmp/test-script.js
   ```
   This parses the script and reports any syntax errors or invalid k6 constructs. Exit code 0 means valid.

3. **Optionally run a single iteration** against a safe target:
   ```bash
   k6 run /tmp/test-script.js --iterations=1 --vus=1 --duration=10s \
     --env BASE_URL=https://test.k6.io
   ```
   This confirms the script actually executes, not just parses.

4. **Check the output** for:
   - No JavaScript syntax errors
   - All imports resolve (k6 built-ins, xk6-kv)
   - The externally-controlled executor is configured correctly
   - The token bucket and should-stop watchdog are present in the generated code

## If validation fails

The compiler change is broken. Fix it before committing. Common issues:
- Missing semicolons or unmatched braces in template strings
- Incorrect `import` paths for k6 modules
- Invalid executor configuration in the `options` export

## Reference

- Live control architecture: `docs/02-architecture.md` section 3
- Plan-to-script transformer: `docs/02-architecture.md` section 9
