from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
TRANSACTION_SCRIPT = REPOSITORY_ROOT / "scripts" / "run-app-deploy-transaction.sh"
IMAGE_REF = f"ghcr.io/tyemirov/llm-proxy@sha256:{'c' * 64}"


class DeployTransactionTests(unittest.TestCase):
    def test_dry_run_never_runs_a_remote_app_phase_or_uses_a_password(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_transaction(fixture, "dry-run")

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                self.read_capture(fixture),
                [
                    "ansible\tlocal",
                    (
                        "controller\t--mode dry-run --target llm-proxy "
                        f"--inventory {fixture['inventory']} "
                        f"--repo-parents {fixture['repo_parent']} "
                        f"--image-ref {IMAGE_REF} "
                        f"--ansible-python {fixture['ansible_python']} "
                        f"--collections-path {fixture['collections']}"
                    ),
                ],
            )
            self.assertIn("production hosts were not contacted", result.stdout)

    def test_deploy_orders_app_preflight_controller_deploy_and_verification(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_transaction(fixture, "deploy")

            self.assertEqual(result.returncode, 0, result.stderr)
            capture = self.read_capture(fixture)
            self.assertEqual(capture[0:2], ["ansible\tlocal", "ansible\tpreflight"])
            self.assertTrue(
                capture[2].startswith(
                    "controller\t--mode deploy --target llm-proxy "
                ),
                capture,
            )
            self.assertIn("--become-password-file", capture[2])
            self.assertEqual(capture[3:], ["ansible\tdeploy", "ansible\tverify"])

    def test_transaction_rejects_a_mutable_image(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_transaction(
                fixture,
                "dry-run",
                image_ref="ghcr.io/tyemirov/llm-proxy:latest",
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                "--image-ref must be the immutable LLM Proxy GHCR digest",
                result.stderr,
            )
            self.assertEqual(self.read_capture(fixture), [])

    def initialize_fixture(self, fixture_root: Path) -> dict[str, Path]:
        repo_parent = fixture_root / "repositories"
        repo_root = repo_parent / "llm-proxy"
        playbook_root = repo_root / ".mprlab" / "deploy" / "ansible"
        inventory = playbook_root / "inventory" / "hosts.yml"
        controller = fixture_root / "controller" / "bin" / "mprlab-gateway-deploy-target"
        collections = fixture_root / "collections"
        capture = fixture_root / "transaction.log"
        ansible_python = fixture_root / "ansible-python"
        for path in (
            playbook_root / "playbooks",
            inventory.parent,
            controller.parent,
            collections,
        ):
            path.mkdir(parents=True, exist_ok=True)
        (playbook_root / "ansible.cfg").write_text("[defaults]\n", encoding="utf-8")
        (playbook_root / "playbooks" / "preflight-local.yml").write_text(
            "---\n",
            encoding="utf-8",
        )
        (playbook_root / "playbooks" / "dispatch.yml").write_text(
            "---\n",
            encoding="utf-8",
        )
        inventory.write_text("---\nall: {}\n", encoding="utf-8")
        controller.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "printf 'controller\\t%s\\n' \"$*\" >>\"${TRANSACTION_CAPTURE}\"\n",
            encoding="utf-8",
        )
        controller.chmod(0o755)
        ansible_python.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "if [[ \"${1:-}\" == '-c' ]]; then\n"
            "  exit 0\n"
            "fi\n"
            "phase=\"${LLM_PROXY_DEPLOY_PHASE:-local}\"\n"
            "printf 'ansible\\t%s\\n' \"${phase}\" >>\"${TRANSACTION_CAPTURE}\"\n",
            encoding="utf-8",
        )
        ansible_python.chmod(0o755)

        unrelated_manifest = (
            repo_parent
            / "unrelated-app"
            / ".mprlab"
            / "deploy"
            / "resources.yml"
        )
        unrelated_manifest.parent.mkdir(parents=True)
        unrelated_manifest.write_text("not: a selected target\n", encoding="utf-8")
        return {
            "repo_parent": repo_parent,
            "repo_root": repo_root,
            "inventory": inventory,
            "controller_root": controller.parents[1],
            "collections": collections,
            "capture": capture,
            "ansible_python": ansible_python,
            "unrelated_manifest": unrelated_manifest,
        }

    def run_transaction(
        self,
        fixture: dict[str, Path],
        mode: str,
        *,
        image_ref: str = IMAGE_REF,
    ) -> subprocess.CompletedProcess[str]:
        arguments = [
            str(TRANSACTION_SCRIPT),
            "--mode",
            mode,
            "--repo-root",
            str(fixture["repo_root"]),
            "--inventory",
            str(fixture["inventory"]),
            "--image-ref",
            image_ref,
            "--controller-root",
            str(fixture["controller_root"]),
            "--ansible-python",
            str(fixture["ansible_python"]),
            "--collections-path",
            str(fixture["collections"]),
            "--repo-parents",
            str(fixture["repo_parent"]),
        ]
        if mode == "deploy":
            password_file = fixture["repo_root"].parent / "become-password"
            password_file.write_text("fixture\n", encoding="utf-8")
            password_file.chmod(0o600)
            arguments.extend(["--become-password-file", str(password_file)])
        environment = os.environ.copy()
        environment["TRANSACTION_CAPTURE"] = str(fixture["capture"])
        return subprocess.run(
            arguments,
            env=environment,
            check=False,
            capture_output=True,
            text=True,
        )

    def read_capture(self, fixture: dict[str, Path]) -> list[str]:
        capture = fixture["capture"]
        if not capture.exists():
            return []
        return capture.read_text(encoding="utf-8").splitlines()


if __name__ == "__main__":
    unittest.main()
