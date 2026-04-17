# Echo API

Simple HTTP echo service for testing.

## Endpoints

- `GET /health` - Health check
- `POST /echo` - Echo request body back
- `GET /delay/:ms` - Delayed response (0-10000ms)
- `GET /status/:code` - Return specific HTTP status code

## Run

```bash
go run main.go
```

Access at: http://localhost:9001
