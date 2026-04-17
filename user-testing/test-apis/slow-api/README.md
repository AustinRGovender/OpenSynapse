# Slow API Simulator

A test API that simulates slow responses and timeouts for performance testing.

## Endpoints

### Health Check
```bash
GET http://localhost:9003/health
```

### Configurable Slow Response
```bash
GET http://localhost:9003/slow/{seconds}
# Example: GET http://localhost:9003/slow/5
# Returns after 5 seconds (0-30 allowed)
```

### Timeout Simulation
```bash
GET http://localhost:9003/timeout
# Delays for 60 seconds (should timeout in most clients)
```

### Flaky Endpoint
```bash
GET http://localhost:9003/flaky
# 50% chance of:
# - Success (200)
# - Failure after random delay (500)
```

## Use Cases

- Test timeout handling
- Validate response time monitoring
- Test retry logic
- Simulate network latency
- Test load balancer behavior with slow backends

## Running

```bash
# Standalone
go run main.go

# Docker
docker build -t slow-api .
docker run -p 9003:9003 slow-api
```
