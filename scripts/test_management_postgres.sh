#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
postgres_dsn="${LLM_PROXY_TEST_POSTGRES_DSN:-}"
container_name=""

cleanup() {
  if [[ -n "${container_name}" ]]; then
    docker stop "${container_name}" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

if [[ -z "${postgres_dsn}" ]]; then
  command -v docker >/dev/null 2>&1 || {
    echo "Docker is required for the disposable PostgreSQL management migration test." >&2
    exit 1
  }
  container_name="llm-proxy-postgres-test-${PPID}-$$"
  docker run \
    --detach \
    --rm \
    --name "${container_name}" \
    --env POSTGRES_HOST_AUTH_METHOD=trust \
    --env POSTGRES_DB=llm_proxy_test \
    --publish 127.0.0.1::5432 \
    postgres:17-alpine >/dev/null
  postgres_port="$(docker port "${container_name}" 5432/tcp | awk -F: 'NR == 1 { print $NF }')"
  [[ "${postgres_port}" =~ ^[0-9]+$ ]] || {
    echo "Disposable PostgreSQL port could not be resolved." >&2
    exit 1
  }
  postgres_ready=false
  for _ in {1..300}; do
    if docker exec "${container_name}" pg_isready --username postgres --dbname llm_proxy_test >/dev/null 2>&1; then
      postgres_ready=true
      break
    fi
    sleep 0.1
  done
  if [[ "${postgres_ready}" != "true" ]]; then
    docker logs "${container_name}" >&2
    echo "Disposable PostgreSQL did not become ready within 30 seconds." >&2
    exit 1
  fi
  postgres_dsn="host=127.0.0.1 port=${postgres_port} user=postgres dbname=llm_proxy_test sslmode=disable"
fi

cd "${repository_root}"
LLM_PROXY_TEST_POSTGRES_DSN="${postgres_dsn}" go test ./internal/proxy \
  -run '^(TestManagedTenantPostgresOwnershipMigrationPreservesRebindsAndRollsBack|TestManagedUsageOutcomePostgresMigrationBackfillsCanonicalCodesAndIndex)$' \
  -count=1
