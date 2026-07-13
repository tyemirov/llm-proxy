from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
DEPLOY_SCRIPT = REPOSITORY_ROOT / "scripts" / "deploy.sh"


class DeployContractTests(unittest.TestCase):
    def test_deploy_rejects_gateway_without_coupled_tauth_contract(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            gateway_directory = self.initialize_gateway(
                Path(temporary_directory),
                ".PHONY: deploy-llm-proxy-backend\n"
                "deploy-llm-proxy-backend:\n"
                "\t@touch deployed\n",
            )

            result = self.run_deploy(gateway_directory)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                "gateway checkout does not satisfy the coupled llm-proxy/TAuth deployment contract",
                result.stderr,
            )
            self.assertFalse((gateway_directory / "deployed").exists())

    def test_deploy_rejects_dirty_gateway_before_contract_or_deployment(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            gateway_directory = self.initialize_gateway(
                Path(temporary_directory),
                ".PHONY: verify-llm-proxy-deployment-contract deploy-llm-proxy-backend\n"
                "verify-llm-proxy-deployment-contract:\n"
                "\t@touch verified\n"
                "deploy-llm-proxy-backend:\n"
                "\t@touch deployed\n",
            )
            (gateway_directory / "uncommitted-change").write_text(
                "dirty\n", encoding="utf-8"
            )

            result = self.run_deploy(gateway_directory)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("gateway working tree is dirty", result.stderr)
            self.assertFalse((gateway_directory / "verified").exists())
            self.assertFalse((gateway_directory / "deployed").exists())

    def initialize_gateway(
        self, temporary_directory: Path, makefile_contents: str
    ) -> Path:
        gateway_directory = temporary_directory / "mprlab-gateway"
        gateway_directory.mkdir()
        (gateway_directory / "Makefile").write_text(
            makefile_contents, encoding="utf-8"
        )
        subprocess.run(
            ["git", "init", "--initial-branch=master"],
            cwd=gateway_directory,
            check=True,
            capture_output=True,
            text=True,
        )
        subprocess.run(
            ["git", "add", "Makefile"],
            cwd=gateway_directory,
            check=True,
            capture_output=True,
            text=True,
        )
        subprocess.run(
            [
                "git",
                "-c",
                "user.name=Deploy Contract Test",
                "-c",
                "user.email=deploy-contract@example.invalid",
                "commit",
                "-m",
                "test gateway fixture",
            ],
            cwd=gateway_directory,
            check=True,
            capture_output=True,
            text=True,
        )
        return gateway_directory

    def run_deploy(self, gateway_directory: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                str(DEPLOY_SCRIPT),
                "--gateway-dir",
                str(gateway_directory),
                "--skip-ci",
                "--skip-image-verify",
                "--skip-pages",
            ],
            cwd=REPOSITORY_ROOT,
            check=False,
            capture_output=True,
            text=True,
        )


if __name__ == "__main__":
    unittest.main()
