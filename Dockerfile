FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ledgerd ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates

COPY --from=builder /ledgerd /ledgerd
# Copy migration files so the container can run them at startup
COPY migrations /migrations

ENV MIGRATIONS_DIR=/migrations

EXPOSE 8080
ENTRYPOINT ["/ledgerd"]
