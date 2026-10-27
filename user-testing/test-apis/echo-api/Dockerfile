FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go build -o echo-api .

FROM alpine:3.24
WORKDIR /root/
COPY --from=builder /app/echo-api .
EXPOSE 9001
CMD ["./echo-api"]
