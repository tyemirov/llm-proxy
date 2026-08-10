# syntax=docker/dockerfile:1

FROM golang:1.26.5-bookworm AS backend-builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /usr/local/bin/llm-proxy ./cmd/cli

FROM node:22-bookworm-slim AS renderer

WORKDIR /workspace

COPY --from=backend-builder /usr/local/bin/llm-proxy /usr/local/bin/llm-proxy
COPY . .
RUN set -eu; \
    capabilities_url="http://127.0.0.1:8080/api/public/capabilities"; \
    /usr/local/bin/llm-proxy --config configs/config.yml --public-capabilities-only >/tmp/capability-api.log 2>&1 & \
    capability_api_pid="$!"; \
    trap 'kill "${capability_api_pid}" 2>/dev/null || true; wait "${capability_api_pid}" 2>/dev/null || true' EXIT; \
    capability_api_ready=false; \
    attempt=0; \
    while [ "${attempt}" -lt 100 ]; do \
      if CAPABILITIES_URL="${capabilities_url}" node --input-type=module --eval 'const response = await fetch(process.env.CAPABILITIES_URL); process.exit(response.ok ? 0 : 1);' >/dev/null 2>&1; then capability_api_ready=true; break; fi; \
      kill -0 "${capability_api_pid}" 2>/dev/null || { cat /tmp/capability-api.log >&2; exit 1; }; \
      attempt=$((attempt + 1)); \
      sleep 0.1; \
    done; \
    [ "${capability_api_ready}" = true ]; \
    node scripts/render_public_site.mjs \
      --source site \
      --output /pages \
      --config-url https://llm-proxy-api.mprlab.com/config-ui.yaml \
      --capabilities-url "${capabilities_url}"; \
    kill "${capability_api_pid}"; \
    wait "${capability_api_pid}" 2>/dev/null || true; \
    trap - EXIT; \
    rm /pages/CNAME; \
    cp docs/openapi.yaml /pages/openapi.yaml

FROM scratch AS pages

COPY --from=renderer /pages/ /
