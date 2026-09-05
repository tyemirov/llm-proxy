"""Exercise release metadata through real Python builds and installations."""

from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import tarfile

import pytest


REPOSITORY_ROOT = Path(__file__).resolve().parents[1]
UV = os.environ.get("UV", "uv")


def run(directory: Path, *arguments: str, input_text: str | None = None) -> str:
    result = subprocess.run(
        arguments, cwd=directory, input=input_text, text=True, capture_output=True,
        check=False,
    )
    assert result.returncode == 0, f"{arguments}:\n{result.stdout}\n{result.stderr}"
    return result.stdout.strip()


@pytest.fixture
def package_repository(tmp_path: Path) -> Path:
    root = tmp_path / "repository"
    root.mkdir()
    for name in run(REPOSITORY_ROOT, "git", "ls-files", "python").splitlines():
        destination = root / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes((REPOSITORY_ROOT / name).read_bytes())
    run(root, "git", "init", "--quiet")
    run(root, "git", "config", "user.email", "package-test@example.invalid")
    run(root, "git", "config", "user.name", "Package test")
    run(root, "git", "add", "python")
    run(root, "git", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Package source")
    run(root, "git", "tag", "v1.4.1")
    run(root, "git", "-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "Release change")
    return root


def assert_installed_version(directory: Path, requirement: str, expected: str) -> None:
    output = run(
        directory, UV, "run", "--no-project", "--with", requirement,
        "python", "-c",
        "from importlib.metadata import version; "
        "from llm_proxy_client import Client, ClientConfig, ClientMessage, "
        "ClientMessagesRequest, LLMProxyModelProfileError; "
        "assert Client and ClientConfig and ClientMessage and "
        "ClientMessagesRequest and LLMProxyModelProfileError; "
        "print(version('llm-proxy-client'))",
    )
    assert output == expected


@pytest.mark.parametrize("selected_version", ["1.5.0", "1.5.1", "1.42.73"])
def test_selected_release_package(
    package_repository: Path, tmp_path: Path, selected_version: str,
) -> None:
    tag = f"v{selected_version}"
    decision = json.dumps({
        "contract": "mprlab.version-decision/v2",
        "policy": {"scheme": "semver", "fixed_major": 1},
        "next_version": tag,
    })
    assert run(
        REPOSITORY_ROOT, "bash", "scripts/validate-release-decision", input_text=decision,
    ) == f"LLM_PROXY_RELEASE_POLICY_OK version={tag}"
    run(package_repository, "git", "tag", tag)
    assert_installed_version(
        tmp_path,
        f"llm-proxy-client @ git+{package_repository.as_uri()}@{tag}#subdirectory=python",
        selected_version,
    )

    artifacts = tmp_path / "artifacts"
    run(package_repository, UV, "build", "--project", "python", "--out-dir", str(artifacts))
    wheel, = artifacts.glob("*.whl")
    assert_installed_version(tmp_path, str(wheel), selected_version)
    source_distribution, = artifacts.glob("*.tar.gz")
    # Rebuild outside the repository to prove the sdist carries its own version.
    extracted = tmp_path / "extracted"
    with tarfile.open(source_distribution) as archive:
        archive.extractall(extracted, filter="data")
    source_root, = extracted.iterdir()
    assert_installed_version(tmp_path, str(source_root), selected_version)


def test_development_package(package_repository: Path, tmp_path: Path) -> None:
    artifacts = tmp_path / "development"
    run(package_repository, UV, "build", "--project", "python", "--out-dir", str(artifacts))
    wheel, = artifacts.glob("*.whl")
    assert ".dev1" in wheel.name
    assert_installed_version(tmp_path, str(wheel), wheel.name.split("-")[1])


def test_local_install_updates_after_release_tag(package_repository: Path, tmp_path: Path) -> None:
    project = str(package_repository / "python")
    run(tmp_path, UV, "run", "--no-project", "--with", project, "python", "-c", "import llm_proxy_client")
    run(package_repository, "git", "tag", "v1.42.73")
    assert_installed_version(tmp_path, project, "1.42.73")
