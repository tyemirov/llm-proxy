# AGENTS.PY.md

## Scope

This file gives backend rules for Python code. Obey root `AGENTS.md` and `.mprlab/POLICY.md` for shared workflow rules.

## Core Principles

- Reuse existing modules first.
- Prefer data-driven registries and explicit domain types instead of branching.
- Use `@dataclass(frozen=True)` or Pydantic when already in use for validated domain values.
- Keep logic small, typed, and testable through public entry points.
- Inject files, network, randomness, time, and environment access.
- Validate at CLI, HTTP, file, and adapter edges.

## Code Style

- Use type hints.
- Use descriptive identifiers.
- Lift repeated literals into constants.
- Use module, class, and function docstrings where they clarify public behavior.
- Use `logging`. Do not leave stray `print` calls in libraries.
- Raise explicit exceptions for domain validation failures.

## Testing

- Use pytest.
- Prefer black-box integration tests through CLI, HTTP, or public package entry points.
- Use fixtures and `tmp_path` to isolate side effects.
- Unit tests are allowed only as narrow guardrails for pure deterministic helpers and never as replacement coverage for user-visible behavior.

## Validation

Use `.mprlab/POLICY.md` for validation.

During the change, run the smallest Python target that validates the changed contract.

When the repository contract includes it, lint must run mypy or pyright for typed Python surfaces.
