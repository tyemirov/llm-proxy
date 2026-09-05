#!/usr/bin/env python3
"""Validate the application-owned release version policy."""

from __future__ import annotations

import json
import re
import sys
from typing import Any


VERSION_DECISION_CONTRACT = "mprlab.version-decision/v2"
FIXED_MAJOR = 1
RELEASE_VERSION_PATTERN = re.compile(r"v1\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)")


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

    version = decision.get("next_version")
    if not isinstance(version, str) or RELEASE_VERSION_PATTERN.fullmatch(version) is None:
        raise SystemExit(
            "llm_proxy.release_version_invalid: expected a major version 1 SemVer release"
        )

    print(f"LLM_PROXY_RELEASE_POLICY_OK version={version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
