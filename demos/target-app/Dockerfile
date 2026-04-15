FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY main.go go.mod ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /target-app .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /target-app /usr/local/bin/target-app
EXPOSE 9090
ENTRYPOINT ["target-app"]
