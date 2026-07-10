# Repository Release Toolchain

This directory owns the immutable local release, publication, container, and
Pages artifact commands used by the root `Makefile`. Keep the scripts and their
black-box tests versioned together so a clean llm-proxy checkout contains the
complete release implementation.

Release versions use the repository's single canonical `vMAJOR.MINOR.PATCH`
SemVer contract, with optional SemVer prerelease identifiers.
