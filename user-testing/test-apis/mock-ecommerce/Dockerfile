FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go build -o mock-ecommerce main.go

FROM alpine:3.24
WORKDIR /root/
COPY --from=builder /app/mock-ecommerce .
EXPOSE 9002
CMD ["./mock-ecommerce"]
