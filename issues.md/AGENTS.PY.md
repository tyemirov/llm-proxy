# AGENTS.PY.md

## Scope

Backend guidance for Python code. Follow AGENTS.md for repo-wide policies, documentation rules, and workflow expectations.

## Backend (Python)

### Core Principles

- Reuse existing modules first; extend or adapt before writing new code.
- Generalize existing implementations rather than duplicating logic.
- Favor **data-driven** solutions (maps, registries, configuration) over imperative branching.
- Encapsulate domain rules in **dataclasses** or dedicated classes with clear invariants.
- Keep functions small, pure, and composable; separate logic from I/O.
- Inject all external dependencies (files, network, randomness, time). No hidden globals.
- Treat inputs as immutable; always return new values instead of mutating.
- Minimal public API surface; expose only one clear solution.
- For validation, error handling, and invariants, follow **POLICY.md (Confident Programming)**.

---

### Code Style

- Descriptive identifiers only; no single-letter names.
- Use `@dataclass(frozen=True)` for immutable domain types.
- Validation happens in `__post_init__` or via Pydantic (if already in use).
- Raise `ValueError` subclasses for domain validation errors.
- Lift repeated string literals to constants.
- Module docstrings and class/function docstrings required; no inline comments.
- Use type hints everywhere; keep strict type checking and run it via the repo `Makefile` (for example, `make lint`).
- Logging through standard `logging` module; no stray `print`.

---

### Project Structure

- `app/` or `src/` as top-level application package.
- `domain/` for core business objects and invariants.
- `infrastructure/` for DB, network, and OS integration.
- `services/` for orchestration logic using domain + infra.

---

### Configuration & CLI

- Use `argparse` or `typer` for CLI.
- Read configuration from environment or `.env` files.
- Validate configuration up front (edge validation).

---

### Dependencies

- Prefer standard library; third-party libraries require explicit approval.
- Allowed: `dataclasses`, `typing`, `pydantic` (optional), `pytest`, `mypy`.

---

### Testing

- Follows the repo-wide **Testing Philosophy** in `AGENTS.md`: inverted test pyramid, coverage driven by black-box integration/end-to-end scenarios through public entry points and trending toward (approximately) 100% with an agreed CI threshold; unit tests are discouraged and allowed only as narrow guardrails.
- Use `pytest` for black-box integration/end-to-end tests; parameterize cases to cover scenario matrices without drifting into unit-test-only coverage.
- Isolate side effects with fixtures.
- Use `tmp_path` for filesystem operations (no pollution).
- Black-box: test only public API contracts.
- CI gate: `make lint` (mypy/pyright) and `make test` (pytest), or `make ci` when available.

---

### Review Checklist

- [ ] Reused/extended existing code.
- [ ] Domain objects created via smart constructors or dataclasses with invariants.
- [ ] No duplicated validation inside core.
- [ ] Constants used for repeated strings.
- [ ] Clear type hints, no single-letter identifiers.
- [ ] Config validated at startup.
- [ ] `make lint` and `make test` (or `make ci`) passing.
