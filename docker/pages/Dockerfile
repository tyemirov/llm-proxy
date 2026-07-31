# syntax=docker/dockerfile:1

FROM golang:1.26.5-bookworm AS renderer

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /usr/local/bin/llm-proxy ./cmd/cli \
    && /usr/local/bin/llm-proxy \
        --site-source site \
        --site-config-url https://llm-proxy-api.mprlab.com/config-ui.yaml \
        --render-site-output /pages \
    && rm /pages/CNAME \
    && cp docs/openapi.yaml /pages/openapi.yaml

FROM scratch AS pages

COPY --from=renderer /pages/ /
