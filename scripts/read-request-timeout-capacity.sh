#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf '%s\n' 'usage: read-request-timeout-capacity.sh <config-path>' >&2
  exit 2
fi

config_path="$1"
if [[ ! -f "${config_path}" || -L "${config_path}" ]]; then
  printf 'error: server config must be a regular, non-symlink file: %s\n' "${config_path}" >&2
  exit 1
fi

maximum="$(
  awk '
    {
      line = $0
      sub(/[[:space:]]*#.*/, "", line)
      sub(/[[:space:]]+$/, "", line)

      if (line ~ /^server:[[:space:]]*$/) {
        server_count++
        in_server = 1
        next
      }
      if (line ~ /^[^[:space:]][^:]*:[[:space:]]*/) {
        in_server = 0
      }
      if (in_server && line ~ /^[[:space:]]+max_request_timeout_seconds:[[:space:]]*/) {
        maximum_count++
        sub(/^[[:space:]]+max_request_timeout_seconds:[[:space:]]*/, "", line)
        maximum = line
      }
    }

    END {
      if (server_count != 1) {
        printf "error: expected exactly one top-level server mapping, found %d\n", server_count > "/dev/stderr"
        exit 1
      }
      if (maximum_count != 1) {
        printf "error: expected exactly one server.max_request_timeout_seconds, found %d\n", maximum_count > "/dev/stderr"
        exit 1
      }
      print maximum
    }
  ' "${config_path}"
)"

if [[ ! "${maximum}" =~ ^[1-9][0-9]*$ ]]; then
  printf 'error: server.max_request_timeout_seconds must be a positive whole number (got: %s)\n' "${maximum}" >&2
  exit 1
fi

printf '%s\n' "${maximum}"
