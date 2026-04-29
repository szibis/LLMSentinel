# Build stage — pure Go, no CGO needed
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/szibis/claude-escalate/internal/config.Version=${VERSION}" \
    -o claude-escalate ./cmd/claude-escalate

# Runtime stage
FROM alpine:3.23

RUN apk add --no-cache ca-certificates wget git

WORKDIR /app

COPY --from=builder /app/claude-escalate /app/claude-escalate

EXPOSE 8077

ENV ESCALATE_BIND=0.0.0.0
ENV ESCALATE_DATA_DIR=/data

ENTRYPOINT ["/app/claude-escalate"]
CMD ["dashboard", "--port", "8077"]
