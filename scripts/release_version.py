#!/usr/bin/env python3
"""Manage the canonical llm-proxy repository release version."""

from __future__ import annotations

import argparse
import os
import re
import stat
import tempfile
import tomllib
from dataclasses import dataclass
from pathlib import Path
from typing import Any


REPOSITORY_VERSION_PATH = Path("VERSION")
PYTHON_PROJECT_PATH = Path("python/pyproject.toml")
PYTHON_LOCK_PATH = Path("python/uv.lock")
PYTHON_PACKAGE_NAME = "llm-proxy-client"
VERSION_PATTERN = re.compile(r"^1\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$")
TOML_VERSION_PATTERN = re.compile(r'^version = "([^"]+)"$')


class ReleaseVersionError(ValueError):
    """Report an invalid repository release version contract."""


@dataclass(frozen=True, order=True)
class ReleaseVersion:
    """Store one validated major-version-1 repository release version."""

    major: int
    minor: int
    patch: int

    @classmethod
    def parse(cls, value: str, subject: str) -> ReleaseVersion:
        """Parse one canonical release version."""
        match = VERSION_PATTERN.fullmatch(value)
        if match is None:
            raise ReleaseVersionError(f"{subject} must use canonical major version 1")
        return cls(major=1, minor=int(match.group(1)), patch=int(match.group(2)))

    def __str__(self) -> str:
        return f"{self.major}.{self.minor}.{self.patch}"


@dataclass(frozen=True)
class RepositoryVersions:
    """Store each explicit repository release version."""

    canonical: ReleaseVersion
    python_project: ReleaseVersion
    python_lock: ReleaseVersion

    def require_equal(self) -> ReleaseVersion:
        """Return the canonical version when all explicit versions agree."""
        if self.python_project != self.canonical:
            raise ReleaseVersionError(
                "Python project version must match repository version "
                f"{self.canonical}"
            )
        if self.python_lock != self.canonical:
            raise ReleaseVersionError(
                "Python lock version must match repository version "
                f"{self.canonical}"
            )
        return self.canonical


def repository_root() -> Path:
    """Return the repository root that owns this program."""
    return Path(__file__).resolve().parents[1]


def read_repository_versions(root: Path) -> RepositoryVersions:
    """Read and validate each explicit repository release version."""
    canonical_text = (root / REPOSITORY_VERSION_PATH).read_text(encoding="utf-8")
    if not canonical_text.endswith("\n") or canonical_text.count("\n") != 1:
        raise ReleaseVersionError("repository version file must contain one version line")
    canonical = ReleaseVersion.parse(canonical_text.removesuffix("\n"), "repository version")

    try:
        with (root / PYTHON_PROJECT_PATH).open("rb") as project_file:
            project: Any = tomllib.load(project_file)
    except tomllib.TOMLDecodeError as error_value:
        raise ReleaseVersionError("Python project metadata is invalid") from error_value
    project_version = project.get("project", {}).get("version")
    if not isinstance(project_version, str):
        raise ReleaseVersionError("Python project version must be one string")

    try:
        with (root / PYTHON_LOCK_PATH).open("rb") as lock_file:
            lock: Any = tomllib.load(lock_file)
    except tomllib.TOMLDecodeError as error_value:
        raise ReleaseVersionError("Python lock metadata is invalid") from error_value
    matching_packages = [
        package
        for package in lock.get("package", [])
        if isinstance(package, dict) and package.get("name") == PYTHON_PACKAGE_NAME
    ]
    if len(matching_packages) != 1 or not isinstance(matching_packages[0].get("version"), str):
        raise ReleaseVersionError("Python lock must contain one llm-proxy client version")

    return RepositoryVersions(
        canonical=canonical,
        python_project=ReleaseVersion.parse(project_version, "Python project version"),
        python_lock=ReleaseVersion.parse(
            matching_packages[0]["version"], "Python lock version"
        ),
    )


def replace_table_version(
    document: str,
    table_header: str,
    current_version: ReleaseVersion,
    target_version: ReleaseVersion,
) -> str:
    """Replace the version in one exact TOML table."""
    lines = document.splitlines(keepends=True)
    in_target_table = False
    replacement_count = 0
    for index, line in enumerate(lines):
        stripped_line = line.rstrip("\r\n")
        if stripped_line.startswith("["):
            in_target_table = stripped_line == table_header
            continue
        if not in_target_table:
            continue
        match = TOML_VERSION_PATTERN.fullmatch(stripped_line)
        if match is None:
            continue
        if match.group(1) != str(current_version):
            raise ReleaseVersionError(f"{table_header} version changed during update")
        line_ending = line[len(stripped_line) :]
        lines[index] = f'version = "{target_version}"{line_ending}'
        replacement_count += 1
    if replacement_count != 1:
        raise ReleaseVersionError(f"{table_header} must contain one version")
    return "".join(lines)


def replace_python_lock_version(
    document: str,
    current_version: ReleaseVersion,
    target_version: ReleaseVersion,
) -> str:
    """Replace the llm-proxy client version in the Python lock."""
    lines = document.splitlines(keepends=True)
    package_start_indexes = [
        index for index, line in enumerate(lines) if line.rstrip("\r\n") == "[[package]]"
    ]
    package_start_indexes.append(len(lines))
    replacement_count = 0
    for package_index in range(len(package_start_indexes) - 1):
        start_index = package_start_indexes[package_index]
        end_index = package_start_indexes[package_index + 1]
        package_lines = lines[start_index:end_index]
        if f'name = "{PYTHON_PACKAGE_NAME}"' not in {
            line.rstrip("\r\n") for line in package_lines
        }:
            continue
        for line_index in range(start_index, end_index):
            stripped_line = lines[line_index].rstrip("\r\n")
            match = TOML_VERSION_PATTERN.fullmatch(stripped_line)
            if match is None:
                continue
            if match.group(1) != str(current_version):
                raise ReleaseVersionError("Python lock version changed during update")
            line_ending = lines[line_index][len(stripped_line) :]
            lines[line_index] = f'version = "{target_version}"{line_ending}'
            replacement_count += 1
    if replacement_count != 1:
        raise ReleaseVersionError("Python lock must contain one llm-proxy client version")
    return "".join(lines)


def write_atomic(path: Path, contents: str) -> None:
    """Replace one file after a durable same-directory temporary write."""
    permissions = stat.S_IMODE(path.stat().st_mode)
    file_descriptor, temporary_name = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}.")
    temporary_path = Path(temporary_name)
    try:
        os.fchmod(file_descriptor, permissions)
        with os.fdopen(file_descriptor, "w", encoding="utf-8", newline="") as temporary_file:
            temporary_file.write(contents)
            temporary_file.flush()
            os.fsync(temporary_file.fileno())
        os.replace(temporary_path, path)
    finally:
        temporary_path.unlink(missing_ok=True)


def set_repository_version(root: Path, target: ReleaseVersion) -> None:
    """Set each explicit repository release version to one value."""
    current = read_repository_versions(root)
    for source_version in (
        current.canonical,
        current.python_project,
        current.python_lock,
    ):
        if target < source_version:
            raise ReleaseVersionError(
                f"repository version cannot decrease from {source_version} to {target}"
            )

    project_path = root / PYTHON_PROJECT_PATH
    lock_path = root / PYTHON_LOCK_PATH
    project_document = project_path.read_text(encoding="utf-8")
    lock_document = lock_path.read_text(encoding="utf-8")
    updated_project = replace_table_version(
        project_document, "[project]", current.python_project, target
    )
    updated_lock = replace_python_lock_version(lock_document, current.python_lock, target)

    write_atomic(project_path, updated_project)
    write_atomic(lock_path, updated_lock)
    write_atomic(root / REPOSITORY_VERSION_PATH, f"{target}\n")


def parse_arguments() -> argparse.Namespace:
    """Parse the release version command arguments."""
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="operation", required=True)
    subparsers.add_parser("check", help="Validate all explicit release versions.")
    set_parser = subparsers.add_parser("set", help="Set all explicit release versions.")
    set_parser.add_argument("version", help="Canonical major-version-1 SemVer value.")
    return parser.parse_args()


def main() -> int:
    """Run the selected release version operation."""
    arguments = parse_arguments()
    root = repository_root()
    try:
        if arguments.operation == "check":
            version = read_repository_versions(root).require_equal()
            print(f"LLM_PROXY_RELEASE_VERSION_OK version=v{version}")
            return 0
        target = ReleaseVersion.parse(arguments.version, "target version")
        set_repository_version(root, target)
        version = read_repository_versions(root).require_equal()
    except (OSError, tomllib.TOMLDecodeError, ReleaseVersionError) as error_value:
        raise SystemExit(f"llm_proxy.release_version_invalid: {error_value}") from error_value
    print(f"LLM_PROXY_RELEASE_VERSION_UPDATED version=v{version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
