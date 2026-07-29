#!/usr/bin/env bash
set -euo pipefail

usage() {
  builtin printf '%s\n' 'Usage:
  publish_container_artifacts.sh

Loads container archives prepared by make release, pushes platform images, and
creates the version and latest manifests. It never builds an image.'
}

if [[ $# -gt 0 ]]; then
  case "$1" in --help|-h) usage; exit 0 ;; *) echo "error: no arguments are supported" >&2; exit 1 ;; esac
fi

command -v docker >/dev/null 2>&1 || { echo "error: docker is required" >&2; exit 1; }
command -v gh >/dev/null 2>&1 || { echo "error: gh is required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required" >&2; exit 1; }
docker buildx version >/dev/null 2>&1 || { echo "error: docker buildx is required" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
artifact_dir="$(git rev-parse --git-path mprlab-release)"
[[ "${artifact_dir}" == /* ]] || artifact_dir="${repo_root}/${artifact_dir}"
script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
helper="${script_directory}/release_helper.py"
container_helper="${script_directory}/container_artifact_helper.py"
container_manifest_digest_helper="${script_directory}/resolve_container_manifest_digest.sh"
[[ -f "${container_helper}" ]] || { echo "error: container artifact helper is missing: ${container_helper}" >&2; exit 1; }
[[ -f "${container_manifest_digest_helper}" ]] || { echo "error: container manifest digest helper is missing: ${container_manifest_digest_helper}" >&2; exit 1; }
"${helper}" verify-release-artifact >/dev/null
release_version="$(python3 -c '
import json
import sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["version"])
' "${artifact_dir}/manifest.json")"
publish_timeout="${PUBLISH_CONTAINER_TIMEOUT_SECONDS:-1200}"
inspection_timeout="${CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS:-30}"
[[ "${publish_timeout}" =~ ^[1-9][0-9]*$ ]] || { echo "error: PUBLISH_CONTAINER_TIMEOUT_SECONDS must be a positive integer" >&2; exit 1; }
[[ "${inspection_timeout}" =~ ^[1-9][0-9]*$ ]] || { echo "error: CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS must be a positive integer" >&2; exit 1; }

mapfile -t descriptors < <(find "${artifact_dir}/payloads/containers" -mindepth 2 -maxdepth 2 -name container.json -type f | LC_ALL=C sort)
[[ "${#descriptors[@]}" -gt 0 ]] || { echo "error: no prepared container artifacts found; run make release" >&2; exit 1; }

temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT

inspect_raw_manifest() {
  local image_reference="$1"
  local output_path="$2"
  local error_path="${output_path}.stderr"
  if timeout -k "${inspection_timeout}s" -s SIGKILL "${inspection_timeout}s" \
    docker buildx imagetools inspect --raw "${image_reference}" \
    >"${output_path}" 2>"${error_path}"
  then
    rm -f "${error_path}"
    return 0
  else
    local inspection_status=$?
    if grep -Eqi '(^|[^[:alpha:]])(manifest unknown|not found)([^[:alpha:]]|$)' "${error_path}"; then
      rm -f "${error_path}"
      return 44
    fi
    cat "${error_path}" >&2
    rm -f "${error_path}"
    return "${inspection_status}"
  fi
}

if python3 -c '
import json
import sys
raise SystemExit(0 if any(json.load(open(path, encoding="utf-8"))["image"].startswith("ghcr.io/") for path in sys.argv[1:]) else 1)
' "${descriptors[@]}"
then
  registry_username="$(gh api user --jq .login)"
  registry_token="$(gh auth token)"
  printf '%s' "${registry_token}" | timeout -k 30s -s SIGKILL 30s docker login ghcr.io --username "${registry_username}" --password-stdin
  unset registry_token
fi

for descriptor in "${descriptors[@]}"; do
  metadata="$(python3 -c '
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
if data.get("schema_version") != 1 or data.get("artifact_kind") != "mprlab.container":
    raise SystemExit("invalid container artifact descriptor")
print(data["name"])
print(data["image"])
print(data["version"])
for platform in data["platforms"]:
    print("\t".join([platform["platform"], platform["token"], platform["local_ref"], platform["image_id"], platform["archive"], platform["sha256"]]))
' "${descriptor}")"
  name="$(sed -n '1p' <<<"${metadata}")"
  image="$(sed -n '2p' <<<"${metadata}")"
  version="$(sed -n '3p' <<<"${metadata}")"
  [[ "${version}" == "${release_version}" ]] || { echo "error: ${name} was prepared for ${version}, expected ${release_version}" >&2; exit 1; }
  publish_latest="true"
  if [[ "${version}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+- ]]; then
    publish_latest="false"
  fi
  if [[ "${image}" == */* && "${image%%/*}" == *.* && "${image}" != ghcr.io/* ]]; then
    echo "error: unsupported explicit container registry for ${image}" >&2
    exit 1
  fi
  sources=()
  expected_index_rows="${temporary_directory}/${name}-expected-index.tsv"
  : >"${expected_index_rows}"

  while IFS=$'\t' read -r platform token local_ref expected_image_id archive_relative expected_sha256; do
    [[ -n "${platform}" ]] || continue
    archive="${artifact_dir}/${archive_relative}"
    actual_sha256="$(shasum -a 256 "${archive}" | awk '{print $1}')"
    [[ "${actual_sha256}" == "${expected_sha256}" ]] || { echo "error: container archive hash mismatch: ${archive_relative}" >&2; exit 1; }
    platform_ref="${image}:${version}-${token}"
    platform_raw="${temporary_directory}/${name}-${token}.json"
    if inspect_raw_manifest "${platform_ref}" "${platform_raw}"; then
      if ! python3 "${container_helper}" validate-platform-manifest \
        --raw "${platform_raw}" \
        --image-id "${expected_image_id}"
      then
        echo "error: immutable container platform conflict for ${platform_ref}" >&2
        exit 1
      fi
      echo "==> [publish] Reusing exact ${platform_ref}"
    else
      inspection_status=$?
      if [[ "${inspection_status}" -ne 44 ]]; then
        echo "error: could not determine immutable container state for ${platform_ref} (Docker exit ${inspection_status})" >&2
        exit 1
      fi
      echo "==> [publish] ${platform_ref} is missing; publishing the prepared image"
      timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker load --input "${archive}" >/dev/null
      actual_image_id="$(docker image inspect "${local_ref}" --format '{{.Id}}')"
      [[ "${actual_image_id}" == "${expected_image_id}" ]] || { echo "error: loaded image does not match prepared ${name} ${platform}" >&2; exit 1; }
      docker tag "${local_ref}" "${platform_ref}"
      timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker push "${platform_ref}"
      bash "${container_manifest_digest_helper}" "${platform_ref}" >/dev/null
      inspect_raw_manifest "${platform_ref}" "${platform_raw}" || {
        echo "error: published platform manifest is unreadable for ${platform_ref}" >&2
        exit 1
      }
      python3 "${container_helper}" validate-platform-manifest \
        --raw "${platform_raw}" \
        --image-id "${expected_image_id}"
    fi
    platform_digest="$(bash "${container_manifest_digest_helper}" "${platform_ref}")"
    printf '%s\t%s\n' "${platform}" "${platform_digest}" >>"${expected_index_rows}"
    sources+=("${platform_ref}")
  done < <(tail -n +4 <<<"${metadata}")

  [[ "${#sources[@]}" -gt 0 ]] || { echo "error: ${name} has no prepared platforms" >&2; exit 1; }
  expected_index="${temporary_directory}/${name}-expected-index.json"
  python3 -c '
import json
import pathlib
import sys

entries = []
for row in pathlib.Path(sys.argv[1]).read_text(encoding="utf-8").splitlines():
    platform, digest = row.split("\t")
    operating_system, architecture = platform.split("/")
    entries.append({"os": operating_system, "architecture": architecture, "digest": digest})
pathlib.Path(sys.argv[2]).write_text(json.dumps(entries, sort_keys=True) + "\n", encoding="utf-8")
' "${expected_index_rows}" "${expected_index}"

  version_ref="${image}:${version}"
  version_raw="${temporary_directory}/${name}-version.json"
  if inspect_raw_manifest "${version_ref}" "${version_raw}"; then
    if ! python3 "${container_helper}" validate-index-manifest \
      --raw "${version_raw}" \
      --expected "${expected_index}"
    then
      echo "error: immutable container version conflict for ${version_ref}" >&2
      exit 1
    fi
    echo "==> [publish] Reusing exact ${version_ref}"
  else
    inspection_status=$?
    if [[ "${inspection_status}" -ne 44 ]]; then
      echo "error: could not determine immutable container state for ${version_ref} (Docker exit ${inspection_status})" >&2
      exit 1
    fi
    echo "==> [publish] Creating ${version_ref}"
    timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker buildx imagetools create --tag "${version_ref}" "${sources[@]}"
    bash "${container_manifest_digest_helper}" "${version_ref}" >/dev/null
    inspect_raw_manifest "${version_ref}" "${version_raw}" || {
      echo "error: published version manifest is unreadable for ${version_ref}" >&2
      exit 1
    }
    python3 "${container_helper}" validate-index-manifest \
      --raw "${version_raw}" \
      --expected "${expected_index}"
  fi

  version_digest="$(bash "${container_manifest_digest_helper}" "${image}:${version}")"
  [[ -n "${version_digest}" ]] || { echo "error: published version digest is missing for ${image}:${version}" >&2; exit 1; }
  if [[ "${publish_latest}" == "true" ]]; then
    latest_ref="${image}:latest"
    latest_raw="${temporary_directory}/${name}-latest.json"
    latest_digest=""
    if inspect_raw_manifest "${latest_ref}" "${latest_raw}"; then
      latest_digest="$(bash "${container_manifest_digest_helper}" "${latest_ref}")"
    else
      inspection_status=$?
      if [[ "${inspection_status}" -ne 44 ]]; then
        echo "error: could not determine container state for ${latest_ref} (Docker exit ${inspection_status})" >&2
        exit 1
      fi
    fi
    if [[ "${latest_digest}" == "${version_digest}" ]]; then
      echo "==> [publish] Reusing exact ${latest_ref}"
    else
      echo "==> [publish] Updating ${latest_ref} to ${version_ref}"
      timeout -k "${publish_timeout}s" -s SIGKILL "${publish_timeout}s" docker buildx imagetools create --tag "${latest_ref}" "${sources[@]}"
      latest_digest="$(bash "${container_manifest_digest_helper}" "${latest_ref}")"
    fi
    [[ "${version_digest}" == "${latest_digest}" ]] || { echo "error: published version and latest digests differ for ${image}" >&2; exit 1; }
  else
    echo "==> [publish] Leaving ${image}:latest unchanged for prerelease ${version}"
  fi
  echo "Published ${image}:${version} at ${version_digest}."
done
