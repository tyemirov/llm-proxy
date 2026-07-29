#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 2 && "$1" == "--mode" ]] || {
  printf '%s\n' 'error: app deployment requires exactly --mode dry-run|deploy' >&2
  exit 2
}
mode="$2"
[[ "${mode}" == "dry-run" || "${mode}" == "deploy" ]] || {
  printf '%s\n' 'error: --mode must be dry-run or deploy' >&2
  exit 2
}

image_ref="${MPRLAB_LLM_PROXY_IMAGE_REF:-}"
[[ "${image_ref}" =~ ^ghcr\.io/tyemirov/llm-proxy@sha256:[0-9a-f]{64}$ ]] || {
  printf '%s\n' 'error: deployment requires the internally resolved immutable LLM Proxy GHCR digest' >&2
  exit 2
}
[[ -z "${ANSIBLE_CONFIG:-}" ]] || {
  printf '%s\n' 'error: ANSIBLE_CONFIG is owned by the LLM Proxy deployment controller' >&2
  exit 2
}
[[ -z "${ANSIBLE_INVENTORY:-}" ]] || {
  printf '%s\n' 'error: ANSIBLE_INVENTORY is not supported; deployment owns the canonical inventory' >&2
  exit 2
}
[[ -z "${LLM_PROXY_ANSIBLE_INVENTORY:-}" ]] || {
  printf '%s\n' 'error: LLM_PROXY_ANSIBLE_INVENTORY is not supported; deployment owns the canonical inventory' >&2
  exit 2
}
[[ -z "${LLM_PROXY_APP_REPO_PARENTS:-}" ]] || {
  printf '%s\n' 'error: LLM_PROXY_APP_REPO_PARENTS is not supported; deployment owns target discovery' >&2
  exit 2
}
[[ -z "${LLM_PROXY_ANSIBLE_BECOME_PASSWORD_FILE:-}" ]] || {
  printf '%s\n' 'error: LLM_PROXY_ANSIBLE_BECOME_PASSWORD_FILE is not supported; deployment prompts once' >&2
  exit 2
}
command -v uv >/dev/null 2>&1 || {
  printf '%s\n' 'error: uv is required' >&2
  exit 2
}
command -v python3 >/dev/null 2>&1 || {
  printf '%s\n' 'error: python3 is required' >&2
  exit 2
}

repo_root="$(git rev-parse --show-toplevel)"
repo_parent="$(dirname "${repo_root}")"
inventory_path="${repo_root}/.mprlab/deploy/ansible/inventory/hosts.yml"
[[ -f "${inventory_path}" && ! -L "${inventory_path}" ]] || {
  printf 'error: LLM Proxy deployment inventory not found: %s; create it from .mprlab/deploy/ansible/inventory/hosts.example.yml\n' "${inventory_path}" >&2
  exit 2
}

controller_home="${HOME}/.local/share/mprlab-gateway"
ansible_venv="${controller_home}/toolchains/python-3.13-ansible-core-2.19.8-community-docker-5.1.0"
ansible_python="${ansible_venv}/bin/python"
ansible_galaxy="${ansible_venv}/bin/ansible-galaxy"
collections_path="${ansible_venv}/collections"
if [[ ! -x "${ansible_python}" || ! -x "${ansible_galaxy}" ]]; then
  uv python install 3.13
  uv venv --python 3.13 --managed-python "${ansible_venv}"
  uv pip install --python "${ansible_python}" ansible-core==2.19.8
fi

controller_extract_root="$(mktemp -d "${repo_parent}/.llm-proxy-gateway-controller.XXXXXX")"
controller_cleanup_path="${controller_extract_root}"
become_password_cleanup_file=""
cleanup() {
  if [[ -n "${become_password_cleanup_file}" ]]; then
    rm -f -- "${become_password_cleanup_file}"
  fi
  if [[ -n "${controller_cleanup_path}" && "${controller_cleanup_path}" == "${repo_parent}/.llm-proxy-gateway-controller."* ]]; then
    rm -rf -- "${controller_cleanup_path}"
  fi
}
trap cleanup EXIT

controller_root="$(
  "${repo_root}/scripts/prepare_gateway_controller.py" \
    --lock "${repo_root}/.mprlab/deploy/gateway-controller.lock.json" \
    --extract-root "${controller_extract_root}" \
    --archive "${repo_root}/.mprlab/deploy/mprlab-gateway-deploy-bundle-v1.3.0.tar.gz"
)"

mkdir -p "${collections_path}"
if [[ ! -d "${collections_path}/ansible_collections/community/docker" ]]; then
  timeout --foreground -k 1200s -s SIGKILL 1200s \
    "${ansible_galaxy}" collection install --force --no-deps \
      -p "${collections_path}" \
      -r "${controller_root}/deploy/ansible/requirements.yml"
fi

transaction_arguments=(
  --mode "${mode}"
  --repo-root "${repo_root}"
  --inventory "${inventory_path}"
  --image-ref "${image_ref}"
  --controller-root "${controller_root}"
  --ansible-python "${ansible_python}"
  --collections-path "${collections_path}"
  --repo-parents "${repo_parent}"
)
if [[ "${mode}" == "deploy" ]]; then
  become_password_file="$(mktemp "${TMPDIR:-/tmp}/llm-proxy-become.XXXXXX")"
  become_password_cleanup_file="${become_password_file}"
  password="$(python3 -c 'import getpass; print(getpass.getpass("Gateway sudo password: "))')"
  printf '%s\n' "${password}" >"${become_password_file}"
  unset password
  chmod 0600 "${become_password_file}"
  transaction_arguments+=(--become-password-file "${become_password_file}")
fi

"${repo_root}/scripts/run-app-deploy-transaction.sh" "${transaction_arguments[@]}"
