from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
DEPLOY_SCRIPT = REPOSITORY_ROOT / "scripts" / "deploy.sh"
RELEASE_VERSION = "v1.2.3"


class DeployContractTests(unittest.TestCase):
    def test_deploy_rejects_missing_sealed_release_before_gateway_work(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            application_directory = self.initialize_application(fixture_root)
            gateway_directory = self.initialize_gateway(
                fixture_root,
                self.complete_gateway_makefile(),
            )

            result = self.run_deploy(
                application_directory,
                gateway_directory,
                {"ok": True, "state": "new"},
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                "make deploy requires the exact sealed release created by make release",
                result.stderr,
            )
            self.assertFalse((application_directory / "ci-ran").exists())
            self.assertFalse((gateway_directory / "verified").exists())
            self.assertFalse((gateway_directory / "deployed").exists())

    def test_deploy_rejects_sealed_release_version_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            application_directory = self.initialize_application(fixture_root)
            gateway_directory = self.initialize_gateway(
                fixture_root,
                self.complete_gateway_makefile(),
            )

            result = self.run_deploy(
                application_directory,
                gateway_directory,
                self.sealed_release_state(
                    application_directory,
                    version="v9.9.9",
                ),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                f"sealed release version v9.9.9 does not match deploy tag {RELEASE_VERSION}",
                result.stderr,
            )
            self.assertFalse((application_directory / "ci-ran").exists())
            self.assertFalse((gateway_directory / "verified").exists())
            self.assertFalse((gateway_directory / "deployed").exists())

    def test_deploy_rejects_sealed_release_commit_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            application_directory = self.initialize_application(fixture_root)
            gateway_directory = self.initialize_gateway(
                fixture_root,
                self.complete_gateway_makefile(),
            )
            mismatched_release_commit = "0" * 40

            result = self.run_deploy(
                application_directory,
                gateway_directory,
                self.sealed_release_state(
                    application_directory,
                    release_commit=mismatched_release_commit,
                ),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                f"sealed release commit {mismatched_release_commit} does not match deploy HEAD",
                result.stderr,
            )
            self.assertFalse((application_directory / "ci-ran").exists())
            self.assertFalse((gateway_directory / "verified").exists())
            self.assertFalse((gateway_directory / "deployed").exists())

    def test_deploy_continues_sealed_release_without_invoking_ci(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            application_directory = self.initialize_application(fixture_root)
            gateway_directory = self.initialize_gateway(
                fixture_root,
                self.complete_gateway_makefile(),
            )

            result = self.run_deploy(
                application_directory,
                gateway_directory,
                self.sealed_release_state(application_directory),
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((application_directory / "ci-ran").exists())
            self.assertTrue((gateway_directory / "verified").exists())
            self.assertTrue((gateway_directory / "deployed").exists())

    def test_deploy_rejects_gateway_without_coupled_deployment_and_capacity_contract(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            application_directory = self.initialize_application(fixture_root)
            gateway_directory = self.initialize_gateway(
                fixture_root,
                ".PHONY: deploy-llm-proxy-backend\n"
                "deploy-llm-proxy-backend:\n"
                "\t@touch deployed\n",
            )

            result = self.run_deploy(
                application_directory,
                gateway_directory,
                self.sealed_release_state(application_directory),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                "gateway checkout does not satisfy the coupled llm-proxy/TAuth "
                "deployment and request-capacity contract",
                result.stderr,
            )
            self.assertFalse((application_directory / "ci-ran").exists())
            self.assertFalse((gateway_directory / "deployed").exists())

    def test_deploy_rejects_dirty_gateway_before_contract_or_deployment(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            application_directory = self.initialize_application(fixture_root)
            gateway_directory = self.initialize_gateway(
                fixture_root,
                self.complete_gateway_makefile(),
            )
            (gateway_directory / "uncommitted-change").write_text(
                "dirty\n", encoding="utf-8"
            )

            result = self.run_deploy(
                application_directory,
                gateway_directory,
                self.sealed_release_state(application_directory),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("gateway working tree is dirty", result.stderr)
            self.assertFalse((application_directory / "ci-ran").exists())
            self.assertFalse((gateway_directory / "verified").exists())
            self.assertFalse((gateway_directory / "deployed").exists())

    def initialize_application(self, fixture_root: Path) -> Path:
        application_directory = fixture_root / "llm-proxy"
        application_directory.mkdir()
        (application_directory / "CHANGELOG.md").write_text(
            "# Changelog\n", encoding="utf-8"
        )
        (application_directory / "Makefile").write_text(
            ".PHONY: ci\n"
            "ci:\n"
            "\t@touch ci-ran\n",
            encoding="utf-8",
        )
        self.initialize_repository(application_directory, "source")
        (application_directory / "CHANGELOG.md").write_text(
            f"# Changelog\n\n## {RELEASE_VERSION}\n", encoding="utf-8"
        )
        self.run_git(application_directory, "add", "CHANGELOG.md")
        self.run_git(
            application_directory,
            "commit",
            "-m",
            f"Release {RELEASE_VERSION}",
        )
        self.run_git(
            application_directory,
            "tag",
            "-a",
            RELEASE_VERSION,
            "-m",
            f"Release {RELEASE_VERSION}",
        )
        self.attach_origin(fixture_root, application_directory, "application-origin.git")
        return application_directory

    def initialize_gateway(
        self, fixture_root: Path, makefile_contents: str
    ) -> Path:
        gateway_directory = fixture_root / "mprlab-gateway"
        gateway_directory.mkdir()
        (gateway_directory / "Makefile").write_text(
            makefile_contents, encoding="utf-8"
        )
        self.initialize_repository(gateway_directory, "gateway fixture")
        self.attach_origin(fixture_root, gateway_directory, "gateway-origin.git")
        return gateway_directory

    def initialize_repository(self, repository: Path, message: str) -> None:
        subprocess.run(
            ["git", "init", "--initial-branch=master"],
            cwd=repository,
            check=True,
            capture_output=True,
            text=True,
        )
        self.run_git(repository, "add", ".")
        self.run_git(repository, "commit", "-m", message)

    def attach_origin(
        self, fixture_root: Path, repository: Path, origin_name: str
    ) -> None:
        origin_directory = fixture_root / origin_name
        subprocess.run(
            ["git", "init", "--bare", str(origin_directory)],
            check=True,
            capture_output=True,
            text=True,
        )
        self.run_git(repository, "remote", "add", "origin", str(origin_directory))
        self.run_git(repository, "push", "--set-upstream", "origin", "master")
        self.run_git(repository, "push", "origin", "--tags")

    def run_git(self, repository: Path, *arguments: str) -> None:
        subprocess.run(
            [
                "git",
                "-c",
                "user.name=Deploy Contract Test",
                "-c",
                "user.email=deploy-contract@example.invalid",
                *arguments,
            ],
            cwd=repository,
            check=True,
            capture_output=True,
            text=True,
        )

    def sealed_release_state(
        self,
        application_directory: Path,
        *,
        version: str = RELEASE_VERSION,
        release_commit: str | None = None,
    ) -> dict[str, object]:
        effective_release_commit = release_commit or subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=application_directory,
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()
        return {
            "ok": True,
            "state": "sealed",
            "version": version,
            "release_commit": effective_release_commit,
        }

    def run_deploy(
        self,
        application_directory: Path,
        gateway_directory: Path,
        release_state: dict[str, object],
    ) -> subprocess.CompletedProcess[str]:
        fixture_root = application_directory.parent
        release_state_path = fixture_root / "release-state.json"
        release_state_path.write_text(
            json.dumps(release_state) + "\n", encoding="utf-8"
        )
        release_helper = fixture_root / "release-helper.py"
        release_helper.write_text(
            "import os\n"
            "from pathlib import Path\n"
            "import sys\n"
            "\n"
            "if sys.argv[1] == 'local-release-state':\n"
            "    print(Path(os.environ['DEPLOY_TEST_RELEASE_STATE']).read_text(), end='')\n"
            "elif sys.argv[1] == 'validate-version':\n"
            "    raise SystemExit(0)\n"
            "else:\n"
            "    raise SystemExit(f'unexpected release helper command: {sys.argv[1]}')\n",
            encoding="utf-8",
        )
        environment = os.environ.copy()
        environment.update(
            {
                "DEPLOY_TEST_RELEASE_STATE": str(release_state_path),
                "RELEASE_HELPER": str(release_helper),
            }
        )
        return subprocess.run(
            [
                str(DEPLOY_SCRIPT),
                "--gateway-dir",
                str(gateway_directory),
                "--skip-image-verify",
                "--skip-pages",
            ],
            cwd=application_directory,
            env=environment,
            check=False,
            capture_output=True,
            text=True,
        )

    def complete_gateway_makefile(self) -> str:
        return (
            ".PHONY: verify-llm-proxy-deployment-contract "
            "deploy-llm-proxy-backend\n"
            "verify-llm-proxy-deployment-contract:\n"
            "\t@touch verified\n"
            "deploy-llm-proxy-backend:\n"
            "\t@touch deployed\n"
        )


if __name__ == "__main__":
    unittest.main()
