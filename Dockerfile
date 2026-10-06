# Build stage — pure Go, no CGO needed
FROM golang:1.27.1-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/szibis/claude-escalate/internal/config.Version=${VERSION}" \
    -o llm-sentinel ./cmd/llm-sentinel
RUN CGO_ENABLED=0 go build -trimpath -o sentinel-gateway ./cmd/sentinel-gateway && \
    CGO_ENABLED=0 go build -trimpath -o sentinel-tools ./cmd/sentinel-tools

# Runtime stage
FROM alpine:3.24

RUN apk add --no-cache ca-certificates wget git

WORKDIR /app

COPY --from=builder /app/llm-sentinel /app/llm-sentinel
COPY --from=builder /app/sentinel-gateway /app/sentinel-gateway
COPY --from=builder /app/sentinel-tools /app/sentinel-tools

EXPOSE 8077

ENV ESCALATE_BIND=0.0.0.0
ENV ESCALATE_DATA_DIR=/data

ENTRYPOINT ["/app/llm-sentinel"]
CMD ["dashboard", "--port", "8077"]
