#!/usr/bin/env python3
"""Copy one consistent SQLite database to a new destination."""

from __future__ import annotations

import argparse
from contextlib import closing
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import sys
from tempfile import TemporaryDirectory


def snapshot(source: Path, destination: Path) -> dict[str, str | int]:
    """Validate a complete snapshot and publish it without replacing a file."""
    source = source.resolve(strict=True)
    destination = destination.absolute()
    with TemporaryDirectory(prefix=".llm-proxy-snapshot-", dir=destination.parent) as temporary:
        image = Path(temporary) / "database.db"
        with closing(sqlite3.connect(source.as_uri() + "?mode=ro", uri=True)) as origin:
            with closing(sqlite3.connect(image)) as copied:
                origin.backup(copied)
                copied.execute("PRAGMA journal_mode=DELETE")
                if copied.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
                    raise ValueError("database integrity check failed")
                if copied.execute("PRAGMA foreign_key_check").fetchone() is not None:
                    raise ValueError("database foreign key check failed")
        with image.open("rb") as stream:
            digest = hashlib.file_digest(stream, "sha256").hexdigest()
            os.fsync(stream.fileno())
        size = image.stat().st_size
        # The destination shares the temporary directory's filesystem. Linking
        # publishes the complete image atomically and rejects existing paths.
        os.link(image, destination)
        directory = os.open(destination.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    return {
        "source": str(source),
        "destination": str(destination),
        "sha256": digest,
        "size_bytes": size,
        "completed_at": datetime.now(timezone.utc).isoformat(),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--destination", required=True, type=Path)
    arguments = parser.parse_args()
    try:
        receipt = snapshot(arguments.source, arguments.destination)
    except (OSError, sqlite3.Error, ValueError) as error:
        print(f"managed_database_snapshot_failed: {error}", file=sys.stderr)
        return 1
    print(json.dumps(receipt, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
