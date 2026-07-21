#!/usr/bin/env python3
"""Validate published Pages release artifacts for deploy_pages_artifact.sh."""

from __future__ import annotations

import argparse
import json
import pathlib
import tarfile


PAGES_BUILD_BUILT = "built"
PAGES_BUILD_ERRORED = "errored"
PAGES_BUILD_WAITING = frozenset({"queued", "building"})


def read_json(path: str) -> dict[str, object]:
    data = json.loads(pathlib.Path(path).read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise SystemExit(f"JSON document must be an object: {path}")
    return data


def command_manifest_values(args: argparse.Namespace) -> int:
    manifest = read_json(args.manifest)
    if manifest.get("schema_version") != 2 or manifest.get("artifact_kind") != "mprlab.release":
        raise SystemExit("published release manifest has an invalid contract")
    if manifest.get("version") != args.version:
        raise SystemExit("published release manifest has the wrong version")
    payloads = manifest.get("payloads")
    if not isinstance(payloads, list):
        raise SystemExit("published release manifest has an invalid payload inventory")
    asset = next(
        (
            item
            for item in payloads
            if isinstance(item, dict) and item.get("path") == "payloads/release-assets/pages.tar.gz"
        ),
        None,
    )
    if asset is None:
        raise SystemExit("published release has no Pages payload; run make release and make publish")
    for field in ("release_commit", "source_commit"):
        value = manifest.get(field)
        if not isinstance(value, str) or not value:
            raise SystemExit(f"published release manifest has an invalid {field}")
        print(value)
    sha256 = asset.get("sha256")
    if not isinstance(sha256, str) or not sha256:
        raise SystemExit("published release manifest has an invalid Pages payload hash")
    print(sha256)
    return 0


def command_validate_archive(args: argparse.Namespace) -> int:
    has_nojekyll = False
    with tarfile.open(args.archive, "r:gz") as archive:
        for member in archive.getmembers():
            path = pathlib.PurePosixPath(member.name)
            if path.is_absolute() or ".." in path.parts or any(part.casefold() == ".git" for part in path.parts):
                raise SystemExit(f"unsafe Pages archive member: {member.name}")
            if not (member.isfile() or member.isdir()):
                raise SystemExit(f"unsafe Pages archive member: {member.name}")
            if member.isfile() and path == pathlib.PurePosixPath(".nojekyll"):
                has_nojekyll = True
    if not has_nojekyll:
        raise SystemExit("published Pages asset has no .nojekyll marker")
    return 0


def command_validate_marker(args: argparse.Namespace) -> int:
    marker_path = pathlib.Path(args.marker)
    if not marker_path.is_file():
        raise SystemExit("published Pages asset has no release marker")
    marker = read_json(args.marker)
    if marker.get("schema_version") != 1:
        raise SystemExit("published Pages release marker has an invalid contract")
    if marker.get("source_commit") != args.source_commit:
        raise SystemExit("published Pages release marker has the wrong source commit")
    if marker.get("release_version") != args.version:
        raise SystemExit("published Pages release marker has the wrong release version")
    return 0


def command_validate_public_marker(args: argparse.Namespace) -> int:
    marker = read_json(args.marker)
    if marker.get("schema_version") != 1:
        return 1
    if marker.get("release_version") != args.version:
        return 1
    return 0 if marker.get("source_commit") == args.source_commit else 1


def command_pages_build_state(args: argparse.Namespace) -> int:
    payload = json.loads(pathlib.Path(args.builds).read_text(encoding="utf-8"))
    if not isinstance(payload, list):
        raise SystemExit("GitHub Pages builds response must be a list")
    matching_builds: list[dict[str, object]] = []
    for build in payload:
        if not isinstance(build, dict):
            raise SystemExit("GitHub Pages builds response contains an invalid build")
        if build.get("commit") == args.commit:
            matching_builds.append(build)
    if not matching_builds:
        print("waiting")
        return 0
    statuses: set[str] = set()
    for build in matching_builds:
        status = build.get("status")
        if not isinstance(status, str):
            raise SystemExit("GitHub Pages build has an invalid status")
        statuses.add(status)
    if PAGES_BUILD_BUILT in statuses:
        print(PAGES_BUILD_BUILT)
        return 0
    if statuses.intersection(PAGES_BUILD_WAITING):
        print("waiting")
        return 0
    if statuses != {PAGES_BUILD_ERRORED}:
        raise SystemExit(f"GitHub Pages build has an unknown status set: {sorted(statuses)}")
    error_message = "no error message reported"
    for build in matching_builds:
        error = build.get("error")
        if isinstance(error, dict):
            message = error.get("message")
            if isinstance(message, str) and message:
                error_message = message
                break
    print("errored")
    print(error_message)
    return 0


def command_pages_site_matches(args: argparse.Namespace) -> int:
    site = read_json(args.site)
    source = site.get("source")
    if not isinstance(source, dict):
        return 1
    return int(
        source.get("branch") != args.branch
        or source.get("path") != "/"
        or site.get("https_enforced") is not True
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)

    manifest = subparsers.add_parser("manifest-values")
    manifest.add_argument("--manifest", required=True)
    manifest.add_argument("--version", required=True)
    manifest.set_defaults(func=command_manifest_values)

    archive = subparsers.add_parser("validate-archive")
    archive.add_argument("--archive", required=True)
    archive.set_defaults(func=command_validate_archive)

    marker = subparsers.add_parser("validate-marker")
    marker.add_argument("--marker", required=True)
    marker.add_argument("--source-commit", required=True)
    marker.add_argument("--version", required=True)
    marker.set_defaults(func=command_validate_marker)

    public_marker = subparsers.add_parser("validate-public-marker")
    public_marker.add_argument("--marker", required=True)
    public_marker.add_argument("--source-commit", required=True)
    public_marker.add_argument("--version", required=True)
    public_marker.set_defaults(func=command_validate_public_marker)

    pages_build_state = subparsers.add_parser("pages-build-state")
    pages_build_state.add_argument("--builds", required=True)
    pages_build_state.add_argument("--commit", required=True)
    pages_build_state.set_defaults(func=command_pages_build_state)

    pages_site_matches = subparsers.add_parser("pages-site-matches")
    pages_site_matches.add_argument("--site", required=True)
    pages_site_matches.add_argument("--branch", required=True)
    pages_site_matches.set_defaults(func=command_pages_site_matches)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
