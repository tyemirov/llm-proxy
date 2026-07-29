#!/usr/bin/env python3
"""Validate immutable OCI publication state for container release retries."""

from __future__ import annotations

import argparse
import json
import pathlib
import re


CONTAINER_DIGEST_PATTERN = re.compile(r"^sha256:[0-9a-f]{64}$")
IMAGE_MANIFEST_MEDIA_TYPES = frozenset(
    {
        "application/vnd.docker.distribution.manifest.v2+json",
        "application/vnd.oci.image.manifest.v1+json",
    }
)
IMAGE_INDEX_MEDIA_TYPES = frozenset(
    {
        "application/vnd.docker.distribution.manifest.list.v2+json",
        "application/vnd.oci.image.index.v1+json",
    }
)


def read_json_object(path: str) -> dict[str, object]:
    """Read one JSON object from a file boundary."""
    document = json.loads(pathlib.Path(path).read_text(encoding="utf-8"))
    if not isinstance(document, dict):
        raise SystemExit(f"JSON document must be an object: {path}")
    return document


def validated_digest(value: object, label: str) -> str:
    """Return one canonical SHA-256 container digest."""
    if not isinstance(value, str) or CONTAINER_DIGEST_PATTERN.fullmatch(value) is None:
        raise SystemExit(f"{label} must be a canonical sha256 digest")
    return value


def command_validate_platform_manifest(args: argparse.Namespace) -> int:
    """Require a published platform tag to identify the prepared image config."""
    manifest = read_json_object(args.raw)
    media_type = manifest.get("mediaType")
    if media_type not in IMAGE_MANIFEST_MEDIA_TYPES:
        raise SystemExit("published platform tag is not a single immutable image manifest")
    config = manifest.get("config")
    if not isinstance(config, dict):
        raise SystemExit("published platform manifest has no image config")
    actual_image_id = validated_digest(config.get("digest"), "published platform image config")
    expected_image_id = validated_digest(args.image_id, "prepared image id")
    if actual_image_id != expected_image_id:
        raise SystemExit(
            "published platform tag conflicts with the prepared image "
            f"(expected {expected_image_id}, found {actual_image_id})"
        )
    return 0


def expected_index_entries(path: str) -> dict[tuple[str, str], str]:
    """Read the exact platform-to-manifest mapping expected in a version index."""
    payload = json.loads(pathlib.Path(path).read_text(encoding="utf-8"))
    if not isinstance(payload, list) or not payload:
        raise SystemExit("expected container index entries must be a nonempty list")
    entries: dict[tuple[str, str], str] = {}
    for item in payload:
        if not isinstance(item, dict):
            raise SystemExit("expected container index entry must be an object")
        operating_system = item.get("os")
        architecture = item.get("architecture")
        if operating_system != "linux" or architecture not in {"amd64", "arm64"}:
            raise SystemExit("expected container index entry has an unsupported platform")
        key = (str(operating_system), str(architecture))
        if key in entries:
            raise SystemExit("expected container index contains a duplicate platform")
        entries[key] = validated_digest(item.get("digest"), "expected platform manifest")
    return entries


def command_validate_index_manifest(args: argparse.Namespace) -> int:
    """Require a published version tag to contain exactly the prepared platforms."""
    manifest = read_json_object(args.raw)
    if manifest.get("mediaType") not in IMAGE_INDEX_MEDIA_TYPES:
        raise SystemExit("published version tag is not an immutable multi-platform image index")
    raw_manifests = manifest.get("manifests")
    if not isinstance(raw_manifests, list) or not raw_manifests:
        raise SystemExit("published version index has no platform manifests")
    actual_entries: dict[tuple[str, str], str] = {}
    for item in raw_manifests:
        if not isinstance(item, dict):
            raise SystemExit("published version index contains an invalid manifest entry")
        platform = item.get("platform")
        if not isinstance(platform, dict):
            raise SystemExit("published version index entry has no platform")
        operating_system = platform.get("os")
        architecture = platform.get("architecture")
        if not isinstance(operating_system, str) or not isinstance(architecture, str):
            raise SystemExit("published version index entry has an invalid platform")
        key = (operating_system, architecture)
        if key in actual_entries:
            raise SystemExit("published version index contains a duplicate platform")
        actual_entries[key] = validated_digest(item.get("digest"), "published platform manifest")
    expected_entries = expected_index_entries(args.expected)
    if actual_entries != expected_entries:
        raise SystemExit(
            "published version tag conflicts with the prepared platform manifests "
            f"(expected {expected_entries}, found {actual_entries})"
        )
    return 0


def build_parser() -> argparse.ArgumentParser:
    """Build the immutable container-state validation CLI."""
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)

    platform_manifest = subparsers.add_parser("validate-platform-manifest")
    platform_manifest.add_argument("--raw", required=True)
    platform_manifest.add_argument("--image-id", required=True)
    platform_manifest.set_defaults(func=command_validate_platform_manifest)

    index_manifest = subparsers.add_parser("validate-index-manifest")
    index_manifest.add_argument("--raw", required=True)
    index_manifest.add_argument("--expected", required=True)
    index_manifest.set_defaults(func=command_validate_index_manifest)
    return parser


def main() -> int:
    """Run the selected immutable container-state validation."""
    args = build_parser().parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
