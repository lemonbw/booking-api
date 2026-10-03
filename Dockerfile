# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY vendor/ ./vendor/

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=vendor \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/booking-api \
    ./cmd/app

FROM alpine:3.20

RUN addgroup -S app && adduser -S -G app -H -s /sbin/nologin app

COPY --from=builder /out/booking-api /usr/local/bin/booking-api

USER app
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1

ENTRYPOINT ["/usr/local/bin/booking-api"]