#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf '%s\n' 'Usage: scripts/run-app-deploy-transaction.sh --mode dry-run|deploy --repo-root <path> --inventory <path> --image-ref <immutable-ref> --controller-root <path> --ansible-python <path> --collections-path <path> --repo-parents <path> [--become-password-file <path>]'
}

mode=""
repo_root=""
inventory_path=""
image_ref=""
controller_root=""
ansible_python=""
collections_path=""
repo_parents=""
become_password_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --mode requires a value' >&2; exit 2; }
      mode="$2"
      shift 2
      ;;
    --repo-root)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --repo-root requires a value' >&2; exit 2; }
      repo_root="$2"
      shift 2
      ;;
    --inventory)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --inventory requires a value' >&2; exit 2; }
      inventory_path="$2"
      shift 2
      ;;
    --image-ref)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --image-ref requires a value' >&2; exit 2; }
      image_ref="$2"
      shift 2
      ;;
    --controller-root)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --controller-root requires a value' >&2; exit 2; }
      controller_root="$2"
      shift 2
      ;;
    --ansible-python)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --ansible-python requires a value' >&2; exit 2; }
      ansible_python="$2"
      shift 2
      ;;
    --collections-path)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --collections-path requires a value' >&2; exit 2; }
      collections_path="$2"
      shift 2
      ;;
    --repo-parents)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --repo-parents requires a value' >&2; exit 2; }
      repo_parents="$2"
      shift 2
      ;;
    --become-password-file)
      [[ $# -ge 2 ]] || { printf '%s\n' 'error: --become-password-file requires a value' >&2; exit 2; }
      become_password_file="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'error: unknown app transaction argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[[ "${mode}" == "dry-run" || "${mode}" == "deploy" ]] || {
  printf '%s\n' 'error: --mode must be dry-run or deploy' >&2
  exit 2
}
[[ -d "${repo_root}" && ! -L "${repo_root}" ]] || {
  printf 'error: --repo-root must name a directory: %s\n' "${repo_root}" >&2
  exit 2
}
[[ -f "${inventory_path}" && ! -L "${inventory_path}" ]] || {
  printf 'error: --inventory must name a regular file: %s\n' "${inventory_path}" >&2
  exit 2
}
[[ "${image_ref}" =~ ^ghcr\.io/tyemirov/llm-proxy@sha256:[0-9a-f]{64}$ ]] || {
  printf '%s\n' 'error: --image-ref must be the immutable LLM Proxy GHCR digest' >&2
  exit 2
}
[[ -d "${controller_root}" && ! -L "${controller_root}" ]] || {
  printf 'error: --controller-root must name a directory: %s\n' "${controller_root}" >&2
  exit 2
}
[[ -x "${controller_root}/bin/mprlab-gateway-deploy-target" ]] || {
  printf '%s\n' 'error: verified gateway target controller is not executable' >&2
  exit 2
}
[[ -x "${ansible_python}" ]] || {
  printf 'error: --ansible-python must name an executable: %s\n' "${ansible_python}" >&2
  exit 2
}
[[ -d "${collections_path}" && ! -L "${collections_path}" ]] || {
  printf 'error: --collections-path must name a directory: %s\n' "${collections_path}" >&2
  exit 2
}
[[ -n "${repo_parents}" && "${repo_parents}" != *":"* ]] || {
  printf '%s\n' 'error: --repo-parents must name exactly one repository parent' >&2
  exit 2
}
if [[ "${mode}" == "dry-run" ]]; then
  [[ -z "${become_password_file}" ]] || {
    printf '%s\n' 'error: dry-run forbids --become-password-file' >&2
    exit 2
  }
else
  [[ -f "${become_password_file}" && ! -L "${become_password_file}" && -r "${become_password_file}" ]] || {
    printf '%s\n' 'error: deploy requires a readable regular --become-password-file' >&2
    exit 2
  }
  "${ansible_python}" -c \
    'import os, stat, sys; mode = stat.S_IMODE(os.lstat(sys.argv[1]).st_mode); mode == 0o600 or sys.exit(f"become password file mode must be 0600, got {mode:04o}")' \
    "${become_password_file}"
fi

app_ansible_config="${repo_root}/.mprlab/deploy/ansible/ansible.cfg"
app_local_playbook="${repo_root}/.mprlab/deploy/ansible/playbooks/preflight-local.yml"
app_dispatch_playbook="${repo_root}/.mprlab/deploy/ansible/playbooks/dispatch.yml"
for required_file in "${app_ansible_config}" "${app_local_playbook}" "${app_dispatch_playbook}"; do
  [[ -f "${required_file}" && ! -L "${required_file}" ]] || {
    printf 'error: app deployment input must be a regular file: %s\n' "${required_file}" >&2
    exit 2
  }
done

app_ansible_local_temp="${repo_root}/.cache/ansible-local"
mkdir -p "${app_ansible_local_temp}"
export MPRLAB_LLM_PROXY_IMAGE_REF="${image_ref}"
export LLM_PROXY_DEPLOY_REPO_ROOT="${repo_root}"

run_app_playbook() {
  ANSIBLE_CONFIG="${app_ansible_config}" \
  ANSIBLE_COLLECTIONS_PATH="${collections_path}" \
  ANSIBLE_LOCAL_TEMP="${app_ansible_local_temp}" \
  timeout --foreground -k 1200s -s SIGKILL 1200s \
    "${ansible_python}" -m ansible.cli.playbook "$@"
}

run_app_phase() {
  local phase="$1"
  local gateway_converged="$2"
  LLM_PROXY_DEPLOY_PHASE="${phase}" \
  MPRLAB_GATEWAY_TARGET_CONVERGED="${gateway_converged}" \
    run_app_playbook \
      --become-password-file "${become_password_file}" \
      --inventory "${inventory_path}" \
      "${app_dispatch_playbook}"
}

run_app_playbook \
  --inventory localhost, \
  "${app_local_playbook}"

controller_arguments=(
  --target llm-proxy
  --inventory "${inventory_path}"
  --repo-parents "${repo_parents}"
  --image-ref "${image_ref}"
  --ansible-python "${ansible_python}"
  --collections-path "${collections_path}"
)
if [[ "${mode}" == "dry-run" ]]; then
  "${controller_root}/bin/mprlab-gateway-deploy-target" \
    --mode dry-run \
    "${controller_arguments[@]}"
  printf '%s\n' 'LLM Proxy deployment preflight passed; production hosts were not contacted and production state was not changed.'
  exit 0
fi

run_app_phase preflight 0
"${controller_root}/bin/mprlab-gateway-deploy-target" \
  --mode deploy \
  "${controller_arguments[@]}" \
  --become-password-file "${become_password_file}"
run_app_phase deploy 1
run_app_phase verify 1

printf '%s\n' 'LLM Proxy backend and shared gateway target converged and verified.'
