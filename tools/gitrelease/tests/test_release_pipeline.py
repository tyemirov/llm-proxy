from __future__ import annotations

import hashlib
import json
import os
import pathlib
import subprocess
import tarfile
import tempfile
import unittest


SKILL_ROOT = pathlib.Path(__file__).resolve().parents[1]
REPOSITORY_ROOT = SKILL_ROOT.parents[1]
HELPER = SKILL_ROOT / "scripts" / "release_helper.py"
PREPARE = SKILL_ROOT / "scripts" / "prepare_release.sh"
DEPLOY_PAGES = SKILL_ROOT / "scripts" / "deploy_pages_artifact.sh"


class ReleasePipelineTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temporary_directory.name)
        self.remote = self.root / "origin.git"
        self.repo = self.root / "repo"
        self.command("git", "init", "--bare", str(self.remote), cwd=self.root)
        self.command("git", "clone", str(self.remote), str(self.repo), cwd=self.root)
        self.command("git", "config", "user.name", "Release Test", cwd=self.repo)
        self.command("git", "config", "user.email", "release-test@example.invalid", cwd=self.repo)
        (self.repo / "README.md").write_text("# Fixture\n", encoding="utf-8")
        (self.repo / "Makefile").write_text("ci:\n\t@true\n", encoding="utf-8")
        self.command("git", "add", "README.md", "Makefile", cwd=self.repo)
        self.command("git", "commit", "-m", "Initial", cwd=self.repo)
        self.command("git", "branch", "-M", "master", cwd=self.repo)
        self.command("git", "push", "-u", "origin", "master", cwd=self.repo)
        self.command("git", "symbolic-ref", "HEAD", "refs/heads/master", cwd=self.remote, git_dir=True)
        self.command("git", "remote", "set-head", "origin", "-a", cwd=self.repo)

    def tearDown(self) -> None:
        self.temporary_directory.cleanup()

    def command(
        self,
        *command: str,
        cwd: pathlib.Path,
        check: bool = True,
        git_dir: bool = False,
        env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        actual_command = list(command)
        if git_dir:
            actual_command = [actual_command[0], f"--git-dir={cwd}", *actual_command[1:]]
            cwd = self.root
        return subprocess.run(
            actual_command,
            cwd=cwd,
            check=check,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=env,
        )

    def test_prepare_is_local_and_finalizes_hashed_payload_inventory(self) -> None:
        env = os.environ.copy()
        env["RELEASE_HELPER"] = str(HELPER)
        self.command(str(PREPARE), "--version", "v1.0.0", cwd=self.repo, env=env)

        remote_head = self.command("git", "rev-parse", "refs/heads/master", cwd=self.remote, git_dir=True).stdout.strip()
        local_parent = self.command("git", "rev-parse", "HEAD^", cwd=self.repo).stdout.strip()
        self.assertEqual(remote_head, local_parent)
        self.assertEqual(
            self.command("git", "rev-parse", "v1.0.0^{}", cwd=self.repo).stdout.strip(),
            self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip(),
        )

        artifact_dir = pathlib.Path(
            self.command("git", "rev-parse", "--git-path", "mprlab-release", cwd=self.repo).stdout.strip()
        )
        if not artifact_dir.is_absolute():
            artifact_dir = self.repo / artifact_dir
        manifest = json.loads((artifact_dir / "manifest.json").read_text(encoding="utf-8"))
        self.assertEqual(manifest["schema_version"], 2)
        self.assertEqual(manifest["payloads"], [])
        self.command(str(HELPER), "verify-release-artifact", cwd=self.repo)

    def test_payload_tampering_is_rejected(self) -> None:
        source_commit = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        self.command(
            str(HELPER),
            "initialize-release-artifact",
            "--version",
            "v1.0.0",
            "--source-commit",
            source_commit,
            "--release-timestamp",
            "2026-07-09T12:00:00-07:00",
            cwd=self.repo,
        )
        artifact_dir = pathlib.Path(
            self.command("git", "rev-parse", "--git-path", "mprlab-release", cwd=self.repo).stdout.strip()
        )
        if not artifact_dir.is_absolute():
            artifact_dir = self.repo / artifact_dir
        payload = artifact_dir / "payloads" / "release-assets" / "fixture.txt"
        payload.parent.mkdir(parents=True)
        payload.write_text("prepared\n", encoding="utf-8")
        notes = self.root / "notes.md"
        notes.write_text("## [v1.0.0] - 2026-07-09\n\n- Initial\n", encoding="utf-8")
        (self.repo / "CHANGELOG.md").write_text(notes.read_text(encoding="utf-8"), encoding="utf-8")
        self.command("git", "add", "CHANGELOG.md", cwd=self.repo)
        self.command("git", "commit", "-m", "Release v1.0.0", cwd=self.repo)
        release_commit = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        self.command("git", "tag", "-a", "v1.0.0", "-m", "Release v1.0.0", cwd=self.repo)
        self.command(
            str(HELPER),
            "write-release-artifact",
            "--version",
            "v1.0.0",
            "--source-commit",
            source_commit,
            "--release-commit",
            release_commit,
            "--notes-file",
            str(notes),
            "--default-branch",
            "master",
            "--release-timestamp",
            "2026-07-09T12:00:00-07:00",
            cwd=self.repo,
        )
        payload.write_text("tampered\n", encoding="utf-8")
        result = self.command(str(HELPER), "verify-release-artifact", cwd=self.repo, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("payload does not match", result.stdout)

    def test_pages_deploy_verifies_prepared_source_commit(self) -> None:
        source_commit, environment = self.pages_release_fixture()
        result = self.deploy_pages(environment)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"Verified https://pages.example.invalid/ at source {source_commit}.", result.stdout)

    def test_pages_deploy_rejects_mismatched_marker_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture(marker_source_commit="0" * 40)
        environment["PAGES_VERIFY_ATTEMPTS"] = "1"
        environment["PAGES_VERIFY_DELAY_SECONDS"] = "1"
        result = self.deploy_pages(environment)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_validates_retry_settings_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture()
        invalid_settings = (
            {"PAGES_VERIFY_ATTEMPTS": "1+1", "PAGES_VERIFY_DELAY_SECONDS": "1"},
            {"PAGES_VERIFY_ATTEMPTS": "1", "PAGES_VERIFY_DELAY_SECONDS": "0"},
        )
        for settings in invalid_settings:
            with self.subTest(settings=settings):
                test_environment = environment | settings
                result = self.deploy_pages(test_environment)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_rejects_git_hook_before_execution_or_remote_mutation(self) -> None:
        self.assert_pages_git_hook_rejected(".git")

    def test_pages_deploy_rejects_uppercase_git_hook_before_remote_mutation(self) -> None:
        self.assert_pages_git_hook_rejected(".GIT")

    def test_pages_deploy_rejects_mixed_case_git_hook_before_remote_mutation(self) -> None:
        self.assert_pages_git_hook_rejected(".GiT")

    def assert_pages_git_hook_rejected(self, git_directory_name: str) -> None:
        _, environment = self.pages_release_fixture(git_hook_directory=git_directory_name)
        hook_sentinel = self.root / "pages-hook-executed"
        result = self.deploy_pages(environment)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(hook_sentinel.exists(), result.stdout + result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_prepare_rejects_invalid_explicit_version_without_mutation(self) -> None:
        original_head = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        env = os.environ.copy()
        env["RELEASE_HELPER"] = str(HELPER)
        result = self.command(
            str(PREPARE),
            "--dry-run",
            "--version",
            "not-a-version",
            cwd=self.repo,
            check=False,
            env=env,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip(), original_head)

    def test_prepare_ignores_obsolete_calver_tags(self) -> None:
        self.command("git", "tag", "2026.7.9.1", cwd=self.repo)
        env = os.environ.copy()
        env["RELEASE_HELPER"] = str(HELPER)
        result = self.command(str(PREPARE), "--dry-run", cwd=self.repo, check=False, env=env)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("version_scheme=semver\n", result.stdout)
        self.assertIn("next_version=v1.0.0\n", result.stdout)

    def test_prepare_rejects_alternate_version_schemes(self) -> None:
        env = os.environ.copy()
        env["RELEASE_HELPER"] = str(HELPER)
        result = self.command(
            str(PREPARE),
            "--dry-run",
            "--scheme",
            "calver",
            cwd=self.repo,
            check=False,
            env=env,
        )
        self.assertNotEqual(result.returncode, 0)

    def test_publish_marks_prerelease_without_latest(self) -> None:
        result, command_log = self.publish_release_fixture("v1.2.3-rc.1")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("release create v1.2.3-rc.1", command_log)
        self.assertIn("--prerelease", command_log)
        self.assertNotIn("--latest", command_log)

    def test_publish_repairs_incorrect_prerelease_state(self) -> None:
        result, command_log = self.publish_release_fixture("v1.2.3-rc.1", existing_prerelease=False)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("release edit v1.2.3-rc.1", command_log)
        self.assertIn("--prerelease=true", command_log)
        published = json.loads(result.stdout)
        self.assertTrue(published["release"]["isPrerelease"])

    def test_publish_prepared_release_validates_selected_remote_tag(self) -> None:
        prepare_environment = os.environ.copy()
        prepare_environment["RELEASE_HELPER"] = str(HELPER)
        self.command(str(PREPARE), "--version", "v1.0.0", cwd=self.repo, env=prepare_environment)
        self.command("git", "remote", "rename", "origin", "upstream", cwd=self.repo)
        self.command("git", "push", "upstream", "refs/tags/v1.0.0:refs/tags/v1.0.0", cwd=self.repo)

        fake_binary_directory = self.root / "publish-bin"
        fake_binary_directory.mkdir()
        fake_gh = fake_binary_directory / "gh"
        fake_gh.write_text(
            "#!/usr/bin/env bash\nset -euo pipefail\n[[ \"$1\" == \"pr\" && \"$2\" == \"list\" ]]\nprintf '[]\\n'\n",
            encoding="utf-8",
        )
        fake_gh.chmod(0o755)
        publish_environment = os.environ.copy()
        publish_environment["PATH"] = f"{fake_binary_directory}{os.pathsep}{publish_environment['PATH']}"
        result = self.command(
            str(HELPER),
            "publish-prepared-release",
            "--remote",
            "upstream",
            "--dry-run",
            cwd=self.repo,
            check=False,
            env=publish_environment,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        published = json.loads(result.stdout)
        self.assertFalse(published["plan"]["push_tag"])

    def test_ci_tracks_repository_owned_release_tools(self) -> None:
        workflow = (REPOSITORY_ROOT / ".github" / "workflows" / "test.yml").read_text(encoding="utf-8")
        self.assertIn("      - 'tools/gitrelease/**'\n", workflow)

    def pages_release_fixture(
        self,
        marker_source_commit: str | None = None,
        git_hook_directory: str | None = None,
    ) -> tuple[str, dict[str, str]]:
        source_commit = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        (self.repo / "CHANGELOG.md").write_text("## [v1.0.0] - 2026-07-09\n\n- Release\n", encoding="utf-8")
        self.command("git", "add", "CHANGELOG.md", cwd=self.repo)
        self.command("git", "commit", "-m", "Release v1.0.0", cwd=self.repo)
        release_commit = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        self.command("git", "tag", "-a", "v1.0.0", "-m", "Release v1.0.0", cwd=self.repo)
        self.command("git", "push", "origin", "master", "--tags", cwd=self.repo)

        release_directory = self.root / "release"
        site_directory = self.root / "site"
        release_directory.mkdir()
        site_directory.mkdir()
        marker = {
            "schema_version": 1,
            "release_version": "v1.0.0",
            "source_commit": marker_source_commit or source_commit,
            "release_timestamp": "2026-07-09T12:00:00-07:00",
        }
        marker_path = site_directory / ".mprlab-release.json"
        marker_path.write_text(json.dumps(marker), encoding="utf-8")
        (site_directory / "index.html").write_text("release\n", encoding="utf-8")
        archive_path = release_directory / "pages.tar.gz"
        with tarfile.open(archive_path, "w:gz") as archive:
            archive.add(marker_path, arcname=".mprlab-release.json")
            archive.add(site_directory / "index.html", arcname="index.html")
            if git_hook_directory is not None:
                hook_path = site_directory / git_hook_directory / "hooks" / "pre-commit"
                hook_path.parent.mkdir(parents=True)
                hook_path.write_text('#!/bin/sh\n: > "${PAGES_HOOK_SENTINEL}"\n', encoding="utf-8")
                hook_path.chmod(0o755)
                archive.add(hook_path, arcname=f"{git_hook_directory}/hooks/pre-commit")
        archive_sha256 = hashlib.sha256(archive_path.read_bytes()).hexdigest()
        manifest = {
            "schema_version": 2,
            "artifact_kind": "mprlab.release",
            "version": "v1.0.0",
            "source_commit": source_commit,
            "release_commit": release_commit,
            "payloads": [
                {
                    "path": "payloads/release-assets/pages.tar.gz",
                    "size": archive_path.stat().st_size,
                    "sha256": archive_sha256,
                }
            ],
        }
        (release_directory / "manifest.json").write_text(json.dumps(manifest), encoding="utf-8")

        fake_binary_directory = self.root / "bin"
        fake_binary_directory.mkdir()
        fake_gh = fake_binary_directory / "gh"
        fake_gh.write_text(
            """#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == "release" && "$2" == "download" ]]
destination=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--dir" ]]; then destination="$2"; shift 2; else shift; fi
done
[[ -n "${destination}" ]]
cp "${FAKE_RELEASE_DIR}/manifest.json" "${destination}/manifest.json"
cp "${FAKE_RELEASE_DIR}/pages.tar.gz" "${destination}/pages.tar.gz"
""",
            encoding="utf-8",
        )
        fake_gh.chmod(0o755)
        fake_curl = fake_binary_directory / "curl"
        fake_curl.write_text("#!/usr/bin/env bash\ncat \"${FAKE_MARKER_PATH}\"\n", encoding="utf-8")
        fake_curl.chmod(0o755)

        environment = os.environ.copy()
        environment["PATH"] = f"{fake_binary_directory}{os.pathsep}{environment['PATH']}"
        environment["FAKE_RELEASE_DIR"] = str(release_directory)
        environment["FAKE_MARKER_PATH"] = str(marker_path)
        environment["PAGES_HOOK_SENTINEL"] = str(self.root / "pages-hook-executed")
        return source_commit, environment

    def deploy_pages(self, environment: dict[str, str]) -> subprocess.CompletedProcess[str]:
        return self.command(
            str(DEPLOY_PAGES),
            "--remote",
            "origin",
            "--branch",
            "gh-pages",
            "--version",
            "v1.0.0",
            "--url",
            "https://pages.example.invalid/",
            "--skip-configure",
            cwd=self.repo,
            check=False,
            env=environment,
        )

    def remote_branch_exists(self, branch: str) -> bool:
        result = self.command(
            "git",
            "show-ref",
            "--verify",
            f"refs/heads/{branch}",
            cwd=self.remote,
            git_dir=True,
            check=False,
        )
        return result.returncode == 0

    def publish_release_fixture(
        self,
        version: str,
        existing_prerelease: bool | None = None,
    ) -> tuple[subprocess.CompletedProcess[str], str]:
        notes = self.root / "release-notes.md"
        notes.write_text(f"## [{version}] - 2026-07-09\n\n- Candidate\n", encoding="utf-8")
        state = self.root / "fake-release-state.json"
        command_log = self.root / "fake-gh-commands.log"
        if existing_prerelease is not None:
            state.write_text(
                json.dumps(
                    {
                        "tagName": version,
                        "name": f"Release {version}",
                        "body": notes.read_text(encoding="utf-8"),
                        "publishedAt": "2026-07-09T19:00:00Z",
                        "isDraft": False,
                        "isPrerelease": existing_prerelease,
                        "targetCommitish": "master",
                        "url": "https://example.invalid/release",
                    }
                ),
                encoding="utf-8",
            )
        fake_binary_directory = self.root / "release-bin"
        fake_binary_directory.mkdir()
        fake_gh = fake_binary_directory / "gh"
        fake_gh.write_text(
            r"""#!/usr/bin/env python3
import json
import os
import pathlib
import sys

arguments = sys.argv[1:]
state = pathlib.Path(os.environ["FAKE_RELEASE_STATE"])
log = pathlib.Path(os.environ["FAKE_GH_COMMAND_LOG"])
if arguments[:2] == ["release", "view"]:
    if not state.exists():
        raise SystemExit(1)
    print(state.read_text(encoding="utf-8"))
    raise SystemExit(0)
if arguments[:2] not in (["release", "create"], ["release", "edit"]):
    raise SystemExit(f"unexpected gh command: {arguments}")
with log.open("a", encoding="utf-8") as handle:
    handle.write(" ".join(arguments) + "\n")
version = arguments[2]
notes_path = pathlib.Path(arguments[arguments.index("--notes-file") + 1])
title = arguments[arguments.index("--title") + 1]
prerelease = "--prerelease" in arguments or "--prerelease=true" in arguments
state.write_text(
    json.dumps(
        {
            "tagName": version,
            "name": title,
            "body": notes_path.read_text(encoding="utf-8"),
            "publishedAt": "2026-07-09T19:00:00Z",
            "isDraft": False,
            "isPrerelease": prerelease,
            "targetCommitish": "master",
            "url": "https://example.invalid/release",
        }
    ),
    encoding="utf-8",
)
""",
            encoding="utf-8",
        )
        fake_gh.chmod(0o755)
        environment = os.environ.copy()
        environment["PATH"] = f"{fake_binary_directory}{os.pathsep}{environment['PATH']}"
        environment["FAKE_RELEASE_STATE"] = str(state)
        environment["FAKE_GH_COMMAND_LOG"] = str(command_log)
        result = self.command(
            str(HELPER),
            "publish-release",
            "--version",
            version,
            "--notes-file",
            str(notes),
            cwd=self.repo,
            check=False,
            env=environment,
        )
        return result, command_log.read_text(encoding="utf-8") if command_log.exists() else ""


if __name__ == "__main__":
    unittest.main()
