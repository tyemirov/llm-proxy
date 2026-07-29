from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]


class DeployContractTests(unittest.TestCase):
    def test_make_deploy_invokes_only_the_installed_neutral_controller(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture_root = Path(temporary_directory)
            capture = fixture_root / "capture"
            tool_directory = fixture_root / "bin"
            tool_directory.mkdir()
            self.write_executable(
                tool_directory / "mprlab-deploy",
                "#!/usr/bin/env bash\n"
                "set -euo pipefail\n"
                "[[ $# -eq 0 ]]\n"
                "printf '%s\\t%s\\n' \"$PWD\" \"$#\" >>\"${DEPLOY_CAPTURE}\"\n",
            )
            environment = os.environ.copy()
            environment.update(
                {
                    "DEPLOY_CAPTURE": str(capture),
                    "PATH": f"{tool_directory}{os.pathsep}{environment['PATH']}",
                }
            )

            first = self.run_make_deploy(environment)
            second = self.run_make_deploy(
                environment,
                "DEPLOY_ARGS=forbidden-selector",
                "GATEWAY_DIR=/unrelated/checkout",
            )

            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertEqual(second.returncode, 0, second.stderr)
            self.assertEqual(
                capture.read_text(encoding="utf-8").splitlines(),
                [
                    f"{REPOSITORY_ROOT}\t0",
                    f"{REPOSITORY_ROOT}\t0",
                ],
            )

    def test_make_deploy_propagates_controller_failure(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            tool_directory = Path(temporary_directory)
            self.write_executable(
                tool_directory / "mprlab-deploy",
                "#!/usr/bin/env bash\nexit 37\n",
            )
            environment = os.environ.copy()
            environment["PATH"] = (
                f"{tool_directory}{os.pathsep}{environment['PATH']}"
            )

            result = self.run_make_deploy(environment)

            self.assertNotEqual(result.returncode, 0)

    def test_repository_contains_declarations_not_a_deployment_controller(self) -> None:
        tracked_deploy_paths = subprocess.run(
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
        self.assertEqual(
            tracked_deploy_paths,
            [
                ".mprlab/deploy/ansible/inventory/hosts.example.yml",
                ".mprlab/deploy/resources.yml",
            ],
        )
        self.assertFalse((REPOSITORY_ROOT / "scripts/deploy.sh").exists())
        self.assertFalse(
            (REPOSITORY_ROOT / "scripts/prepare_gateway_controller.py").exists()
        )
        self.assertFalse(
            (REPOSITORY_ROOT / "scripts/run-app-ansible-deploy.sh").exists()
        )
        self.assertFalse(
            (REPOSITORY_ROOT / "scripts/run-app-deploy-transaction.sh").exists()
        )
        makefile = (REPOSITORY_ROOT / "Makefile").read_text(encoding="utf-8")
        deploy_recipe = makefile.split("\ndeploy:\n", maxsplit=1)[1].split(
            "\n\n", maxsplit=1
        )[0]
        self.assertEqual(deploy_recipe.rstrip("\n"), "\t@mprlab-deploy")

    def run_make_deploy(
        self,
        environment: dict[str, str],
        *assignments: str,
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["make", "--no-print-directory", "deploy", *assignments],
            cwd=REPOSITORY_ROOT,
            env=environment,
            check=False,
            capture_output=True,
            text=True,
        )

    def write_executable(self, path: Path, contents: str) -> None:
        path.write_text(contents, encoding="utf-8")
        path.chmod(0o755)


if __name__ == "__main__":
    unittest.main()
