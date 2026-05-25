# Build stage (Debian-based Go image)
FROM --platform=$BUILDPLATFORM golang:1.24-bullseye AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS="${TARGETOS:-$(go env GOOS)}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build -o llm-proxy ./cmd/cli

# Runtime stage
FROM debian:bullseye-slim
WORKDIR /app
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/llm-proxy /usr/local/bin/llm-proxy

EXPOSE 8080
CMD ["/usr/local/bin/llm-proxy"]
