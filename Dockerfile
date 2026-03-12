FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /bot ./cmd/bot
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /healthcheck ./cmd/healthcheck

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S bot && adduser -S bot -G bot

WORKDIR /app

COPY --from=builder /bot /app/bot
COPY --from=builder /healthcheck /app/healthcheck

RUN chown -R bot:bot /app
USER bot

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/healthcheck"]

ENTRYPOINT ["/app/bot"]
