from __future__ import annotations

import hashlib
import json
import os
import pathlib
import shutil
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
        self.original_gh_repo = os.environ.get("GH_REPO")
        os.environ["GH_REPO"] = "example/release-fixture"
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
        if self.original_gh_repo is None:
            os.environ.pop("GH_REPO", None)
        else:
            os.environ["GH_REPO"] = self.original_gh_repo

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

    def test_pages_deploy_scopes_release_download_to_selected_remote_repository(self) -> None:
        _, environment = self.pages_release_fixture()
        github_remote_url = "https://github.com/example/pages-target.git"
        self.command("git", "config", f"url.{self.remote}.insteadOf", github_remote_url, cwd=self.repo)
        self.command("git", "remote", "set-url", "origin", github_remote_url, cwd=self.repo)
        command_log = self.root / "pages-gh-commands.log"
        environment["FAKE_GH_COMMAND_LOG"] = str(command_log)
        environment["PAGES_VERIFY_ATTEMPTS"] = "1"
        environment["PAGES_VERIFY_DELAY_SECONDS"] = "1"

        result = self.deploy_pages(environment, configure=True)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        gh_commands = command_log.read_text(encoding="utf-8")
        self.assertIn("--repo example/pages-target", gh_commands)
        self.assertIn("repos/example/pages-target/pages", gh_commands)
        self.assertNotIn("repos/{owner}/{repo}/pages", gh_commands)

    def test_pages_deploy_requires_curl_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture()
        restricted_binary_directory = self.root / "preflight-bin"
        restricted_binary_directory.mkdir()
        for command_name in ("bash", "git", "python3", "tar"):
            command_path = shutil.which(command_name)
            self.assertIsNotNone(command_path)
            (restricted_binary_directory / command_name).symlink_to(command_path)
        (restricted_binary_directory / "gh").symlink_to(pathlib.Path(environment["PATH"].split(os.pathsep)[0]) / "gh")
        environment["PATH"] = str(restricted_binary_directory)

        result = self.deploy_pages(environment)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("curl is required", result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_requires_sleep_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture()
        restricted_binary_directory = self.root / "sleep-preflight-bin"
        restricted_binary_directory.mkdir()
        fixture_binary_directory = pathlib.Path(environment["PATH"].split(os.pathsep)[0])
        for command_name in ("bash", "git", "python3", "tar"):
            command_path = shutil.which(command_name)
            self.assertIsNotNone(command_path)
            (restricted_binary_directory / command_name).symlink_to(command_path)
        for command_name in ("gh", "curl"):
            (restricted_binary_directory / command_name).symlink_to(fixture_binary_directory / command_name)
        environment["PATH"] = str(restricted_binary_directory)

        result = self.deploy_pages(environment)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("sleep is required", result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_rejects_mismatched_push_repository_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture()
        push_remote = self.root / "push-target.git"
        self.command("git", "init", "--bare", str(push_remote), cwd=self.root)
        fetch_url = "https://github.com/example/pages-target.git"
        push_url = "git@github.com:example/other-target.git"
        self.command("git", "config", f"url.{self.remote}.insteadOf", fetch_url, cwd=self.repo)
        self.command("git", "config", f"url.{push_remote}.insteadOf", push_url, cwd=self.repo)
        self.command("git", "remote", "set-url", "origin", fetch_url, cwd=self.repo)
        self.command("git", "remote", "set-url", "--push", "origin", push_url, cwd=self.repo)
        environment["PAGES_VERIFY_ATTEMPTS"] = "1"
        environment["PAGES_VERIFY_DELAY_SECONDS"] = "1"

        result = self.deploy_pages(environment)

        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)
        push_branch = self.command(
            "git",
            "show-ref",
            "--verify",
            "refs/heads/gh-pages",
            cwd=push_remote,
            git_dir=True,
            check=False,
        )
        self.assertNotEqual(push_branch.returncode, 0, result.stdout + result.stderr)

    def test_pages_deploy_rejects_push_instead_of_repositories_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture()
        with tempfile.TemporaryDirectory(prefix="llm-proxy-pages-push-", dir="/tmp") as push_directory:
            parseable_push_remote = pathlib.Path(push_directory)
            unscopable_push_remote = self.root / "unscopable-push-target.git"
            scenarios = (
                (
                    "parseable",
                    "example/pages-target",
                    parseable_push_remote,
                    f"file://localhost{parseable_push_remote}",
                ),
                (
                    "unscopable",
                    "example/pages-target-local",
                    unscopable_push_remote,
                    str(unscopable_push_remote),
                ),
            )
            for scenario_name, github_repository, push_remote, push_url in scenarios:
                with self.subTest(scenario=scenario_name):
                    fetch_url = f"https://github.com/{github_repository}.git"
                    self.command("git", "init", "--bare", str(push_remote), cwd=self.root)
                    self.command("git", "config", f"url.{self.remote}.insteadOf", fetch_url, cwd=self.repo)
                    self.command("git", "config", f"url.{push_url}.pushInsteadOf", fetch_url, cwd=self.repo)
                    self.command("git", "remote", "set-url", "origin", fetch_url, cwd=self.repo)
                    configured_push_url = self.command(
                        "git",
                        "config",
                        "--get",
                        "remote.origin.pushurl",
                        cwd=self.repo,
                        check=False,
                    )
                    self.assertNotEqual(configured_push_url.returncode, 0)
                    self.assertEqual(
                        self.command("git", "remote", "get-url", "--push", "origin", cwd=self.repo).stdout.strip(),
                        push_url,
                    )
                    command_log = self.root / f"{scenario_name}-push-instead-of-pages-gh-commands.log"
                    test_environment = environment | {
                        "FAKE_GH_COMMAND_LOG": str(command_log),
                        "GH_REPO": github_repository,
                    }

                    result = self.deploy_pages(test_environment)

                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertIn(
                        "selected Git remote fetch and push URLs identify different GitHub repositories",
                        result.stderr,
                    )
                    self.assertFalse(command_log.exists(), result.stdout + result.stderr)
                    self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)
                    push_branch = self.command(
                        "git",
                        "show-ref",
                        "--verify",
                        "refs/heads/gh-pages",
                        cwd=push_remote,
                        git_dir=True,
                        check=False,
                    )
                    self.assertNotEqual(push_branch.returncode, 0, result.stdout + result.stderr)

    def test_pages_deploy_pushes_to_once_resolved_push_instead_of_repository(self) -> None:
        source_commit, environment = self.pages_release_fixture()
        with (
            tempfile.TemporaryDirectory(prefix="llm-proxy-pages-first-push-", dir="/tmp") as first_directory,
            tempfile.TemporaryDirectory(prefix="llm-proxy-pages-second-push-", dir="/tmp") as second_directory,
        ):
            first_push_remote = pathlib.Path(first_directory)
            second_push_remote = pathlib.Path(second_directory)
            first_push_url = f"file://localhost{first_push_remote}"
            second_push_url = f"file://localhost{second_push_remote}"
            fetch_url = f"https://localhost{first_push_remote}"
            self.command("git", "init", "--bare", str(first_push_remote), cwd=self.root)
            self.command("git", "init", "--bare", str(second_push_remote), cwd=self.root)
            self.command("git", "config", f"url.{self.remote}.insteadOf", fetch_url, cwd=self.repo)
            self.command("git", "remote", "set-url", "origin", fetch_url, cwd=self.repo)
            global_config = self.root / "chained-push-instead-of.config"
            self.command(
                "git",
                "config",
                "--file",
                str(global_config),
                f"url.{first_push_url}.pushInsteadOf",
                fetch_url,
                cwd=self.root,
            )
            self.command(
                "git",
                "config",
                "--file",
                str(global_config),
                f"url.{second_push_url}.pushInsteadOf",
                first_push_url,
                cwd=self.root,
            )
            test_environment = environment | {
                "GIT_CONFIG_GLOBAL": str(global_config),
                "GIT_CONFIG_NOSYSTEM": "1",
            }
            self.assertEqual(
                self.command(
                    "git",
                    "remote",
                    "get-url",
                    "--push",
                    "origin",
                    cwd=self.repo,
                    env=test_environment,
                ).stdout.strip(),
                first_push_url,
            )

            result = self.deploy_pages(test_environment)

            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn(f"Verified https://pages.example.invalid/ at source {source_commit}.", result.stdout)
            self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)
            first_push_branch = self.command(
                "git",
                "show-ref",
                "--verify",
                "refs/heads/gh-pages",
                cwd=first_push_remote,
                git_dir=True,
                check=False,
            )
            self.assertEqual(first_push_branch.returncode, 0, result.stdout + result.stderr)
            second_push_branch = self.command(
                "git",
                "show-ref",
                "--verify",
                "refs/heads/gh-pages",
                cwd=second_push_remote,
                git_dir=True,
                check=False,
            )
            self.assertNotEqual(second_push_branch.returncode, 0, result.stdout + result.stderr)

    def test_pages_deploy_rejects_noncanonical_version_before_github_download(self) -> None:
        _, environment = self.pages_release_fixture()
        command_log = self.root / "invalid-pages-version-gh-commands.log"
        environment["FAKE_GH_COMMAND_LOG"] = str(command_log)

        result = self.deploy_pages(environment, version="v1.0.0-01")

        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(command_log.exists(), result.stdout + result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_rejects_unscopable_remote_before_github_download(self) -> None:
        _, environment = self.pages_release_fixture()
        environment.pop("GH_REPO", None)
        command_log = self.root / "unscopable-pages-gh-commands.log"
        environment["FAKE_GH_COMMAND_LOG"] = str(command_log)

        result = self.deploy_pages(environment)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("cannot scope GitHub operations", result.stderr)
        self.assertFalse(command_log.exists(), result.stdout + result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

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
        environment["PAGES_VERIFY_ATTEMPTS"] = "1"
        environment["PAGES_VERIFY_DELAY_SECONDS"] = "1"
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

    def test_prepare_rejects_noncanonical_numeric_prerelease_identifiers(self) -> None:
        env = os.environ.copy()
        env["RELEASE_HELPER"] = str(HELPER)
        original_head = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        for version in ("v1.2.3-01", "v1.2.3-rc.01"):
            with self.subTest(version=version):
                result = self.command(
                    str(PREPARE),
                    "--dry-run",
                    "--version",
                    version,
                    cwd=self.repo,
                    check=False,
                    env=env,
                )
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip(), original_head)

    def test_prepare_rejects_existing_explicit_tag_without_mutation(self) -> None:
        self.command("git", "tag", "v1.0.0", cwd=self.repo)
        (self.repo / "README.md").write_text("# Fixture\n\nNext change.\n", encoding="utf-8")
        self.command("git", "add", "README.md", cwd=self.repo)
        self.command("git", "commit", "-m", "Next change", cwd=self.repo)
        original_head = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        env = os.environ.copy()
        env["RELEASE_HELPER"] = str(HELPER)

        result = self.command(
            str(PREPARE),
            "--version",
            "v1.0.0",
            cwd=self.repo,
            check=False,
            env=env,
        )

        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip(), original_head)
        self.assertEqual(self.command("git", "status", "--short", cwd=self.repo).stdout, "")

    def test_initialize_release_artifact_rejects_noncanonical_version(self) -> None:
        source_commit = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        result = self.command(
            str(HELPER),
            "initialize-release-artifact",
            "--version",
            "v1.0.0-01",
            "--source-commit",
            source_commit,
            "--release-timestamp",
            "2026-07-10T12:00:00-07:00",
            cwd=self.repo,
            check=False,
        )
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_publish_marks_prerelease_without_latest(self) -> None:
        result, command_log = self.publish_release_fixture("v1.2.3-rc.1")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("release create v1.2.3-rc.1", command_log)
        self.assertIn("--prerelease", command_log)
        self.assertNotIn("--latest", command_log)

    def test_publish_rejects_noncanonical_version_before_github_mutation(self) -> None:
        result, command_log = self.publish_release_fixture("v1.2.3-01")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(command_log, "")

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

    def test_publish_prepared_release_verifies_selected_remote_after_publish(self) -> None:
        prepare_environment = os.environ.copy()
        prepare_environment["RELEASE_HELPER"] = str(HELPER)
        self.command(str(PREPARE), "--version", "v1.0.0", cwd=self.repo, env=prepare_environment)
        self.command("git", "remote", "rename", "origin", "upstream", cwd=self.repo)
        github_remote_url = "https://github.com/example/upstream.git"
        self.command("git", "config", f"url.{self.remote}.insteadOf", github_remote_url, cwd=self.repo)
        self.command("git", "remote", "set-url", "upstream", github_remote_url, cwd=self.repo)
        unrelated_remote = self.root / "unrelated-origin.git"
        self.command("git", "init", "--bare", str(unrelated_remote), cwd=self.root)
        self.command("git", "remote", "add", "origin", str(unrelated_remote), cwd=self.repo)

        fake_binary_directory = self.root / "publish-selected-remote-bin"
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
with pathlib.Path(os.environ["FAKE_GH_COMMAND_LOG"]).open("a", encoding="utf-8") as handle:
    handle.write(json.dumps(arguments) + "\n")
if arguments[:2] == ["pr", "list"] or arguments[:2] == ["run", "list"]:
    print("[]")
    raise SystemExit(0)
if arguments[:2] == ["release", "view"]:
    if not state.exists():
        raise SystemExit(1)
    print(state.read_text(encoding="utf-8"))
    raise SystemExit(0)
if arguments[:2] == ["release", "create"]:
    notes_path = pathlib.Path(arguments[arguments.index("--notes-file") + 1])
    state.write_text(
        json.dumps(
            {
                "tagName": arguments[2],
                "name": arguments[arguments.index("--title") + 1],
                "body": notes_path.read_text(encoding="utf-8"),
                "publishedAt": "2026-07-09T19:00:00Z",
                "isDraft": False,
                "isPrerelease": "--prerelease" in arguments,
                "targetCommitish": "master",
                "url": "https://example.invalid/release",
            }
        ),
        encoding="utf-8",
    )
    raise SystemExit(0)
if arguments[:2] == ["release", "upload"]:
    raise SystemExit(0)
if arguments[:2] == ["release", "download"]:
    asset_name = arguments[arguments.index("--pattern") + 1]
    destination = pathlib.Path(arguments[arguments.index("--dir") + 1]) / asset_name
    destination.write_bytes((pathlib.Path.cwd() / ".git" / "mprlab-release" / asset_name).read_bytes())
    raise SystemExit(0)
raise SystemExit(f"unexpected gh command: {arguments}")
""",
            encoding="utf-8",
        )
        fake_gh.chmod(0o755)
        publish_environment = os.environ.copy()
        publish_environment["PATH"] = f"{fake_binary_directory}{os.pathsep}{publish_environment['PATH']}"
        publish_environment["FAKE_RELEASE_STATE"] = str(self.root / "fake-release-state.json")
        command_log = self.root / "publish-selected-remote-gh-commands.log"
        publish_environment["FAKE_GH_COMMAND_LOG"] = str(command_log)

        result = self.command(
            str(HELPER),
            "publish-prepared-release",
            "--remote",
            "upstream",
            cwd=self.repo,
            check=False,
            env=publish_environment,
        )

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        for arguments in map(json.loads, command_log.read_text(encoding="utf-8").splitlines()):
            if arguments[:1] in (["pr"], ["release"], ["run"]):
                self.assertIn("--repo", arguments, arguments)
                self.assertEqual(arguments[arguments.index("--repo") + 1], "example/upstream", arguments)
        self.assertFalse(
            self.command(
                "git",
                "show-ref",
                "--verify",
                "refs/tags/v1.0.0",
                cwd=unrelated_remote,
                git_dir=True,
                check=False,
            ).returncode
            == 0
        )

    def test_ci_tracks_repository_owned_release_tools(self) -> None:
        workflow = (REPOSITORY_ROOT / ".github" / "workflows" / "test.yml").read_text(encoding="utf-8")
        self.assertIn("      - 'tools/gitrelease/**'\n", workflow)

    def test_make_publish_release_forwards_selected_remote(self) -> None:
        result = self.command(
            "make",
            "--dry-run",
            "publish-release",
            "PUBLISH_REMOTE=upstream",
            cwd=REPOSITORY_ROOT,
        )
        self.assertIn('--remote "upstream"', result.stdout)

    def test_remote_preflight_does_not_require_gix(self) -> None:
        restricted_binary_directory = self.root / "preflight-without-gix-bin"
        restricted_binary_directory.mkdir()
        for command_name in ("bash", "git", "uv"):
            command_path = shutil.which(command_name)
            self.assertIsNotNone(command_path)
            (restricted_binary_directory / command_name).symlink_to(command_path)
        fake_gh = restricted_binary_directory / "gh"
        fake_gh.write_text(
            "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$1 $2\" == \"repo view\" ]]; then printf '%s\\n' '{\"defaultBranchRef\":{\"name\":\"master\"}}'; elif [[ \"$1 $2\" == \"pr list\" ]]; then printf '[]\\n'; else exit 2; fi\n",
            encoding="utf-8",
        )
        fake_gh.chmod(0o755)
        environment = os.environ.copy()
        environment["PATH"] = str(restricted_binary_directory)

        result = self.command(
            str(HELPER),
            "preflight",
            "--release-timestamp",
            "2026-07-10T12:00:00-07:00",
            cwd=self.repo,
            check=False,
            env=environment,
        )

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
if [[ -n "${FAKE_GH_COMMAND_LOG:-}" ]]; then printf '%s\n' "$*" >>"${FAKE_GH_COMMAND_LOG}"; fi
if [[ "$1" == "api" ]]; then exit 0; fi
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

    def deploy_pages(
        self,
        environment: dict[str, str],
        configure: bool = False,
        version: str = "v1.0.0",
    ) -> subprocess.CompletedProcess[str]:
        arguments = [
            "--remote",
            "origin",
            "--branch",
            "gh-pages",
            "--version",
            version,
            "--url",
            "https://pages.example.invalid/",
        ]
        if not configure:
            arguments.append("--skip-configure")
        return self.command(
            str(DEPLOY_PAGES),
            *arguments,
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
