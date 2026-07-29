#!/usr/bin/env python3
"""Validate immutable OCI publication state for container release retries."""

from __future__ import annotations

import argparse
import json
import pathlib
import re


CONTAINER_DIGEST_PATTERN = re.compile(r"^sha256:[0-9a-f]{64}$")
CONTAINER_PLATFORM_PATTERN = re.compile(r"^linux/(amd64|arm64)$")
ATTESTATION_REFERENCE_DIGEST = "vnd.docker.reference.digest"
ATTESTATION_REFERENCE_TYPE = "vnd.docker.reference.type"
ATTESTATION_MANIFEST_TYPE = "attestation-manifest"
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


def validated_size(value: object, label: str) -> int:
    """Return one positive OCI descriptor size."""
    if isinstance(value, bool) or not isinstance(value, int) or value <= 0:
        raise SystemExit(f"{label} must be a positive integer")
    return value


def normalized_platform(value: object, label: str) -> dict[str, str]:
    """Return one exact OCI operating-system and architecture pair."""
    if not isinstance(value, dict) or set(value) != {"os", "architecture"}:
        raise SystemExit(f"{label} must contain exactly os and architecture")
    operating_system = value.get("os")
    architecture = value.get("architecture")
    if not isinstance(operating_system, str) or not isinstance(architecture, str):
        raise SystemExit(f"{label} must contain text os and architecture values")
    return {"os": operating_system, "architecture": architecture}


def normalized_image_descriptor(value: object, label: str) -> dict[str, object]:
    """Return one exact runnable image descriptor from a platform index."""
    if not isinstance(value, dict):
        raise SystemExit(f"{label} must be an object")
    if set(value) != {"mediaType", "digest", "size", "platform"}:
        raise SystemExit(f"{label} has an invalid descriptor shape")
    media_type = value.get("mediaType")
    if media_type not in IMAGE_MANIFEST_MEDIA_TYPES:
        raise SystemExit(f"{label} has an invalid image manifest media type")
    return {
        "mediaType": media_type,
        "digest": validated_digest(value.get("digest"), f"{label} digest"),
        "size": validated_size(value.get("size"), f"{label} size"),
        "platform": normalized_platform(value.get("platform"), f"{label} platform"),
    }


def normalized_attestation_descriptor(
    value: object,
    image_digest: str,
    label: str,
) -> dict[str, object]:
    """Return one exact provenance attestation descriptor."""
    if not isinstance(value, dict):
        raise SystemExit(f"{label} must be an object")
    if set(value) != {"mediaType", "digest", "size", "annotations", "platform"}:
        raise SystemExit(f"{label} has an invalid descriptor shape")
    media_type = value.get("mediaType")
    if media_type not in IMAGE_MANIFEST_MEDIA_TYPES:
        raise SystemExit(f"{label} has an invalid image manifest media type")
    annotations = value.get("annotations")
    expected_annotations = {
        ATTESTATION_REFERENCE_DIGEST: image_digest,
        ATTESTATION_REFERENCE_TYPE: ATTESTATION_MANIFEST_TYPE,
    }
    if annotations != expected_annotations:
        raise SystemExit(f"{label} does not reference the runnable image manifest")
    platform = normalized_platform(value.get("platform"), f"{label} platform")
    if platform != {"os": "unknown", "architecture": "unknown"}:
        raise SystemExit(f"{label} must use the unknown/unknown platform")
    return {
        "mediaType": media_type,
        "digest": validated_digest(value.get("digest"), f"{label} digest"),
        "size": validated_size(value.get("size"), f"{label} size"),
        "annotations": expected_annotations,
        "platform": platform,
    }


def normalized_platform_index_descriptors(
    manifest: dict[str, object],
    expected_platform: str | None,
    label: str,
) -> list[dict[str, object]]:
    """Return the runnable image and provenance descriptors from one platform index."""
    if manifest.get("schemaVersion") != 2 or manifest.get("mediaType") not in IMAGE_INDEX_MEDIA_TYPES:
        raise SystemExit(f"{label} is not an immutable OCI image index")
    raw_manifests = manifest.get("manifests")
    if not isinstance(raw_manifests, list) or len(raw_manifests) != 2:
        raise SystemExit(f"{label} must contain exactly one image and one attestation manifest")
    image_candidates = [
        item
        for item in raw_manifests
        if isinstance(item, dict)
        and isinstance(item.get("platform"), dict)
        and item["platform"].get("os") == "linux"
    ]
    attestation_candidates = [
        item
        for item in raw_manifests
        if isinstance(item, dict)
        and isinstance(item.get("platform"), dict)
        and item["platform"].get("os") == "unknown"
        and item["platform"].get("architecture") == "unknown"
    ]
    if len(image_candidates) != 1 or len(attestation_candidates) != 1:
        raise SystemExit(f"{label} must contain one Linux image and one attestation manifest")
    image_descriptor = normalized_image_descriptor(image_candidates[0], f"{label} image")
    image_platform = image_descriptor["platform"]
    if not isinstance(image_platform, dict):
        raise AssertionError("normalized image platform must be an object")
    platform_text = f"{image_platform['os']}/{image_platform['architecture']}"
    if CONTAINER_PLATFORM_PATTERN.fullmatch(platform_text) is None:
        raise SystemExit(f"{label} image has an unsupported platform")
    if expected_platform is not None and platform_text != expected_platform:
        raise SystemExit(
            f"{label} has the wrong platform (expected {expected_platform}, found {platform_text})"
        )
    image_digest = str(image_descriptor["digest"])
    attestation_descriptor = normalized_attestation_descriptor(
        attestation_candidates[0],
        image_digest,
        f"{label} attestation",
    )
    return [image_descriptor, attestation_descriptor]


def command_validate_platform_manifest(args: argparse.Namespace) -> int:
    """Require a published platform tag to identify the prepared OCI index."""
    manifest = read_json_object(args.raw)
    if CONTAINER_PLATFORM_PATTERN.fullmatch(args.platform) is None:
        raise SystemExit("prepared platform must be linux/amd64 or linux/arm64")
    normalized_platform_index_descriptors(manifest, args.platform, "published platform tag")
    actual_image_id = validated_digest(args.manifest_digest, "published platform index")
    expected_image_id = validated_digest(args.image_id, "prepared image id")
    if actual_image_id != expected_image_id:
        raise SystemExit(
            "published platform index conflicts with the prepared image "
            f"(expected {expected_image_id}, found {actual_image_id})"
        )
    return 0


def command_validate_index_manifest(args: argparse.Namespace) -> int:
    """Require a published version tag to contain exactly the prepared platforms."""
    manifest = read_json_object(args.raw)
    if manifest.get("schemaVersion") != 2 or manifest.get("mediaType") not in IMAGE_INDEX_MEDIA_TYPES:
        raise SystemExit("published version tag is not an immutable multi-platform image index")
    raw_manifests = manifest.get("manifests")
    if not isinstance(raw_manifests, list) or not raw_manifests:
        raise SystemExit("published version index has no platform manifests")
    actual_entries: dict[str, dict[str, object]] = {}
    for item in raw_manifests:
        if not isinstance(item, dict):
            raise SystemExit("published version index contains an invalid manifest entry")
        digest = validated_digest(item.get("digest"), "published version index manifest")
        if digest in actual_entries:
            raise SystemExit("published version index contains a duplicate manifest")
        actual_entries[digest] = item
    expected_entries: dict[str, dict[str, object]] = {}
    expected_platforms: set[str] = set()
    for platform_raw in args.platform_raw:
        platform_manifest = read_json_object(platform_raw)
        descriptors = normalized_platform_index_descriptors(
            platform_manifest,
            None,
            f"prepared platform index {platform_raw}",
        )
        platform = descriptors[0]["platform"]
        if not isinstance(platform, dict):
            raise AssertionError("normalized image platform must be an object")
        platform_text = f"{platform['os']}/{platform['architecture']}"
        if platform_text in expected_platforms:
            raise SystemExit("prepared platform indexes contain a duplicate platform")
        expected_platforms.add(platform_text)
        for descriptor in descriptors:
            digest = str(descriptor["digest"])
            if digest in expected_entries:
                raise SystemExit("prepared platform indexes contain a duplicate manifest")
            expected_entries[digest] = descriptor
    if actual_entries != expected_entries:
        raise SystemExit(
            "published version tag conflicts with the prepared platform indexes "
            f"(expected {expected_entries}, found {actual_entries})"
        )
    return 0


def build_parser() -> argparse.ArgumentParser:
    """Build the immutable container-state validation CLI."""
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)

    platform_manifest = subparsers.add_parser("validate-platform-index")
    platform_manifest.add_argument("--raw", required=True)
    platform_manifest.add_argument("--manifest-digest", required=True)
    platform_manifest.add_argument("--image-id", required=True)
    platform_manifest.add_argument("--platform", required=True)
    platform_manifest.set_defaults(func=command_validate_platform_manifest)

    index_manifest = subparsers.add_parser("validate-version-index")
    index_manifest.add_argument("--raw", required=True)
    index_manifest.add_argument("--platform-raw", action="append", required=True)
    index_manifest.set_defaults(func=command_validate_index_manifest)
    return parser


def main() -> int:
    """Run the selected immutable container-state validation."""
    args = build_parser().parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
