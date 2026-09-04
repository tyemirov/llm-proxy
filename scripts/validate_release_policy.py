#!/usr/bin/env python3
"""Validate the application-owned release version policy."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

from release_version import ReleaseVersionError, read_repository_versions


VERSION_DECISION_CONTRACT = "mprlab.version-decision/v2"
FIXED_MAJOR = 1


def main() -> int:
    try:
        decision: Any = json.load(sys.stdin)
    except json.JSONDecodeError as error_value:
        raise SystemExit(
            "llm_proxy.release_policy_invalid: expected one release decision document"
        ) from error_value
    if not isinstance(decision, dict) or decision.get("contract") != VERSION_DECISION_CONTRACT:
        raise SystemExit(
            "llm_proxy.release_policy_invalid: expected one release decision document"
        )
    policy = decision.get("policy")
    if policy != {"scheme": "semver", "fixed_major": FIXED_MAJOR}:
        raise SystemExit(
            "llm_proxy.release_policy_invalid: expected SemVer decision with fixed major 1"
        )

    repository_root = Path(__file__).resolve().parents[1]
    try:
        repository_version = read_repository_versions(repository_root).require_equal()
    except (OSError, ReleaseVersionError) as error_value:
        raise SystemExit(
            f"llm_proxy.release_policy_invalid: {error_value}"
        ) from error_value
    expected_version = f"v{repository_version}"
    version = decision.get("next_version")
    if not isinstance(version, str) or version != expected_version:
        raise SystemExit(
            "llm_proxy.release_version_invalid: release version must match repository "
            f"version {expected_version}"
        )

    print(f"LLM_PROXY_RELEASE_POLICY_OK version={version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
