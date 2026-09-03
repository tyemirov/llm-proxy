#!/usr/bin/env python3
"""Validate the application-owned release version policy."""

from __future__ import annotations

import json
import re
import sys
import tomllib
from pathlib import Path
from typing import Any


VERSION_DECISION_CONTRACT = "mprlab.version-decision/v2"
FIXED_MAJOR = 1
V1_VERSION = re.compile(r"^v1\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$")
PYTHON_PROJECT_PATH = Path("python/pyproject.toml")


def official_client_version() -> str:
    with PYTHON_PROJECT_PATH.open("rb") as project_file:
        project = tomllib.load(project_file)
    version = project.get("project", {}).get("version")
    if not isinstance(version, str) or re.fullmatch(
        rf"{FIXED_MAJOR}\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version
    ) is None:
        raise SystemExit(
            "llm_proxy.release_policy_invalid: official client version must use major 1"
        )
    return version


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

    client_version = official_client_version()
    expected_version = f"v{client_version}"
    version = decision.get("next_version")
    if (
        not isinstance(version, str)
        or V1_VERSION.fullmatch(version) is None
        or version != expected_version
    ):
        raise SystemExit(
            "llm_proxy.release_version_invalid: release version must match "
            f"official client version {expected_version}"
        )

    print(f"LLM_PROXY_RELEASE_POLICY_OK version={version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
