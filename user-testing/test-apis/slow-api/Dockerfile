FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go build -o slow-api main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/slow-api .
EXPOSE 9003
CMD ["./slow-api"]
