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
PREPARE_PAGES = SKILL_ROOT / "scripts" / "prepare_pages_artifact.sh"
DEPLOY_PAGES = SKILL_ROOT / "scripts" / "deploy_pages_artifact.sh"
CONTAINER_MANIFEST_DIGEST = SKILL_ROOT / "scripts" / "resolve_container_manifest_digest.sh"
PUBLISH_CONTAINERS = SKILL_ROOT / "scripts" / "publish_container_artifacts.sh"
RELEASE_ENVIRONMENT_KEYS = (
    "RELEASE_ARTIFACT_TARGETS",
    "RELEASE_VERSION",
    "RELEASE_TIMESTAMP",
    "MOBILE_RELEASE_TIMESTAMP",
    "RELEASE_ARTIFACT_DIR",
    "RELEASE_BUMP",
    "RELEASE_HELPER",
    "RELEASE_PIPELINE",
)


class ReleasePipelineTest(unittest.TestCase):
    def setUp(self) -> None:
        self.original_gh_repo = os.environ.get("GH_REPO")
        os.environ["GH_REPO"] = "example/release-fixture"
        self.original_release_environment = {key: os.environ.get(key) for key in RELEASE_ENVIRONMENT_KEYS}
        for key in RELEASE_ENVIRONMENT_KEYS:
            os.environ.pop(key, None)
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
        for key, value in self.original_release_environment.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value

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

    def git_private_path(self, name: str) -> pathlib.Path:
        raw_path = self.command("git", "rev-parse", "--git-path", name, cwd=self.repo).stdout.strip()
        path = pathlib.Path(raw_path)
        return path if path.is_absolute() else self.repo / path

    def artifact_snapshot(self, artifact_directory: pathlib.Path) -> dict[str, bytes]:
        return {
            path.relative_to(artifact_directory).as_posix(): path.read_bytes()
            for path in artifact_directory.rglob("*")
            if path.is_file()
        }

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

    def test_prepare_exact_retry_reuses_sealed_release_without_ci_or_mutation(self) -> None:
        ci_log = self.root / "release-ci.log"
        (self.repo / "Makefile").write_text(
            "ci:\n\t@printf 'ci\\n' >> \"$$FIXTURE_CI_LOG\"\n",
            encoding="utf-8",
        )
        self.command("git", "add", "Makefile", cwd=self.repo)
        self.command("git", "commit", "-m", "Record release CI calls", cwd=self.repo)
        self.command("git", "push", "origin", "master", cwd=self.repo)
        environment = os.environ.copy() | {
            "RELEASE_HELPER": str(HELPER),
            "FIXTURE_CI_LOG": str(ci_log),
        }

        first = self.command(
            str(PREPARE),
            "--version",
            "v1.0.0",
            cwd=self.repo,
            env=environment,
        )
        release_head = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        artifact_directory = self.git_private_path("mprlab-release")
        sealed_snapshot = self.artifact_snapshot(artifact_directory)

        second = self.command(
            str(PREPARE),
            cwd=self.repo,
            check=False,
            env=environment,
        )

        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        self.assertIn("Prepared v1.0.0", second.stdout)
        self.assertEqual(ci_log.read_text(encoding="utf-8").splitlines(), ["ci"])
        self.assertEqual(self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip(), release_head)
        self.assertEqual(self.artifact_snapshot(artifact_directory), sealed_snapshot)
        self.assertFalse(self.git_private_path("mprlab-release.pending").exists())

    def test_failed_new_preparation_preserves_the_previous_sealed_release(self) -> None:
        environment = os.environ.copy() | {"RELEASE_HELPER": str(HELPER)}
        self.command(str(PREPARE), "--version", "v1.0.0", cwd=self.repo, env=environment)
        artifact_directory = self.git_private_path("mprlab-release")
        sealed_snapshot = self.artifact_snapshot(artifact_directory)
        (self.repo / "Makefile").write_text(
            "ci:\n\t@true\n"
            "failing-artifact:\n"
            "\t@mkdir -p \"$$RELEASE_ARTIFACT_DIR/payloads/release-assets\"\n"
            "\t@printf 'partial\\n' > \"$$RELEASE_ARTIFACT_DIR/payloads/release-assets/partial.txt\"\n"
            "\t@false\n",
            encoding="utf-8",
        )
        self.command("git", "add", "Makefile", cwd=self.repo)
        self.command("git", "commit", "-m", "Add the next release change", cwd=self.repo)
        self.command("git", "push", "origin", "master", cwd=self.repo)
        failing_environment = environment | {"RELEASE_ARTIFACT_TARGETS": "failing-artifact"}

        result = self.command(
            str(PREPARE),
            "--version",
            "v1.0.1",
            cwd=self.repo,
            check=False,
            env=failing_environment,
        )

        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.artifact_snapshot(artifact_directory), sealed_snapshot)
        sealed_manifest = json.loads((artifact_directory / "manifest.json").read_text(encoding="utf-8"))
        self.assertEqual(sealed_manifest["version"], "v1.0.0")
        self.assertFalse(
            self.command(
                "git",
                "show-ref",
                "--verify",
                "refs/tags/v1.0.1",
                cwd=self.repo,
                check=False,
            ).returncode
            == 0
        )

    def test_prepare_resumes_an_interrupted_release_commit_from_pending_payloads(self) -> None:
        source_commit = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        pending_directory = self.git_private_path("mprlab-release.pending")
        self.command(
            str(HELPER),
            "initialize-release-artifact",
            "--version",
            "v1.0.0",
            "--source-commit",
            source_commit,
            "--release-timestamp",
            "2026-07-28T12:00:00-07:00",
            "--artifact-dir",
            str(pending_directory),
            cwd=self.repo,
        )
        notes = self.root / "interrupted-notes.md"
        notes.write_text("## [v1.0.0] - 2026-07-28\n\n- Initial\n", encoding="utf-8")
        self.command(
            str(HELPER),
            "insert-changelog",
            "--notes-file",
            str(notes),
            cwd=self.repo,
        )
        self.command("git", "add", "CHANGELOG.md", cwd=self.repo)
        self.command("git", "commit", "-m", "Release v1.0.0", cwd=self.repo)
        interrupted_head = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        environment = os.environ.copy() | {"RELEASE_HELPER": str(HELPER)}

        result = self.command(str(PREPARE), cwd=self.repo, check=False, env=environment)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Resuming v1.0.0", result.stdout)
        self.assertEqual(self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip(), interrupted_head)
        self.assertEqual(
            self.command("git", "rev-parse", "v1.0.0^{}", cwd=self.repo).stdout.strip(),
            interrupted_head,
        )
        manifest = json.loads(
            (self.git_private_path("mprlab-release") / "manifest.json").read_text(encoding="utf-8")
        )
        self.assertEqual(manifest["version"], "v1.0.0")
        self.assertEqual(manifest["source_commit"], source_commit)
        self.assertFalse(pending_directory.exists())

    def test_prepare_runs_ci_without_release_artifact_environment(self) -> None:
        (self.repo / "Makefile").write_text(
            "\n".join(
                [
                    "ci:",
                    "\t@test -z \"$$RELEASE_ARTIFACT_TARGETS\"",
                    "\t@test -z \"$$RELEASE_VERSION\"",
                    "\t@test -z \"$$RELEASE_TIMESTAMP\"",
                    "\t@test -z \"$$MOBILE_RELEASE_TIMESTAMP\"",
                    "\t@test -z \"$$RELEASE_ARTIFACT_DIR\"",
                    "fixture-artifact:",
                    "\t@test \"$$RELEASE_ARTIFACT_TARGETS\" = \"fixture-artifact\"",
                    "\t@test \"$$RELEASE_VERSION\" = \"v1.0.0\"",
                    "\t@test -n \"$$RELEASE_TIMESTAMP\"",
                    "\t@test -n \"$$MOBILE_RELEASE_TIMESTAMP\"",
                    "\t@test -n \"$$RELEASE_ARTIFACT_DIR\"",
                    "",
                ]
            ),
            encoding="utf-8",
        )
        self.command("git", "add", "Makefile", cwd=self.repo)
        self.command("git", "commit", "-m", "Assert release CI environment", cwd=self.repo)
        self.command("git", "push", "origin", "master", cwd=self.repo)

        env = os.environ.copy()
        env["RELEASE_HELPER"] = str(HELPER)
        env["RELEASE_ARTIFACT_TARGETS"] = "fixture-artifact"
        env["RELEASE_VERSION"] = "v9.9.9"
        env["RELEASE_TIMESTAMP"] = "1999-01-01T00:00:00-07:00"
        env["MOBILE_RELEASE_TIMESTAMP"] = "1999-01-01T00:00:00-07:00"
        env["RELEASE_ARTIFACT_DIR"] = str(self.root / "ambient-artifact-dir")
        self.command(str(PREPARE), "--version", "v1.0.0", cwd=self.repo, env=env)

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

    def test_prepare_pages_artifact_writes_nojekyll_and_canonical_marker(self) -> None:
        source_commit = self.command("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        release_timestamp = "2026-07-10T12:00:00-07:00"
        self.command(
            str(HELPER),
            "initialize-release-artifact",
            "--version",
            "v1.0.0",
            "--source-commit",
            source_commit,
            "--release-timestamp",
            release_timestamp,
            cwd=self.repo,
        )
        artifact_dir = pathlib.Path(
            self.command("git", "rev-parse", "--git-path", "mprlab-release", cwd=self.repo).stdout.strip()
        )
        if not artifact_dir.is_absolute():
            artifact_dir = self.repo / artifact_dir
        site_directory = self.root / "pages-source"
        site_directory.mkdir()
        (site_directory / "index.html").write_text("release\n", encoding="utf-8")
        environment = os.environ.copy()
        environment["RELEASE_VERSION"] = "v1.0.0"
        environment["RELEASE_ARTIFACT_DIR"] = str(artifact_dir)

        result = self.command(
            str(PREPARE_PAGES),
            "--source",
            str(site_directory),
            cwd=self.repo,
            env=environment,
        )

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        with tarfile.open(artifact_dir / "payloads" / "release-assets" / "pages.tar.gz", "r:gz") as archive:
            members = {pathlib.PurePosixPath(member.name): member for member in archive.getmembers()}
            nojekyll_member = members[pathlib.PurePosixPath(".nojekyll")]
            nojekyll_file = archive.extractfile(nojekyll_member)
            self.assertIsNotNone(nojekyll_file)
            self.assertEqual(nojekyll_file.read(), b"")
            marker_member = members[pathlib.PurePosixPath(".mprlab-release.json")]
            marker_file = archive.extractfile(marker_member)
            self.assertIsNotNone(marker_file)
            self.assertEqual(
                json.load(marker_file),
                {
                    "release_timestamp": release_timestamp,
                    "release_version": "v1.0.0",
                    "schema_version": 1,
                    "source_commit": source_commit,
                },
            )

    def test_container_publication_reuses_exact_state_resumes_missing_indexes_and_rejects_conflicts(
        self,
    ) -> None:
        prepare_environment = os.environ.copy() | {"RELEASE_HELPER": str(HELPER)}
        self.command(
            str(PREPARE),
            "--version",
            "v1.0.0",
            cwd=self.repo,
            env=prepare_environment,
        )
        artifact_directory = self.git_private_path("mprlab-release")
        container_directory = artifact_directory / "payloads" / "containers" / "fixture"
        container_directory.mkdir(parents=True)
        platforms = (
            ("linux/amd64", "linux-amd64", "a"),
            ("linux/arm64", "linux-arm64", "b"),
        )
        descriptor_platforms = []
        for platform, token, digest_character in platforms:
            archive = container_directory / f"{token}.tar"
            archive.write_bytes(f"{platform}-archive\n".encode())
            descriptor_platforms.append(
                {
                    "platform": platform,
                    "token": token,
                    "local_ref": f"mprlab-release.local/fixture:v1.0.0-{token}",
                    "image_id": f"sha256:{digest_character * 64}",
                    "archive": archive.relative_to(artifact_directory).as_posix(),
                    "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
                }
            )
        descriptor = {
            "schema_version": 1,
            "artifact_kind": "mprlab.container",
            "name": "fixture",
            "image": "ghcr.io/example/fixture",
            "version": "v1.0.0",
            "platforms": descriptor_platforms,
        }
        descriptor_path = container_directory / "container.json"
        descriptor_path.write_text(json.dumps(descriptor, sort_keys=True), encoding="utf-8")
        manifest_path = artifact_directory / "manifest.json"
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        manifest["payloads"] = [
            {
                "path": path.relative_to(artifact_directory).as_posix(),
                "size": path.stat().st_size,
                "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
            }
            for path in sorted(container_directory.iterdir())
        ]
        manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")

        fake_binary_directory = self.root / "container-publish-bin"
        fake_binary_directory.mkdir()
        fake_docker = fake_binary_directory / "docker"
        fake_docker.write_text(
            r"""#!/usr/bin/env python3
import hashlib
import json
import os
import pathlib
import sys

arguments = sys.argv[1:]
state_directory = pathlib.Path(os.environ["FAKE_DOCKER_STATE"])
state_directory.mkdir(exist_ok=True)
with pathlib.Path(os.environ["FAKE_DOCKER_LOG"]).open("a", encoding="utf-8") as handle:
    handle.write(json.dumps(arguments) + "\n")

def state_paths(reference):
    token = hashlib.sha256(reference.encode()).hexdigest()
    return state_directory / f"{token}.json", state_directory / f"{token}.digest"

def read_manifest(reference):
    if os.environ.get("FAKE_DOCKER_INSPECTION_FAILURE_REFERENCE") == reference:
        print(f"registry transport unavailable: {reference}", file=sys.stderr)
        raise SystemExit(2)
    manifest_path, digest_path = state_paths(reference)
    if not manifest_path.exists():
        print(f"manifest unknown: {reference}", file=sys.stderr)
        raise SystemExit(1)
    return manifest_path.read_text(encoding="utf-8"), digest_path.read_text(encoding="utf-8").strip()

def write_manifest(reference, document):
    manifest_path, digest_path = state_paths(reference)
    raw = json.dumps(document, separators=(",", ":"), sort_keys=True) + "\n"
    manifest_path.write_text(raw, encoding="utf-8")
    digest_path.write_text(f"sha256:{hashlib.sha256(raw.encode()).hexdigest()}\n", encoding="utf-8")

def image_id(reference):
    return "sha256:" + ("a" if reference.endswith("linux-amd64") else "b") * 64

if arguments[:2] == ["buildx", "version"]:
    raise SystemExit(0)
if arguments[:1] == ["login"]:
    sys.stdin.read()
    raise SystemExit(0)
if arguments[:2] == ["image", "inspect"]:
    print(image_id(arguments[2]))
    raise SystemExit(0)
if arguments[:1] == ["load"]:
    raise SystemExit(0)
if arguments[:1] == ["tag"]:
    tag_map = state_directory / "tags.json"
    values = json.loads(tag_map.read_text(encoding="utf-8")) if tag_map.exists() else {}
    values[arguments[2]] = arguments[1]
    tag_map.write_text(json.dumps(values), encoding="utf-8")
    raise SystemExit(0)
if arguments[:1] == ["push"]:
    tag_map = json.loads((state_directory / "tags.json").read_text(encoding="utf-8"))
    reference = arguments[1]
    write_manifest(
        reference,
        {
            "schemaVersion": 2,
            "mediaType": "application/vnd.oci.image.manifest.v1+json",
            "config": {"digest": image_id(tag_map[reference])},
            "layers": [],
        },
    )
    print(f"pushed {reference}")
    raise SystemExit(0)
if arguments[:3] == ["buildx", "imagetools", "inspect"]:
    raw_requested = "--raw" in arguments
    reference = arguments[-1]
    raw, digest = read_manifest(reference)
    if raw_requested:
        print(raw, end="")
    else:
        print(f"Name: {reference}\nDigest: {digest}")
    raise SystemExit(0)
if arguments[:3] == ["buildx", "imagetools", "create"]:
    target = arguments[arguments.index("--tag") + 1]
    sources = arguments[arguments.index("--tag") + 2 :]
    manifests = []
    for source in sources:
        _, digest = read_manifest(source)
        architecture = "amd64" if source.endswith("linux-amd64") else "arm64"
        manifests.append(
            {
                "digest": digest,
                "mediaType": "application/vnd.oci.image.manifest.v1+json",
                "platform": {"os": "linux", "architecture": architecture},
            }
        )
    write_manifest(
        target,
        {
            "schemaVersion": 2,
            "mediaType": "application/vnd.oci.image.index.v1+json",
            "manifests": manifests,
        },
    )
    raise SystemExit(0)
raise SystemExit(f"unexpected docker command: {arguments}")
""",
            encoding="utf-8",
        )
        fake_docker.chmod(0o755)
        fake_gh = fake_binary_directory / "gh"
        fake_gh.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "if [[ \"$1 $2\" == \"api user\" ]]; then printf '%s\\n' release-test; "
            "elif [[ \"$1 $2\" == \"auth token\" ]]; then printf '%s\\n' fixture-token; "
            "else printf 'unexpected gh command: %s\\n' \"$*\" >&2; exit 2; fi\n",
            encoding="utf-8",
        )
        fake_gh.chmod(0o755)
        docker_state = self.root / "fake-docker-state"
        docker_log = self.root / "fake-docker.log"
        publication_environment = os.environ.copy() | {
            "PATH": f"{fake_binary_directory}{os.pathsep}{os.environ['PATH']}",
            "FAKE_DOCKER_STATE": str(docker_state),
            "FAKE_DOCKER_LOG": str(docker_log),
            "CONTAINER_REGISTRY_VERIFY_ATTEMPTS": "1",
            "CONTAINER_REGISTRY_VERIFY_DELAY_SECONDS": "1",
            "CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS": "5",
        }

        first = self.command(
            str(PUBLISH_CONTAINERS),
            cwd=self.repo,
            check=False,
            env=publication_environment,
        )
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        first_commands = list(map(json.loads, docker_log.read_text(encoding="utf-8").splitlines()))
        self.assertEqual(sum(command[:1] == ["push"] for command in first_commands), 2)
        self.assertEqual(
            sum(command[:3] == ["buildx", "imagetools", "create"] for command in first_commands),
            2,
        )

        second = self.command(
            str(PUBLISH_CONTAINERS),
            cwd=self.repo,
            check=False,
            env=publication_environment,
        )
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        second_commands = list(map(json.loads, docker_log.read_text(encoding="utf-8").splitlines()))
        self.assertEqual(sum(command[:1] == ["push"] for command in second_commands), 2)
        self.assertEqual(
            sum(command[:3] == ["buildx", "imagetools", "create"] for command in second_commands),
            2,
        )

        uncertain_reference = "ghcr.io/example/fixture:v1.0.0-linux-amd64"
        uncertain = self.command(
            str(PUBLISH_CONTAINERS),
            cwd=self.repo,
            check=False,
            env=publication_environment
            | {"FAKE_DOCKER_INSPECTION_FAILURE_REFERENCE": uncertain_reference},
        )
        self.assertNotEqual(uncertain.returncode, 0, uncertain.stdout + uncertain.stderr)
        self.assertIn(
            f"could not determine immutable container state for {uncertain_reference}",
            uncertain.stderr,
        )
        uncertain_commands = list(map(json.loads, docker_log.read_text(encoding="utf-8").splitlines()))
        self.assertEqual(sum(command[:1] == ["push"] for command in uncertain_commands), 2)

        for reference in ("ghcr.io/example/fixture:v1.0.0", "ghcr.io/example/fixture:latest"):
            token = hashlib.sha256(reference.encode()).hexdigest()
            (docker_state / f"{token}.json").unlink()
            (docker_state / f"{token}.digest").unlink()
        resumed = self.command(
            str(PUBLISH_CONTAINERS),
            cwd=self.repo,
            check=False,
            env=publication_environment,
        )
        self.assertEqual(resumed.returncode, 0, resumed.stdout + resumed.stderr)
        resumed_commands = list(map(json.loads, docker_log.read_text(encoding="utf-8").splitlines()))
        self.assertEqual(sum(command[:1] == ["push"] for command in resumed_commands), 2)
        self.assertEqual(
            sum(command[:3] == ["buildx", "imagetools", "create"] for command in resumed_commands),
            4,
        )

        conflicting_reference = "ghcr.io/example/fixture:v1.0.0-linux-amd64"
        conflicting_token = hashlib.sha256(conflicting_reference.encode()).hexdigest()
        conflicting_document = {
            "schemaVersion": 2,
            "mediaType": "application/vnd.oci.image.manifest.v1+json",
            "config": {"digest": f"sha256:{'c' * 64}"},
            "layers": [],
        }
        conflicting_raw = json.dumps(conflicting_document, separators=(",", ":"), sort_keys=True) + "\n"
        (docker_state / f"{conflicting_token}.json").write_text(conflicting_raw, encoding="utf-8")
        (docker_state / f"{conflicting_token}.digest").write_text(
            f"sha256:{hashlib.sha256(conflicting_raw.encode()).hexdigest()}\n",
            encoding="utf-8",
        )
        conflict = self.command(
            str(PUBLISH_CONTAINERS),
            cwd=self.repo,
            check=False,
            env=publication_environment,
        )
        self.assertNotEqual(conflict.returncode, 0, conflict.stdout + conflict.stderr)
        self.assertIn("immutable container platform conflict", conflict.stderr)
        conflict_commands = list(map(json.loads, docker_log.read_text(encoding="utf-8").splitlines()))
        self.assertEqual(sum(command[:1] == ["push"] for command in conflict_commands), 2)

    def test_container_manifest_digest_waits_for_registry_readiness(self) -> None:
        fake_binary_directory = self.root / "manifest-bin"
        fake_binary_directory.mkdir()
        fake_docker = fake_binary_directory / "docker"
        fake_docker.write_text(
            """#!/usr/bin/env bash
set -euo pipefail
[[ \"$1 $2 $3\" == \"buildx imagetools inspect\" ]] || exit 2
attempt=0
if [[ -f \"${FAKE_DOCKER_STATE}\" ]]; then attempt=\"$(cat \"${FAKE_DOCKER_STATE}\")\"; fi
attempt=$((attempt + 1))
printf '%s\\n' \"${attempt}\" >\"${FAKE_DOCKER_STATE}\"
if (( attempt <= FAKE_DOCKER_UNREADY_ATTEMPTS )); then
  echo 'simulated manifest is not available' >&2
  exit 255
fi
printf '%s\\n' 'Name: ghcr.io/example/image:latest' 'Digest:    sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
""",
            encoding="utf-8",
        )
        fake_docker.chmod(0o755)
        state_path = self.root / "docker-inspect-attempts"
        environment = os.environ.copy() | {
            "PATH": f"{fake_binary_directory}{os.pathsep}{os.environ['PATH']}",
            "FAKE_DOCKER_STATE": str(state_path),
            "FAKE_DOCKER_UNREADY_ATTEMPTS": "1",
            "CONTAINER_REGISTRY_VERIFY_ATTEMPTS": "2",
            "CONTAINER_REGISTRY_VERIFY_DELAY_SECONDS": "1",
        }

        result = self.command(
            "bash",
            str(CONTAINER_MANIFEST_DIGEST),
            "ghcr.io/example/image:latest",
            cwd=self.repo,
            check=False,
            env=environment,
        )

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            result.stdout.strip(),
            "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        )
        self.assertEqual(state_path.read_text(encoding="utf-8").strip(), "2")
        self.assertIn("Waiting for ghcr.io/example/image:latest manifest", result.stderr)
        self.assertIn("Docker exit 255", result.stderr)

    def test_container_manifest_digest_bounds_each_inspection_attempt(self) -> None:
        fake_binary_directory = self.root / "manifest-timeout-bin"
        fake_binary_directory.mkdir()
        fake_docker = fake_binary_directory / "docker"
        fake_docker.write_text("#!/usr/bin/env bash\nexit 97\n", encoding="utf-8")
        fake_docker.chmod(0o755)
        fake_timeout = fake_binary_directory / "timeout"
        fake_timeout.write_text(
            """#!/usr/bin/env bash
set -euo pipefail
printf '%s\\n' \"$*\" >>\"${FAKE_TIMEOUT_CAPTURE:?}\"
exit 124
""",
            encoding="utf-8",
        )
        fake_timeout.chmod(0o755)
        fake_sleep = fake_binary_directory / "sleep"
        fake_sleep.write_text(
            """#!/usr/bin/env bash
set -euo pipefail
printf '%s\\n' \"$*\" >>\"${FAKE_SLEEP_CAPTURE:?}\"
""",
            encoding="utf-8",
        )
        fake_sleep.chmod(0o755)
        timeout_capture_path = self.root / "timeout-arguments"
        sleep_capture_path = self.root / "sleep-arguments"
        environment = os.environ.copy() | {
            "PATH": f"{fake_binary_directory}{os.pathsep}{os.environ['PATH']}",
            "FAKE_TIMEOUT_CAPTURE": str(timeout_capture_path),
            "FAKE_SLEEP_CAPTURE": str(sleep_capture_path),
            "CONTAINER_REGISTRY_VERIFY_ATTEMPTS": "2",
            "CONTAINER_REGISTRY_VERIFY_DELAY_SECONDS": "1",
            "CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS": "1",
        }

        result = self.command(
            "bash",
            str(CONTAINER_MANIFEST_DIGEST),
            "ghcr.io/example/image:latest",
            cwd=self.repo,
            check=False,
            env=environment,
        )

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        expected_timeout_arguments = (
            "-k 1s -s SIGKILL 1s docker buildx imagetools inspect "
            "ghcr.io/example/image:latest"
        )
        self.assertEqual(
            timeout_capture_path.read_text(encoding="utf-8").splitlines(),
            [expected_timeout_arguments, expected_timeout_arguments],
        )
        self.assertEqual(sleep_capture_path.read_text(encoding="utf-8").splitlines(), ["1"])
        self.assertIn("Docker exit 124", result.stderr)
        self.assertIn("container manifest did not become readable for ghcr.io/example/image:latest", result.stderr)

    def test_container_manifest_digest_rejects_invalid_attempt_timeout(self) -> None:
        fake_binary_directory = self.root / "manifest-invalid-timeout-bin"
        fake_binary_directory.mkdir()
        fake_docker = fake_binary_directory / "docker"
        fake_docker.write_text("#!/usr/bin/env bash\nexit 2\n", encoding="utf-8")
        fake_docker.chmod(0o755)
        environment = os.environ.copy() | {
            "PATH": f"{fake_binary_directory}{os.pathsep}{os.environ['PATH']}",
            "CONTAINER_REGISTRY_VERIFY_ATTEMPTS": "1",
            "CONTAINER_REGISTRY_VERIFY_DELAY_SECONDS": "1",
            "CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS": "0",
        }

        result = self.command(
            "bash",
            str(CONTAINER_MANIFEST_DIGEST),
            "ghcr.io/example/image:latest",
            cwd=self.repo,
            check=False,
            env=environment,
        )

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS must be a positive integer", result.stderr)

    def test_container_manifest_digest_reports_docker_failure_with_image_context(self) -> None:
        fake_binary_directory = self.root / "manifest-error-bin"
        fake_binary_directory.mkdir()
        fake_docker = fake_binary_directory / "docker"
        fake_docker.write_text(
            """#!/usr/bin/env bash
set -euo pipefail
[[ \"$1 $2 $3\" == \"buildx imagetools inspect\" ]] || exit 2
echo 'simulated registry inspection failure' >&2
exit 255
""",
            encoding="utf-8",
        )
        fake_docker.chmod(0o755)
        environment = os.environ.copy() | {
            "PATH": f"{fake_binary_directory}{os.pathsep}{os.environ['PATH']}",
            "CONTAINER_REGISTRY_VERIFY_ATTEMPTS": "1",
            "CONTAINER_REGISTRY_VERIFY_DELAY_SECONDS": "1",
        }

        result = self.command(
            "bash",
            str(CONTAINER_MANIFEST_DIGEST),
            "ghcr.io/example/image:latest",
            cwd=self.repo,
            check=False,
            env=environment,
        )

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("simulated registry inspection failure", result.stderr)
        self.assertIn("container manifest did not become readable for ghcr.io/example/image:latest", result.stderr)

    def test_pages_deploy_verifies_prepared_source_commit(self) -> None:
        source_commit, environment = self.pages_release_fixture()
        release_commit = self.command("git", "rev-parse", "v1.0.0^{}", cwd=self.repo).stdout.strip()
        public_marker = json.loads(pathlib.Path(environment["FAKE_MARKER_PATH"]).read_text(encoding="utf-8"))
        self.assertNotEqual(source_commit, release_commit)
        self.assertEqual(public_marker["source_commit"], source_commit)
        self.assertNotEqual(public_marker["source_commit"], release_commit)
        result = self.deploy_pages(environment)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"Verified https://pages.example.invalid/ at source {source_commit}.", result.stdout)

    def test_pages_deploy_waits_for_newest_matching_pages_build_before_marker(self) -> None:
        source_commit, environment = self.pages_release_fixture()
        build_counter = self.root / "pages-build-counter"
        environment |= {
            "FAKE_PAGES_PREVIOUS_BUILD_STATUS": "built",
            "FAKE_PAGES_BUILD_STATES": "queued,built",
            "FAKE_PAGES_BUILD_COUNTER": str(build_counter),
            "PAGES_BUILD_VERIFY_ATTEMPTS": "2",
            "PAGES_BUILD_VERIFY_DELAY_SECONDS": "1",
        }

        result = self.deploy_pages(environment)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(build_counter.read_text(encoding="utf-8").strip(), "2")
        self.assertIn("Waiting for GitHub Pages build", result.stdout)
        self.assertIn(f"Verified https://pages.example.invalid/ at source {source_commit}.", result.stdout)

    def test_pages_deploy_requests_only_one_replacement_for_persistent_build_error(self) -> None:
        _, environment = self.pages_release_fixture()
        command_log = self.root / "pages-failed-build-gh.log"
        environment |= {
            "FAKE_PAGES_PREVIOUS_BUILD_STATUS": "built",
            "FAKE_PAGES_BUILD_STATES": "errored",
            "FAKE_PAGES_BUILD_ERROR": "simulated Pages build failure",
            "FAKE_GH_COMMAND_LOG": str(command_log),
            "PAGES_BUILD_VERIFY_ATTEMPTS": "2",
            "PAGES_BUILD_VERIFY_DELAY_SECONDS": "1",
        }

        result = self.deploy_pages(environment)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("GitHub Pages build did not recover for branch commit", result.stderr)
        self.assertIn("simulated Pages build failure", result.stderr)
        self.assertEqual(command_log.read_text(encoding="utf-8").count("--method POST"), 1)
        self.assertTrue(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_resumes_after_one_failed_build_replacement(self) -> None:
        source_commit, environment = self.pages_release_fixture()
        command_log = self.root / "pages-recovered-build-gh.log"
        build_counter = self.root / "pages-recovered-build-counter"
        environment |= {
            "FAKE_PAGES_BUILD_STATES": "errored,built",
            "FAKE_PAGES_BUILD_ERROR": "simulated Pages build failure",
            "FAKE_GH_COMMAND_LOG": str(command_log),
            "FAKE_PAGES_BUILD_COUNTER": str(build_counter),
            "PAGES_BUILD_VERIFY_ATTEMPTS": "2",
            "PAGES_BUILD_VERIFY_DELAY_SECONDS": "1",
        }

        result = self.deploy_pages(environment)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(command_log.read_text(encoding="utf-8").count("--method POST"), 1)
        self.assertIn(f"Verified https://pages.example.invalid/ at source {source_commit}.", result.stdout)

    def test_pages_deploy_rejects_built_pages_build_for_another_commit(self) -> None:
        _, environment = self.pages_release_fixture()
        environment |= {
            "FAKE_PAGES_BUILD_COMMIT": "0" * 40,
            "PAGES_BUILD_VERIFY_ATTEMPTS": "1",
            "PAGES_BUILD_VERIFY_DELAY_SECONDS": "1",
        }

        result = self.deploy_pages(environment)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("GitHub Pages build did not reach built state for branch commit", result.stderr)
        self.assertTrue(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_uses_cache_distinct_public_marker_request(self) -> None:
        source_commit, environment = self.pages_release_fixture()
        curl_log = self.root / "pages-curl.log"
        environment["FAKE_CURL_COMMAND_LOG"] = str(curl_log)

        result = self.deploy_pages(environment)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(f".mprlab-release.json?source_commit={source_commit}", curl_log.read_text(encoding="utf-8"))

    def test_pages_deploy_does_not_request_a_duplicate_build_for_matching_site_settings(self) -> None:
        _, environment = self.pages_release_fixture()
        command_log = self.root / "pages-gh.log"
        environment |= {
            "FAKE_GH_COMMAND_LOG": str(command_log),
            "FAKE_PAGES_SITE_MATCHES": "true",
        }

        result = self.deploy_pages(environment, configure=True)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        gh_commands = command_log.read_text(encoding="utf-8")
        self.assertNotIn("--method PUT", gh_commands)
        self.assertNotIn("--method POST", gh_commands)

    def test_pages_deploy_exact_retry_reuses_branch_and_active_build(self) -> None:
        _, environment = self.pages_release_fixture()
        first = self.deploy_pages(environment)
        first_branch_commit = self.command(
            "git",
            "rev-parse",
            "refs/heads/gh-pages",
            cwd=self.remote,
            git_dir=True,
        ).stdout.strip()
        command_log = self.root / "pages-exact-retry-gh.log"
        build_counter = self.root / "pages-exact-retry-build-counter"
        retry_environment = environment | {
            "FAKE_GH_COMMAND_LOG": str(command_log),
            "FAKE_PAGES_BUILD_STATES": "building,built",
            "FAKE_PAGES_BUILD_COUNTER": str(build_counter),
            "PAGES_BUILD_VERIFY_ATTEMPTS": "2",
            "PAGES_BUILD_VERIFY_DELAY_SECONDS": "1",
        }

        second = self.deploy_pages(retry_environment)

        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        self.assertIn("Pages branch already contains v1.0.0.", second.stdout)
        self.assertIn("matching build is active", second.stdout)
        self.assertNotIn("--method POST", command_log.read_text(encoding="utf-8"))
        self.assertEqual(
            self.command(
                "git",
                "rev-parse",
                "refs/heads/gh-pages",
                cwd=self.remote,
                git_dir=True,
            ).stdout.strip(),
            first_branch_commit,
        )

    def test_pages_deploy_requests_one_missing_build_and_then_reuses_it(self) -> None:
        _, environment = self.pages_release_fixture()
        first = self.deploy_pages(environment)
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        pages_commit = self.command(
            "git",
            "rev-parse",
            "refs/heads/gh-pages",
            cwd=self.remote,
            git_dir=True,
        ).stdout.strip()
        command_log = self.root / "pages-missing-build-gh.log"
        build_counter = self.root / "pages-missing-build-counter"
        retry_environment = environment | {
            "FAKE_GH_COMMAND_LOG": str(command_log),
            "FAKE_PAGES_BUILD_STATES": "built,built",
            "FAKE_PAGES_BUILD_COMMITS": f"{'0' * 40},{pages_commit}",
            "FAKE_PAGES_BUILD_COUNTER": str(build_counter),
            "PAGES_BUILD_VERIFY_ATTEMPTS": "2",
            "PAGES_BUILD_VERIFY_DELAY_SECONDS": "1",
        }

        result = self.deploy_pages(retry_environment)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Requesting the missing GitHub Pages build", result.stdout)
        self.assertEqual(command_log.read_text(encoding="utf-8").count("--method POST"), 1)

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
                "FAKE_PAGES_REMOTE": str(first_push_remote),
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

    def test_pages_deploy_rejects_invalid_marker_schema_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture(marker_schema_version=2)
        result = self.deploy_pages(environment)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("release marker has an invalid contract", result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_rejects_invalid_marker_version_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture(marker_release_version="v1.0.1")
        result = self.deploy_pages(environment)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("release marker has the wrong release version", result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_rejects_archive_without_nojekyll_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture(include_nojekyll=False)
        result = self.deploy_pages(environment)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Pages asset has no .nojekyll marker", result.stderr)
        self.assertFalse(self.remote_branch_exists("gh-pages"), result.stdout + result.stderr)

    def test_pages_deploy_rejects_invalid_public_marker_contract(self) -> None:
        source_commit, environment = self.pages_release_fixture()
        environment["PAGES_VERIFY_ATTEMPTS"] = "1"
        environment["PAGES_VERIFY_DELAY_SECONDS"] = "1"
        marker_path = pathlib.Path(environment["FAKE_MARKER_PATH"])
        scenarios = (
            ("schema", {"schema_version": 2}),
            ("version", {"release_version": "v1.0.1"}),
            ("source", {"source_commit": "0" * 40}),
        )
        for scenario_name, replacement in scenarios:
            with self.subTest(scenario=scenario_name):
                marker = {
                    "schema_version": 1,
                    "release_version": "v1.0.0",
                    "source_commit": source_commit,
                    "release_timestamp": "2026-07-09T12:00:00-07:00",
                }
                marker.update(replacement)
                marker_path.write_text(json.dumps(marker), encoding="utf-8")
                result = self.deploy_pages(environment)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Pages marker did not reach source", result.stderr)

    def test_pages_deploy_validates_retry_settings_before_remote_mutation(self) -> None:
        _, environment = self.pages_release_fixture()
        invalid_settings = (
            {"PAGES_VERIFY_ATTEMPTS": "1+1", "PAGES_VERIFY_DELAY_SECONDS": "1"},
            {"PAGES_VERIFY_ATTEMPTS": "1", "PAGES_VERIFY_DELAY_SECONDS": "0"},
            {"PAGES_BUILD_VERIFY_ATTEMPTS": "1+1", "PAGES_BUILD_VERIFY_DELAY_SECONDS": "1"},
            {"PAGES_BUILD_VERIFY_ATTEMPTS": "1", "PAGES_BUILD_VERIFY_DELAY_SECONDS": "0"},
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

    def test_publish_rejects_conflicting_prerelease_state(self) -> None:
        result, command_log = self.publish_release_fixture("v1.2.3-rc.1", existing_prerelease=False)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("existing GitHub Release conflicts", result.stdout)
        self.assertEqual(command_log, "")

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
assets = pathlib.Path(os.environ["FAKE_RELEASE_ASSET_DIR"])
assets.mkdir(exist_ok=True)
with pathlib.Path(os.environ["FAKE_GH_COMMAND_LOG"]).open("a", encoding="utf-8") as handle:
    handle.write(json.dumps(arguments) + "\n")
if arguments[:2] == ["pr", "list"] or arguments[:2] == ["run", "list"]:
    print("[]")
    raise SystemExit(0)
if arguments[:2] == ["release", "view"]:
    if not state.exists():
        raise SystemExit(1)
    if arguments[arguments.index("--json") + 1] == "assets":
        print(json.dumps({"assets": [{"name": path.name} for path in sorted(assets.iterdir())]}))
        raise SystemExit(0)
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
    for value in arguments[3:]:
        if value.startswith("--"):
            break
        source = pathlib.Path(value)
        (assets / source.name).write_bytes(source.read_bytes())
    raise SystemExit(0)
if arguments[:2] == ["release", "download"]:
    asset_name = arguments[arguments.index("--pattern") + 1]
    destination = pathlib.Path(arguments[arguments.index("--dir") + 1]) / asset_name
    destination.write_bytes((assets / asset_name).read_bytes())
    raise SystemExit(0)
raise SystemExit(f"unexpected gh command: {arguments}")
""",
            encoding="utf-8",
        )
        fake_gh.chmod(0o755)
        publish_environment = os.environ.copy()
        publish_environment["PATH"] = f"{fake_binary_directory}{os.pathsep}{publish_environment['PATH']}"
        publish_environment["FAKE_RELEASE_STATE"] = str(self.root / "fake-release-state.json")
        release_asset_directory = self.root / "fake-release-assets"
        publish_environment["FAKE_RELEASE_ASSET_DIR"] = str(release_asset_directory)
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
        repeated_result = self.command(
            str(HELPER),
            "publish-prepared-release",
            "--remote",
            "upstream",
            cwd=self.repo,
            check=False,
            env=publish_environment,
        )
        self.assertEqual(repeated_result.returncode, 0, repeated_result.stdout + repeated_result.stderr)
        logged_arguments = list(map(json.loads, command_log.read_text(encoding="utf-8").splitlines()))
        self.assertEqual(
            sum(arguments[:2] == ["release", "upload"] for arguments in logged_arguments),
            1,
        )
        (release_asset_directory / "manifest.json").write_text("conflict\n", encoding="utf-8")
        conflicting_result = self.command(
            str(HELPER),
            "publish-prepared-release",
            "--remote",
            "upstream",
            cwd=self.repo,
            check=False,
            env=publish_environment,
        )
        self.assertNotEqual(
            conflicting_result.returncode,
            0,
            conflicting_result.stdout + conflicting_result.stderr,
        )
        self.assertIn("does not match the prepared payload", conflicting_result.stdout)
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
        marker_schema_version: int = 1,
        marker_release_version: str = "v1.0.0",
        include_nojekyll: bool = True,
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
            "schema_version": marker_schema_version,
            "release_version": marker_release_version,
            "source_commit": marker_source_commit or source_commit,
            "release_timestamp": "2026-07-09T12:00:00-07:00",
        }
        marker_path = site_directory / ".mprlab-release.json"
        marker_path.write_text(json.dumps(marker), encoding="utf-8")
        nojekyll_path = site_directory / ".nojekyll"
        nojekyll_path.write_text("", encoding="utf-8")
        (site_directory / "index.html").write_text("release\n", encoding="utf-8")
        archive_path = release_directory / "pages.tar.gz"
        with tarfile.open(archive_path, "w:gz") as archive:
            archive.add(marker_path, arcname=".mprlab-release.json")
            if include_nojekyll:
                archive.add(nojekyll_path, arcname=".nojekyll")
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
if [[ "$1" == "api" ]]; then
  if [[ "$*" == *"/pages/builds?per_page=100"* ]]; then
    build_index=0
    if [[ -n "${FAKE_PAGES_BUILD_COUNTER:-}" ]]; then
      if [[ -f "${FAKE_PAGES_BUILD_COUNTER}" ]]; then build_index="$(cat "${FAKE_PAGES_BUILD_COUNTER}")"; fi
      printf '%s\n' "$((build_index + 1))" >"${FAKE_PAGES_BUILD_COUNTER}"
    fi
    IFS=, read -r -a build_states <<<"${FAKE_PAGES_BUILD_STATES:-built}"
    if (( build_index >= ${#build_states[@]} )); then build_index=$((${#build_states[@]} - 1)); fi
    build_status="${build_states[build_index]}"
    default_build_commit="$(git --git-dir="${FAKE_PAGES_REMOTE}" rev-parse refs/heads/gh-pages)"
    IFS=, read -r -a build_commits <<<"${FAKE_PAGES_BUILD_COMMITS:-${FAKE_PAGES_BUILD_COMMIT:-${default_build_commit}}}"
    build_commit_index="${build_index}"
    if (( build_commit_index >= ${#build_commits[@]} )); then build_commit_index=$((${#build_commits[@]} - 1)); fi
    build_commit="${build_commits[build_commit_index]}"
    build_error='null'
    if [[ "${build_status}" == "errored" ]]; then build_error="\\\"${FAKE_PAGES_BUILD_ERROR:-simulated Pages build failure}\\\""; fi
    build_created_at="${FAKE_PAGES_BUILD_CREATED_AT:-2026-07-21T20:43:32Z}"
    previous_build_status="${FAKE_PAGES_PREVIOUS_BUILD_STATUS:-}"
    if [[ -n "${previous_build_status}" ]]; then
      previous_build_error='null'
      if [[ "${previous_build_status}" == "errored" ]]; then previous_build_error="\\\"${FAKE_PAGES_PREVIOUS_BUILD_ERROR:-simulated stale Pages build failure}\\\""; fi
      previous_build_created_at="${FAKE_PAGES_PREVIOUS_BUILD_CREATED_AT:-2026-07-21T20:43:31Z}"
      printf '[{\"commit\":\"%s\",\"status\":\"%s\",\"created_at\":\"%s\",\"error\":{\"message\":%s}},' "${build_commit}" "${previous_build_status}" "${previous_build_created_at}" "${previous_build_error}"
    else
      printf '['
    fi
    printf '{\"commit\":\"%s\",\"status\":\"%s\",\"created_at\":\"%s\",\"error\":{\"message\":%s}}]\n' "${build_commit}" "${build_status}" "${build_created_at}" "${build_error}"
  elif [[ "${FAKE_PAGES_SITE_MATCHES:-}" == "true" ]]; then
    printf '%s\n' '{"source":{"branch":"gh-pages","path":"/"},"https_enforced":true}'
  else
    printf '%s\n' '{}'
  fi
  exit 0
fi
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
        fake_curl.write_text(
            "#!/usr/bin/env bash\nset -euo pipefail\nif [[ -n \"${FAKE_CURL_COMMAND_LOG:-}\" ]]; then printf '%s\\n' \"$*\" >>\"${FAKE_CURL_COMMAND_LOG}\"; fi\ncat \"${FAKE_MARKER_PATH}\"\n",
            encoding="utf-8",
        )
        fake_curl.chmod(0o755)

        environment = os.environ.copy()
        environment["PATH"] = f"{fake_binary_directory}{os.pathsep}{environment['PATH']}"
        environment["FAKE_RELEASE_DIR"] = str(release_directory)
        environment["FAKE_MARKER_PATH"] = str(marker_path)
        environment["FAKE_PAGES_REMOTE"] = str(self.remote)
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
