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
RELEASE_DIGEST = f"sha256:{'a' * 64}"
LATEST_DIGEST = RELEASE_DIGEST
IMMUTABLE_IMAGE = f"ghcr.io/tyemirov/llm-proxy@{RELEASE_DIGEST}"


class DeployContractTests(unittest.TestCase):
    def test_deploy_rejects_missing_sealed_release_before_artifact_work(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_deploy(fixture, {"ok": True, "state": "new"})

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                "make deploy requires the exact sealed release created by make release",
                result.stderr,
            )
            self.assertEqual(self.read_capture(fixture), [])

    def test_deploy_rejects_sealed_release_version_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_deploy(
                fixture,
                self.sealed_release_state(fixture["application"], version="v9.9.9"),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                f"sealed release version v9.9.9 does not match deploy tag {RELEASE_VERSION}",
                result.stderr,
            )
            self.assertEqual(self.read_capture(fixture), [])

    def test_deploy_rejects_sealed_release_commit_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))
            mismatched_release_commit = "0" * 40

            result = self.run_deploy(
                fixture,
                self.sealed_release_state(
                    fixture["application"],
                    release_commit=mismatched_release_commit,
                ),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                f"sealed release commit {mismatched_release_commit} does not match deploy HEAD",
                result.stderr,
            )
            self.assertEqual(self.read_capture(fixture), [])

    def test_deploy_validates_pages_before_backend_and_forwards_exact_image(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_deploy(
                fixture,
                self.sealed_release_state(fixture["application"]),
                remote="upstream",
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                self.read_capture(fixture),
                [
                    f"pages\t--verify-only\tupstream\t{RELEASE_VERSION}",
                    f"backend\t--mode deploy\t{IMMUTABLE_IMAGE}",
                    f"pages\t\tupstream\t{RELEASE_VERSION}",
                ],
            )
            self.assertTrue(fixture["unrelated_manifest"].exists())

    def test_deploy_stops_before_backend_when_pages_preflight_fails(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_deploy(
                fixture,
                self.sealed_release_state(fixture["application"]),
                fail_pages_preflight=True,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(
                self.read_capture(fixture),
                [f"pages\t--verify-only\torigin\t{RELEASE_VERSION}"],
            )

    def test_deploy_dry_run_is_local_transaction_only(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_deploy(
                fixture,
                self.sealed_release_state(fixture["application"]),
                mode="dry-run",
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                self.read_capture(fixture),
                [f"backend\t--mode dry-run\t{IMMUTABLE_IMAGE}"],
            )
            self.assertIn("production state was not changed", result.stdout)

    def test_repeated_deploy_reuses_the_exact_desired_state(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))
            release_state = self.sealed_release_state(fixture["application"])

            first = self.run_deploy(fixture, release_state)
            second = self.run_deploy(fixture, release_state)

            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertEqual(second.returncode, 0, second.stderr)
            expected_transaction = [
                f"pages\t--verify-only\torigin\t{RELEASE_VERSION}",
                f"backend\t--mode deploy\t{IMMUTABLE_IMAGE}",
                f"pages\t\torigin\t{RELEASE_VERSION}",
            ]
            self.assertEqual(
                self.read_capture(fixture),
                expected_transaction + expected_transaction,
            )

    def test_deploy_rejects_conflicting_latest_image(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            fixture = self.initialize_fixture(Path(temporary_directory))

            result = self.run_deploy(
                fixture,
                self.sealed_release_state(fixture["application"]),
                latest_digest=f"sha256:{'b' * 64}",
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("does not match", result.stderr)
            self.assertIn("run make publish first", result.stderr)
            self.assertEqual(self.read_capture(fixture), [])

    def initialize_fixture(self, fixture_root: Path) -> dict[str, Path]:
        application = fixture_root / "llm-proxy"
        scripts = application / "scripts"
        release_tools = application / "tools" / "gitrelease" / "scripts"
        tool_directory = fixture_root / "bin"
        scripts.mkdir(parents=True)
        release_tools.mkdir(parents=True)
        tool_directory.mkdir()
        capture = fixture_root / "transaction.log"

        self.write_executable(
            scripts / "run-app-ansible-deploy.sh",
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "printf 'backend\\t%s\\t%s\\n' \"$*\" "
            "\"${MPRLAB_LLM_PROXY_IMAGE_REF:-}\" >>\"${DEPLOY_TEST_CAPTURE}\"\n",
        )
        self.write_executable(
            release_tools / "resolve_container_manifest_digest.sh",
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "case \"$1\" in\n"
            "  *:latest) printf '%s\\n' \"${DEPLOY_TEST_LATEST_DIGEST}\" ;;\n"
            "  *) printf '%s\\n' \"${DEPLOY_TEST_RELEASE_DIGEST}\" ;;\n"
            "esac\n",
        )
        self.write_executable(
            tool_directory / "docker",
            "#!/usr/bin/env bash\nexit 0\n",
        )
        self.write_executable(
            tool_directory / "make",
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "printf 'pages\\t%s\\t%s\\t%s\\n' "
            "\"${DEPLOY_PAGES_ARGS:-}\" \"${PUBLISH_REMOTE:-}\" "
            "\"${PAGES_VERSION:-}\" >>\"${DEPLOY_TEST_CAPTURE}\"\n"
            "if [[ \"${DEPLOY_PAGES_ARGS:-}\" == '--verify-only' "
            "&& \"${DEPLOY_TEST_FAIL_PAGES_PREFLIGHT:-0}\" == '1' ]]; then\n"
            "  exit 42\n"
            "fi\n",
        )

        (application / "README.md").write_text("fixture\n", encoding="utf-8")
        self.initialize_repository(application)
        self.run_git(
            application,
            "tag",
            "-a",
            RELEASE_VERSION,
            "-m",
            f"Release {RELEASE_VERSION}",
        )

        origin = fixture_root / "application-origin.git"
        subprocess.run(
            ["git", "init", "--bare", str(origin)],
            check=True,
            capture_output=True,
            text=True,
        )
        self.run_git(application, "remote", "add", "origin", str(origin))
        self.run_git(application, "remote", "add", "upstream", str(origin))
        self.run_git(application, "push", "--set-upstream", "origin", "master")
        self.run_git(application, "push", "origin", "--tags")

        unrelated_manifest = (
            fixture_root
            / "download_your_data"
            / ".mprlab"
            / "deploy"
            / "resources.yml"
        )
        unrelated_manifest.parent.mkdir(parents=True)
        unrelated_manifest.write_text(
            "this: is intentionally unrelated and untracked\n",
            encoding="utf-8",
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
            "    version = sys.argv[sys.argv.index('--version') + 1]\n"
            "    if not version.startswith('v'):\n"
            "        raise SystemExit('version must be canonical')\n"
            "else:\n"
            "    raise SystemExit(f'unexpected release helper command: {sys.argv[1]}')\n",
            encoding="utf-8",
        )
        return {
            "application": application,
            "capture": capture,
            "release_helper": release_helper,
            "tool_directory": tool_directory,
            "unrelated_manifest": unrelated_manifest,
        }

    def initialize_repository(self, repository: Path) -> None:
        subprocess.run(
            ["git", "init", "--initial-branch=master"],
            cwd=repository,
            check=True,
            capture_output=True,
            text=True,
        )
        self.run_git(repository, "add", ".")
        self.run_git(repository, "commit", "-m", "Release fixture")

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
        application: Path,
        *,
        version: str = RELEASE_VERSION,
        release_commit: str | None = None,
    ) -> dict[str, object]:
        effective_release_commit = release_commit or subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=application,
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
        fixture: dict[str, Path],
        release_state: dict[str, object],
        *,
        mode: str = "deploy",
        remote: str = "origin",
        latest_digest: str = LATEST_DIGEST,
        fail_pages_preflight: bool = False,
    ) -> subprocess.CompletedProcess[str]:
        release_state_path = fixture["application"].parent / "release-state.json"
        release_state_path.write_text(
            json.dumps(release_state) + "\n",
            encoding="utf-8",
        )
        environment = os.environ.copy()
        environment.update(
            {
                "PATH": (
                    f"{fixture['tool_directory']}{os.pathsep}"
                    f"{environment.get('PATH', '')}"
                ),
                "DEPLOY_TEST_CAPTURE": str(fixture["capture"]),
                "DEPLOY_TEST_RELEASE_STATE": str(release_state_path),
                "DEPLOY_TEST_RELEASE_DIGEST": RELEASE_DIGEST,
                "DEPLOY_TEST_LATEST_DIGEST": latest_digest,
                "DEPLOY_TEST_FAIL_PAGES_PREFLIGHT": (
                    "1" if fail_pages_preflight else "0"
                ),
                "LLM_PROXY_DEPLOY_MODE": mode,
                "PUBLISH_REMOTE": remote,
                "PUBLISH_BRANCH": "master",
                "RELEASE_HELPER": str(fixture["release_helper"]),
            }
        )
        return subprocess.run(
            [str(DEPLOY_SCRIPT)],
            cwd=fixture["application"],
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

    def write_executable(self, path: Path, contents: str) -> None:
        path.write_text(contents, encoding="utf-8")
        path.chmod(0o755)


if __name__ == "__main__":
    unittest.main()
