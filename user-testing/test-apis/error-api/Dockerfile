FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go build -o error-api main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/error-api .
EXPOSE 9004
CMD ["./error-api"]
