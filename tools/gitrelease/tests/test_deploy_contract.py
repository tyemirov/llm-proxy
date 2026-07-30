from __future__ import annotations

import http.server
import json
import os
import shutil
import subprocess
import tempfile
import threading
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
ANSIBLE_ROOT = REPOSITORY_ROOT / ".mprlab/deploy/ansible"
DEPLOY_PLAYBOOK = ANSIBLE_ROOT / "playbooks/deploy.yml"
ANSIBLE_CORE_SPEC = "ansible-core==2.19.8"


class DeployContractTests(unittest.TestCase):
    def test_make_deploy_invokes_only_repository_owned_ansible(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            capture = fixture_root / "capture"
            tool_directory = fixture_root / "bin"
            tool_directory.mkdir()
            self.write_executable(
                tool_directory / "uv",
                "#!/usr/bin/env bash\n"
                "set -euo pipefail\n"
                "[[ \"${MPRLAB_DEPLOY_MODE}\" == deploy ]]\n"
                "[[ \"${ANSIBLE_CONFIG}\" == \"${PWD}/.mprlab/deploy/ansible/ansible.cfg\" ]]\n"
                "[[ \"$*\" == *\"tool run --from ansible-core==2.19.8 ansible-playbook\"* ]]\n"
                "[[ \"$*\" == *\"--ask-become-pass\"* ]]\n"
                "[[ \"$*\" == *\"${PWD}/.mprlab/deploy/ansible/playbooks/deploy.yml\"* ]]\n"
                "printf '%s\\t%s\\n' \"${PWD}\" \"$*\" >>\"${DEPLOY_CAPTURE}\"\n",
            )
            environment = os.environ.copy()
            environment.update(
                {
                    "DEPLOY_CAPTURE": str(capture),
                    "PATH": f"{tool_directory}{os.pathsep}{environment['PATH']}",
                }
            )

            first = self.run_make(environment, "deploy")
            second = self.run_make(
                environment,
                "deploy",
                "DEPLOY_ARGS=forbidden-selector",
                "GATEWAY_DIR=/unrelated/checkout",
            )

            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertEqual(second.returncode, 0, second.stderr)
            captured = capture.read_text(encoding="utf-8").splitlines()
            self.assertEqual(len(captured), 2)
            self.assertTrue(
                all(line.startswith(f"{REPOSITORY_ROOT}\t") for line in captured)
            )
            self.assertTrue(all("mprlab-deploy" not in line for line in captured))
            self.assertEqual(captured[0], captured[1])

    def test_make_deploy_dry_run_never_requests_a_password(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            capture = fixture_root / "capture"
            tool_directory = fixture_root / "bin"
            tool_directory.mkdir()
            self.write_executable(
                tool_directory / "uv",
                "#!/usr/bin/env bash\n"
                "set -euo pipefail\n"
                "[[ \"${MPRLAB_DEPLOY_MODE}\" == dry-run ]]\n"
                "[[ \"$*\" != *\"--ask-become-pass\"* ]]\n"
                "printf '%s\\n' \"$*\" >\"${DEPLOY_CAPTURE}\"\n",
            )
            environment = os.environ.copy()
            environment.update(
                {
                    "DEPLOY_CAPTURE": str(capture),
                    "PATH": f"{tool_directory}{os.pathsep}{environment['PATH']}",
                }
            )

            result = self.run_make(environment, "deploy-dry-run")

            self.assertEqual(result.returncode, 0, result.stderr)
            invocation = capture.read_text(encoding="utf-8")
            self.assertIn(str(DEPLOY_PLAYBOOK), invocation)
            self.assertNotIn("mprlab-deploy", invocation)

    def test_repository_tracks_the_complete_readable_deployment_source(self) -> None:
        deploy_paths = set(
            subprocess.run(
                [
                    "git",
                    "ls-files",
                    "--cached",
                    "--others",
                    "--exclude-standard",
                    ".mprlab/deploy",
                ],
                cwd=REPOSITORY_ROOT,
                check=True,
                capture_output=True,
                text=True,
            ).stdout.splitlines()
        )
        self.assertEqual(
            deploy_paths,
            {
                ".mprlab/deploy/ansible/ansible.cfg",
                ".mprlab/deploy/ansible/inventory/hosts.example.yml",
                ".mprlab/deploy/ansible/playbooks/deploy.yml",
                ".mprlab/deploy/ansible/tasks/local-preflight.yml",
                ".mprlab/deploy/ansible/tasks/reconcile-caddy.yml",
                ".mprlab/deploy/ansible/tasks/reconcile-service.yml",
                ".mprlab/deploy/ansible/tasks/reconcile-tauth.yml",
                ".mprlab/deploy/ansible/tasks/remote-preflight.yml",
                ".mprlab/deploy/ansible/tasks/verify.yml",
                ".mprlab/deploy/ansible/templates/caddy-route.caddy.j2",
                ".mprlab/deploy/ansible/templates/docker-compose.yml.j2",
                ".mprlab/deploy/resources.yml",
            },
        )
        deployment_source = "\n".join(
            path.read_text(encoding="utf-8")
            for path in sorted(
                [
                    REPOSITORY_ROOT / "Makefile",
                    REPOSITORY_ROOT / ".mprlab/deploy/resources.yml",
                    *ANSIBLE_ROOT.rglob("*"),
                ]
            )
            if path.is_file() and path.name != "hosts.yml"
        )
        for forbidden_dependency in (
            "mprlab-deploy",
            "gateway-controller",
            "prepare_gateway_controller",
            "mprlab-gateway-deploy",
        ):
            self.assertNotIn(forbidden_dependency, deployment_source)

    def test_dry_run_admits_only_the_current_repository(self) -> None:
        uv_executable = shutil.which("uv")
        self.assertIsNotNone(uv_executable)
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            repository = fixture_root / "llm-proxy"
            unrelated = fixture_root / "unrelated-invalid-repository"
            bare_remote = fixture_root / "llm-proxy.git"
            shutil.copytree(REPOSITORY_ROOT / ".mprlab/deploy", repository / ".mprlab/deploy")
            (repository / "configs").mkdir(parents=True)
            (repository / "configs/config.yml").write_text(
                "server:\n  max_request_timeout_seconds: 3600\n",
                encoding="utf-8",
            )
            private_environment = repository / ".mprlab/deploy/.env"
            private_environment.write_text("PLACEHOLDER=value\n", encoding="utf-8")
            private_environment.chmod(0o600)
            inventory = repository / ".mprlab/deploy/ansible/inventory/hosts.yml"
            shutil.copy(
                repository / ".mprlab/deploy/ansible/inventory/hosts.example.yml",
                inventory,
            )
            inventory.chmod(0o600)
            release_scripts = repository / "tools/gitrelease/scripts"
            release_scripts.mkdir(parents=True)
            self.write_executable(
                release_scripts / "release_helper.py",
                "#!/usr/bin/env python3\n"
                "import json\n"
                "import subprocess\n"
                "import sys\n"
                "if sys.argv[1] != 'local-release-state':\n"
                "    raise SystemExit(2)\n"
                "commit = subprocess.run(\n"
                "    ['git', 'rev-parse', 'HEAD'],\n"
                "    check=True,\n"
                "    capture_output=True,\n"
                "    text=True,\n"
                ").stdout.strip()\n"
                "print(json.dumps({\n"
                "    'ok': True,\n"
                "    'state': 'sealed',\n"
                "    'version': 'v1.0.0',\n"
                "    'release_commit': commit,\n"
                "}))\n",
            )
            self.write_executable(
                release_scripts / "resolve_container_manifest_digest.sh",
                "#!/usr/bin/env bash\n"
                "set -euo pipefail\n"
                "[[ $# -eq 1 ]]\n"
                "printf 'sha256:%064d\\n' 0\n",
            )
            (repository / ".gitignore").write_text(
                ".mprlab/deploy/.env\n"
                ".mprlab/deploy/ansible/inventory/hosts.yml\n",
                encoding="utf-8",
            )
            self.run_git(repository, "init", "--initial-branch=master")
            self.run_git(repository, "config", "user.email", "fixture@example.invalid")
            self.run_git(repository, "config", "user.name", "Fixture")
            self.run_git(repository, "add", ".")
            self.run_git(repository, "commit", "-m", "fixture release")
            self.run_git(repository, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
            subprocess.run(
                ["git", "init", "--bare", "--initial-branch=master", str(bare_remote)],
                check=True,
                capture_output=True,
                text=True,
            )
            self.run_git(repository, "remote", "add", "origin", str(bare_remote))
            self.run_git(repository, "push", "--set-upstream", "origin", "master")
            self.run_git(repository, "push", "origin", "v1.0.0")

            (unrelated / ".mprlab/deploy").mkdir(parents=True)
            (unrelated / ".mprlab/deploy/resources.yml").write_text(
                "not: [valid\n",
                encoding="utf-8",
            )
            environment = os.environ.copy()
            for inherited_name in (
                "ANSIBLE_CONFIG",
                "ANSIBLE_INVENTORY",
                "MPRLAB_APP_REPO_PARENT",
                "MPRLAB_APP_REPO_PARENTS",
            ):
                environment.pop(inherited_name, None)
            environment.update(
                {
                    "MPRLAB_DEPLOY_MODE": "dry-run",
                    "LLM_PROXY_DEPLOY_PUBLISH_REMOTE": "origin",
                    "LLM_PROXY_DEPLOY_PUBLISH_BRANCH": "master",
                    "LLM_PROXY_DEPLOY_PAGES_BRANCH": "gh-pages",
                    "LLM_PROXY_DEPLOY_PAGES_URL": "https://llm-proxy.mprlab.com/",
                    "ANSIBLE_CONFIG": str(
                        repository / ".mprlab/deploy/ansible/ansible.cfg"
                    ),
                    "ANSIBLE_LOCAL_TEMP": str(fixture_root / "ansible-local"),
                }
            )

            result = subprocess.run(
                [
                    str(uv_executable),
                    "tool",
                    "run",
                    "--from",
                    ANSIBLE_CORE_SPEC,
                    "ansible-playbook",
                    "--inventory",
                    str(inventory),
                    str(repository / ".mprlab/deploy/ansible/playbooks/deploy.yml"),
                ],
                cwd=repository,
                env=environment,
                check=False,
                capture_output=True,
                text=True,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("llm-proxy deploy dry-run complete", result.stdout)
            self.assertNotIn(str(unrelated), result.stdout + result.stderr)

    def test_deploy_converges_the_declared_resources_idempotently(self) -> None:
        uv_executable = shutil.which("uv")
        self.assertIsNotNone(uv_executable)

        class BoundaryHandler(http.server.BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                statuses = {
                    "/": 403,
                    "/config-ui.yaml": 200,
                }
                status = statuses.get(self.path, 404)
                self.send_response(status)
                self.end_headers()
                self.wfile.write(b"ok")

            def log_message(self, format: str, *args: object) -> None:
                return

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), BoundaryHandler)
        server_thread = threading.Thread(target=server.serve_forever, daemon=True)
        server_thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        port = server.server_address[1]

        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            repository = fixture_root / "llm-proxy"
            unrelated = fixture_root / "unrelated-invalid-repository"
            bare_remote = fixture_root / "llm-proxy.git"
            remote_root = fixture_root / "target"
            docker_state = fixture_root / "docker-state"
            docker_log = fixture_root / "docker.log"
            pages_log = fixture_root / "pages.log"
            tool_directory = fixture_root / "bin"
            tool_directory.mkdir()
            docker_state.mkdir()
            (remote_root / "configs").mkdir(parents=True)
            (remote_root / "caddy/sites").mkdir(parents=True)
            (remote_root / "caddy/sites/other.caddy").write_text(
                "other.example.invalid { respond 200 }\n",
                encoding="utf-8",
            )
            (remote_root / "configs/config.tauth.yml").write_text(
                "server:\n"
                "  cors_allowed_origins:\n"
                "    - https://other.example.invalid\n"
                "    - https://stale-llm-proxy.example.invalid\n"
                "tenants:\n"
                "  - id: other\n"
                "    display_name: Other\n"
                "    tenant_origins:\n"
                "      - https://other.example.invalid\n"
                "    session_cookie_name: other_session\n"
                "    refresh_cookie_name: other_refresh\n"
                "  - id: llm-proxy\n"
                "    display_name: Stale LLM Proxy\n"
                "    tenant_origins:\n"
                "      - https://stale-llm-proxy.example.invalid\n"
                "    session_cookie_name: app_session_llm_proxy\n"
                "    refresh_cookie_name: app_refresh_llm_proxy\n",
                encoding="utf-8",
            )
            tauth_environment = remote_root / "configs/.env.tauth"
            tauth_environment.write_text("PLACEHOLDER=value\n", encoding="utf-8")
            tauth_environment.chmod(0o600)
            self.write_executable(
                tool_directory / "docker",
                """#!/usr/bin/env python3
import json
import os
import sys
from pathlib import Path

arguments = sys.argv[1:]
state = Path(os.environ["FAKE_DOCKER_STATE"])
log = Path(os.environ["FAKE_DOCKER_LOG"])
with log.open("a", encoding="utf-8") as stream:
    stream.write(json.dumps(arguments) + "\\n")

if arguments == ["version"]:
    raise SystemExit(0)
if arguments[:2] == ["network", "inspect"]:
    raise SystemExit(0)
if arguments == ["inspect", "tauth-api"] or arguments == ["inspect", "caddy2"]:
    print("[]")
    raise SystemExit(0)
if arguments == ["inspect", "--format", "{{.Config.Image}}", "tauth-api"]:
    print("example.invalid/tauth:v1")
    raise SystemExit(0)
if arguments == ["inspect", "--format", "{{.State.Running}}", "tauth-api"]:
    print("true")
    raise SystemExit(0)
if arguments[:1] == ["run"]:
    raise SystemExit(0)
if arguments == ["restart", "tauth-api"]:
    raise SystemExit(0)
if arguments[:1] == ["cp"]:
    raise SystemExit(0)
if arguments[:2] == ["exec", "caddy2"]:
    raise SystemExit(0)
if arguments[:2] == ["image", "inspect"]:
    image = arguments[2]
    marker = state / "image"
    if marker.exists() and marker.read_text(encoding="utf-8") == image:
        print("[]")
        raise SystemExit(0)
    raise SystemExit(1)
if arguments[:1] == ["pull"]:
    (state / "image").write_text(arguments[1], encoding="utf-8")
    raise SystemExit(0)
if arguments[:1] == ["compose"]:
    compose_path = Path(arguments[arguments.index("--file") + 1])
    image_line = next(
        line for line in compose_path.read_text(encoding="utf-8").splitlines()
        if line.strip().startswith("image:")
    )
    image = image_line.split(":", 1)[1].strip()
    marker = state / "backend"
    if not marker.exists():
        print("Created")
    marker.write_text(image, encoding="utf-8")
    raise SystemExit(0)
if arguments == ["inspect", "llm-proxy"]:
    image = (state / "backend").read_text(encoding="utf-8")
    print(json.dumps([{
        "State": {"Running": True},
        "Config": {"Image": image},
        "NetworkSettings": {
            "Networks": {"mprlab-nginx-gateway_default": {}}
        },
    }]))
    raise SystemExit(0)
print(f"unsupported fake Docker command: {arguments}", file=sys.stderr)
raise SystemExit(2)
""",
            )

            shutil.copytree(
                REPOSITORY_ROOT / ".mprlab/deploy",
                repository / ".mprlab/deploy",
            )
            resources = repository / ".mprlab/deploy/resources.yml"
            resources_text = resources.read_text(encoding="utf-8")
            resources_text = resources_text.replace(
                "https://llm-proxy-api.mprlab.com/config-ui.yaml",
                f"http://127.0.0.1:{port}/config-ui.yaml",
            ).replace(
                "https://llm-proxy-api.mprlab.com/",
                f"http://127.0.0.1:{port}/",
            )
            resources.write_text(resources_text, encoding="utf-8")
            inventory = repository / ".mprlab/deploy/ansible/inventory/hosts.yml"
            inventory.write_text(
                "---\n"
                "all:\n"
                "  children:\n"
                "    gateway:\n"
                "      hosts:\n"
                "        gateway-host:\n"
                "          ansible_connection: local\n"
                "          ansible_become: false\n"
                f"          mprlab_runtime_root: {remote_root}\n"
                f"          mprlab_docker_cli: {tool_directory / 'docker'}\n"
                "          mprlab_compose_project_name: mprlab-nginx-gateway\n",
                encoding="utf-8",
            )
            inventory.chmod(0o600)
            private_environment = repository / ".mprlab/deploy/.env"
            private_environment.write_text("PLACEHOLDER=value\n", encoding="utf-8")
            private_environment.chmod(0o600)
            (repository / "configs").mkdir(parents=True)
            (repository / "configs/config.yml").write_text(
                "server:\n  max_request_timeout_seconds: 3600\n",
                encoding="utf-8",
            )
            (repository / "Makefile").write_text(
                "pages-deploy:\n"
                "\t@printf '%s\\t%s\\n' \"$(PAGES_VERSION)\" "
                "\"$(DEPLOY_PAGES_ARGS)\" >>\"$(PAGES_CAPTURE)\"\n",
                encoding="utf-8",
            )
            release_scripts = repository / "tools/gitrelease/scripts"
            release_scripts.mkdir(parents=True)
            self.write_executable(
                release_scripts / "release_helper.py",
                "#!/usr/bin/env python3\n"
                "import json\n"
                "import subprocess\n"
                "import sys\n"
                "if sys.argv[1] != 'local-release-state':\n"
                "    raise SystemExit(2)\n"
                "commit = subprocess.run(\n"
                "    ['git', 'rev-parse', 'HEAD'],\n"
                "    check=True,\n"
                "    capture_output=True,\n"
                "    text=True,\n"
                ").stdout.strip()\n"
                "print(json.dumps({\n"
                "    'ok': True,\n"
                "    'state': 'sealed',\n"
                "    'version': 'v1.0.0',\n"
                "    'release_commit': commit,\n"
                "}))\n",
            )
            self.write_executable(
                release_scripts / "resolve_container_manifest_digest.sh",
                "#!/usr/bin/env bash\n"
                "set -euo pipefail\n"
                "[[ $# -eq 1 ]]\n"
                "printf 'sha256:%064d\\n' 0\n",
            )
            (repository / ".gitignore").write_text(
                ".mprlab/deploy/.env\n"
                ".mprlab/deploy/ansible/inventory/hosts.yml\n",
                encoding="utf-8",
            )
            self.run_git(repository, "init", "--initial-branch=master")
            self.run_git(repository, "config", "user.email", "fixture@example.invalid")
            self.run_git(repository, "config", "user.name", "Fixture")
            self.run_git(repository, "add", ".")
            self.run_git(repository, "commit", "-m", "fixture release")
            self.run_git(repository, "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
            subprocess.run(
                ["git", "init", "--bare", "--initial-branch=master", str(bare_remote)],
                check=True,
                capture_output=True,
                text=True,
            )
            self.run_git(repository, "remote", "add", "origin", str(bare_remote))
            self.run_git(repository, "push", "--set-upstream", "origin", "master")
            self.run_git(repository, "push", "origin", "v1.0.0")

            (unrelated / ".mprlab/deploy").mkdir(parents=True)
            (unrelated / ".mprlab/deploy/resources.yml").write_text(
                "not: [valid\n",
                encoding="utf-8",
            )
            environment = os.environ.copy()
            environment.update(
                {
                    "MPRLAB_DEPLOY_MODE": "deploy",
                    "LLM_PROXY_DEPLOY_PUBLISH_REMOTE": "origin",
                    "LLM_PROXY_DEPLOY_PUBLISH_BRANCH": "master",
                    "LLM_PROXY_DEPLOY_PAGES_BRANCH": "gh-pages",
                    "LLM_PROXY_DEPLOY_PAGES_URL": "https://llm-proxy.mprlab.com/",
                    "ANSIBLE_CONFIG": str(
                        repository / ".mprlab/deploy/ansible/ansible.cfg"
                    ),
                    "ANSIBLE_LOCAL_TEMP": str(fixture_root / "ansible-local"),
                    "FAKE_DOCKER_STATE": str(docker_state),
                    "FAKE_DOCKER_LOG": str(docker_log),
                    "PAGES_CAPTURE": str(pages_log),
                    "DEPLOY_ARGS": "forbidden-selector",
                    "GATEWAY_DIR": str(unrelated),
                    "MPRLAB_APP_REPO_PARENT": str(unrelated),
                    "MPRLAB_APP_REPO_PARENTS": str(unrelated),
                }
            )
            command = [
                str(uv_executable),
                "tool",
                "run",
                "--from",
                ANSIBLE_CORE_SPEC,
                "ansible-playbook",
                "--inventory",
                str(inventory),
                str(repository / ".mprlab/deploy/ansible/playbooks/deploy.yml"),
            ]

            first = subprocess.run(
                command,
                cwd=repository,
                env=environment,
                check=False,
                capture_output=True,
                text=True,
            )
            second = subprocess.run(
                command,
                cwd=repository,
                env=environment,
                check=False,
                capture_output=True,
                text=True,
            )

            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertEqual(second.returncode, 0, second.stderr)
            self.assertIn("llm-proxy deploy complete version=v1.0.0", second.stdout)
            self.assertNotIn(str(unrelated), first.stdout + first.stderr)
            self.assertNotIn(str(unrelated), second.stdout + second.stderr)
            tauth_document = (
                remote_root / "configs/config.tauth.yml"
            ).read_text(encoding="utf-8")
            self.assertEqual(tauth_document.count("id: llm-proxy"), 1)
            self.assertEqual(tauth_document.count("id: other"), 1)
            self.assertNotIn(
                "https://stale-llm-proxy.example.invalid",
                tauth_document,
            )
            self.assertTrue(
                (remote_root / "caddy/sites/llm-proxy.caddy").is_file()
            )
            self.assertTrue(
                (remote_root / "apps/llm-proxy/docker-compose.yml").is_file()
            )
            docker_invocations = [
                json.loads(line)
                for line in docker_log.read_text(encoding="utf-8").splitlines()
            ]
            self.assertEqual(
                docker_invocations.count(
                    [
                        "pull",
                        "ghcr.io/tyemirov/llm-proxy@"
                        f"sha256:{'0' * 64}",
                    ]
                ),
                1,
            )
            self.assertEqual(
                docker_invocations.count(["restart", "tauth-api"]),
                1,
            )
            tauth_preflight_invocations = [
                invocation
                for invocation in docker_invocations
                if invocation[:1] == ["run"]
            ]
            self.assertEqual(len(tauth_preflight_invocations), 2)
            self.assertTrue(
                all(
                    "TAUTH_DATABASE_URL=" in invocation
                    for invocation in tauth_preflight_invocations
                )
            )
            self.assertEqual(
                sum(
                    invocation[:5]
                    == ["exec", "caddy2", "caddy", "reload", "--config"]
                    for invocation in docker_invocations
                ),
                1,
            )
            self.assertEqual(
                sum(
                    invocation[:2] == ["cp", str(remote_root / "caddy/sites/other.caddy")]
                    for invocation in docker_invocations
                ),
                2,
            )
            self.assertEqual(
                sum(invocation[:1] == ["compose"] for invocation in docker_invocations),
                2,
            )
            self.assertEqual(
                pages_log.read_text(encoding="utf-8").splitlines(),
                [
                    "v1.0.0\t--verify-only",
                    "v1.0.0\t",
                    "v1.0.0\t--verify-only",
                    "v1.0.0\t",
                ],
            )

    def run_make(
        self,
        environment: dict[str, str],
        target: str,
        *assignments: str,
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["make", "--no-print-directory", target, *assignments],
            cwd=REPOSITORY_ROOT,
            env=environment,
            check=False,
            capture_output=True,
            text=True,
        )

    def run_git(self, repository: Path, *arguments: str) -> None:
        subprocess.run(
            ["git", *arguments],
            cwd=repository,
            check=True,
            capture_output=True,
            text=True,
        )

    def write_executable(self, path: Path, contents: str) -> None:
        path.write_text(contents, encoding="utf-8")
        path.chmod(0o755)


if __name__ == "__main__":
    unittest.main()
