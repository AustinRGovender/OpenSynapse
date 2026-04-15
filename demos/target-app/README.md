# Demo Target App

A minimal Go HTTP server that serves as a target for OpenSynapse load testing.

## Endpoints

| Method | Path                | Description                          |
| ------ | ------------------- | ------------------------------------ |
| GET    | /health             | Health check, returns `{"status":"ok"}` |
| GET    | /api/products       | Returns a JSON list of products      |
| GET    | /api/products/:id   | Returns a single product by ID       |
| POST   | /api/login          | Accepts username/password, returns token and CSRF token |
| GET    | /api/search?q=...   | Search products by name, description, or category |

## Running locally

```bash
cd demos/target-app
go run main.go
```

The server starts on port 9090 by default. Set the `PORT` environment variable to change it.

## Running with Docker

```bash
cd demos/target-app
docker build -t opensynapse-target-app .
docker run -p 9090:9090 opensynapse-target-app
```

## Login endpoint

The login endpoint accepts any non-empty username and password. It returns a bearer token and a CSRF token suitable for testing automatic correlation in the OpenSynapse crawler.

Request:

```json
{
  "username": "testuser",
  "password": "testpass"
}
```

Response:

```json
{
  "token": "a1b2c3...",
  "csrf_token": "csrf_d4e5f6...",
  "expires_in": 3600
}
```

## Search endpoint

Pass a query string parameter `q` to search products:

```
GET /api/search?q=monitor
```

The search matches against product name, description, and category (case-insensitive).
