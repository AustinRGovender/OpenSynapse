# Error Generator API

A test API that generates various error conditions for testing error handling and resilience.

## Endpoints

### Health Check
```bash
GET http://localhost:9004/health
```

### Random Errors
```bash
GET http://localhost:9004/random-error
# Returns a random error status code:
# 400, 401, 403, 404, 429, 500, 502, 503, 504
```

### Rate Limiting
```bash
GET http://localhost:9004/rate-limit
# Allows 100 requests per 10 seconds
# Returns 429 after limit exceeded
# Resets every 10 seconds
```

### Progressive Degradation
```bash
GET http://localhost:9004/degradation
# Behavior degrades with request count:
# - Response time increases (up to 5 seconds)
# - Error rate increases (30% after 50 requests)
# - Resets every 10 seconds
```

## Use Cases

- Test error handling and retry logic
- Validate rate limiting behavior
- Test circuit breaker patterns
- Train ML models on anomaly detection
- Simulate service degradation
- Test monitoring and alerting

## Running

```bash
# Standalone
go run main.go

# Docker
docker build -t error-api .
docker run -p 9004:9004 error-api
```
