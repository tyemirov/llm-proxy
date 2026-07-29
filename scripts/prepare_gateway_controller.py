#!/usr/bin/env python3
"""Verify and extract the content-pinned MPRLab gateway controller bundle."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import tarfile
from typing import Any


ATTESTATION_NAME = "BUNDLE-ATTESTATION.json"
MANIFEST_NAME = "BUNDLE-MANIFEST.txt"
VERSION_NAME = "VERSION"
TARGET_CONTROLLER_PATH = "bin/mprlab-gateway-deploy-target"
ATTESTATION_KEYS = {
    "schemaVersion",
    "releaseVersion",
    "sourceCommit",
    "sourceKind",
    "contentDigest",
    "files",
}
LOCK_KEYS = {
    "schemaVersion",
    "releaseVersion",
    "assetName",
    "contentDigest",
}
RELEASE_PATTERN = re.compile(r"^v[0-9]+[.][0-9]+[.][0-9]+$")
DIGEST_PATTERN = re.compile(r"^[0-9a-f]{64}$")
SOURCE_COMMIT_PATTERN = re.compile(r"^[0-9a-f]{40}$")


class ControllerError(ValueError):
    """The locked controller input violates the deployment contract."""


def canonical_json(value: object) -> bytes:
    """Return the gateway bundle's canonical JSON encoding."""
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode(
        "utf-8"
    )


def sha256(payload: bytes) -> str:
    """Return a lowercase SHA-256 digest."""
    return hashlib.sha256(payload).hexdigest()


def load_lock(path: Path) -> dict[str, Any]:
    """Load and strictly validate one controller lock."""
    if path.is_symlink() or not path.is_file():
        raise ControllerError(f"controller lock must be a regular file: {path}")
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (UnicodeError, json.JSONDecodeError) as error:
        raise ControllerError("controller lock must be valid JSON") from error
    if not isinstance(value, dict) or set(value) != LOCK_KEYS:
        raise ControllerError("controller lock has an invalid schema")
    if value.get("schemaVersion") != 1:
        raise ControllerError("controller lock schemaVersion must be 1")
    release_version = value.get("releaseVersion")
    asset_name = value.get("assetName")
    content_digest = value.get("contentDigest")
    if (
        not isinstance(release_version, str)
        or RELEASE_PATTERN.fullmatch(release_version) is None
    ):
        raise ControllerError("controller lock releaseVersion is invalid")
    expected_asset = f"mprlab-gateway-deploy-bundle-{release_version}.tar.gz"
    if asset_name != expected_asset:
        raise ControllerError(f"controller lock assetName must be {expected_asset}")
    if (
        not isinstance(content_digest, str)
        or DIGEST_PATTERN.fullmatch(content_digest) is None
    ):
        raise ControllerError("controller lock contentDigest is invalid")
    return value


def require_archive(lock: dict[str, Any], archive: Path) -> Path:
    """Return the canonical committed controller archive."""
    if archive.name != lock["assetName"]:
        raise ControllerError("controller archive name does not match its lock")
    if archive.is_symlink() or not archive.is_file():
        raise ControllerError(f"controller archive must be a regular file: {archive}")
    return archive.resolve(strict=True)


def canonical_member_path(raw: str, expected_root: str) -> str:
    """Validate and return one bundle-relative archive path."""
    path = PurePosixPath(raw)
    if path.is_absolute() or ".." in path.parts or len(path.parts) < 2:
        raise ControllerError(f"controller archive has a non-canonical member: {raw}")
    if path.parts[0] != expected_root:
        raise ControllerError("controller archive has an unexpected top-level directory")
    relative = PurePosixPath(*path.parts[1:]).as_posix()
    if raw != f"{expected_root}/{relative}" or relative.startswith("./"):
        raise ControllerError(f"controller archive has a non-canonical member: {raw}")
    return relative


def file_entry(path: str, payload: bytes, mode: int) -> dict[str, object]:
    """Build one canonical bundle attestation entry."""
    return {
        "path": path,
        "mode": mode,
        "size": len(payload),
        "sha256": sha256(payload),
    }


def verify_archive(
    archive_path: Path, lock: dict[str, Any]
) -> tuple[str, dict[str, tuple[bytes, int]]]:
    """Verify every archive byte against the locked content digest."""
    expected_root = f"mprlab-gateway-deploy-bundle-{lock['releaseVersion']}"
    files: dict[str, tuple[bytes, int]] = {}
    try:
        with tarfile.open(archive_path, mode="r:gz") as archive:
            for member in archive.getmembers():
                relative = canonical_member_path(member.name, expected_root)
                if not member.isfile():
                    raise ControllerError(
                        f"controller archive member must be a regular file: {member.name}"
                    )
                if relative in files:
                    raise ControllerError(
                        f"controller archive path is duplicated: {relative}"
                    )
                stream = archive.extractfile(member)
                if stream is None:
                    raise ControllerError(
                        f"cannot read controller archive member: {member.name}"
                    )
                files[relative] = (stream.read(), stat.S_IMODE(member.mode))
    except (tarfile.TarError, OSError) as error:
        raise ControllerError(f"cannot read controller archive: {error}") from error
    for required in (
        ATTESTATION_NAME,
        MANIFEST_NAME,
        VERSION_NAME,
        TARGET_CONTROLLER_PATH,
    ):
        if required not in files:
            raise ControllerError(f"controller archive is missing {required}")
    try:
        attestation = json.loads(files[ATTESTATION_NAME][0])
    except (UnicodeError, json.JSONDecodeError) as error:
        raise ControllerError("controller bundle attestation is invalid") from error
    if not isinstance(attestation, dict) or set(attestation) != ATTESTATION_KEYS:
        raise ControllerError("controller bundle attestation has an invalid schema")
    if attestation.get("schemaVersion") != 1:
        raise ControllerError("controller bundle attestation schema must be 1")
    if attestation.get("releaseVersion") != lock["releaseVersion"]:
        raise ControllerError("controller bundle release version does not match its lock")
    if attestation.get("sourceKind") != "commit":
        raise ControllerError("controller release bundle must come from committed source")
    source_commit = attestation.get("sourceCommit")
    if (
        not isinstance(source_commit, str)
        or SOURCE_COMMIT_PATTERN.fullmatch(source_commit) is None
    ):
        raise ControllerError("controller bundle source commit is invalid")
    entries = [
        file_entry(path, payload, mode)
        for path, (payload, mode) in sorted(files.items())
        if path != ATTESTATION_NAME
    ]
    if attestation.get("files") != entries:
        raise ControllerError("controller archive content does not match its attestation")
    computed_digest = sha256(canonical_json(entries))
    if attestation.get("contentDigest") != computed_digest:
        raise ControllerError("controller archive attestation digest is invalid")
    if computed_digest != lock["contentDigest"]:
        raise ControllerError("controller archive content does not match the pinned digest")
    try:
        manifest_paths = files[MANIFEST_NAME][0].decode("utf-8").splitlines()
    except UnicodeError as error:
        raise ControllerError("controller bundle manifest is invalid") from error
    expected_manifest_paths = sorted(
        path for path in files if path not in {ATTESTATION_NAME, MANIFEST_NAME}
    )
    if manifest_paths != expected_manifest_paths:
        raise ControllerError("controller bundle manifest does not match its content")
    if files[VERSION_NAME][0] != f"{lock['releaseVersion']}\n".encode("utf-8"):
        raise ControllerError("controller archive VERSION does not match its lock")
    if files[TARGET_CONTROLLER_PATH][1] & 0o111 == 0:
        raise ControllerError("controller target entrypoint is not executable")
    return expected_root, files


def extract_verified(
    files: dict[str, tuple[bytes, int]],
    extract_root: Path,
) -> Path:
    """Materialize verified regular files in one empty controller root."""
    if extract_root.is_symlink() or not extract_root.is_dir():
        raise ControllerError(f"extract root must be a directory: {extract_root}")
    if any(extract_root.iterdir()):
        raise ControllerError("extract root must be empty")
    for relative, (payload, mode) in sorted(files.items()):
        destination = extract_root / relative
        destination.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        descriptor = os.open(
            destination,
            os.O_CREAT | os.O_EXCL | os.O_WRONLY,
            0o600,
        )
        try:
            remaining_payload = memoryview(payload)
            while remaining_payload:
                written_bytes = os.write(descriptor, remaining_payload)
                if written_bytes == 0:
                    raise OSError(
                        f"cannot write controller archive member: {relative}"
                    )
                remaining_payload = remaining_payload[written_bytes:]
        finally:
            os.close(descriptor)
        os.chmod(destination, mode)
    return extract_root


def parse_arguments() -> argparse.Namespace:
    """Parse the strict controller preparation command."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--lock", type=Path, required=True)
    parser.add_argument("--extract-root", type=Path, required=True)
    parser.add_argument("--archive", type=Path, required=True)
    return parser.parse_args()


def main() -> int:
    """Prepare one verified gateway controller and print its root."""
    arguments = parse_arguments()
    try:
        lock = load_lock(arguments.lock)
        archive = require_archive(lock, arguments.archive)
        _, files = verify_archive(archive, lock)
        root = extract_verified(files, arguments.extract_root)
    except (ControllerError, OSError) as error:
        print(f"error: {error}", file=os.sys.stderr)
        return 1
    print(root)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
