"""Exercise the operator snapshot command through its CLI."""

import hashlib
import json
from pathlib import Path
import sqlite3
import subprocess
import sys


SCRIPT = Path(__file__).resolve().parents[2] / "scripts" / "snapshot_managed_database.py"


def copy_database(source: Path, destination: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, str(SCRIPT), "--source", str(source), "--destination", str(destination)],
        capture_output=True, text=True, check=False,
    )


def test_snapshot_copies_committed_wal_and_preserves_destinations(tmp_path: Path) -> None:
    source = tmp_path / "managed ' account.db"
    destination = tmp_path / "backup ' account.db"
    database = sqlite3.connect(source)
    try:
        database.execute("PRAGMA journal_mode=WAL")
        database.execute("CREATE TABLE financial_effects (id INTEGER PRIMARY KEY, cents INTEGER)")
        database.execute("INSERT INTO financial_effects VALUES (1, 500)")
        database.commit()
        database.execute("INSERT INTO financial_effects VALUES (2, 100)")
        result = copy_database(source, destination)
        assert result.returncode == 0, result.stderr
        receipt = json.loads(result.stdout)
        assert receipt["sha256"] == hashlib.sha256(destination.read_bytes()).hexdigest()
        assert receipt["size_bytes"] == destination.stat().st_size
        restored = sqlite3.connect(destination)
        try:
            assert restored.execute("SELECT * FROM financial_effects").fetchall() == [(1, 500)]
        finally:
            restored.close()
        original = destination.read_bytes()
        assert copy_database(source, destination).returncode != 0
        assert destination.read_bytes() == original
        restored_path = tmp_path / "restored.db"
        restored_result = copy_database(destination, restored_path)
        assert restored_result.returncode == 0, restored_result.stderr
        assert json.loads(restored_result.stdout)["sha256"] == hashlib.sha256(restored_path.read_bytes()).hexdigest()
        assert destination.read_bytes() == original
    finally:
        database.rollback()
        database.close()


def test_snapshot_rejects_missing_corrupt_and_inconsistent_sources(tmp_path: Path) -> None:
    destination = tmp_path / "unpublished.db"
    missing = tmp_path / "missing.db"
    assert copy_database(missing, destination).returncode != 0
    assert not missing.exists()
    assert not destination.exists()
    corrupt = tmp_path / "corrupt.db"
    corrupt.write_bytes(b"not a database")
    assert copy_database(corrupt, destination).returncode != 0
    assert not destination.exists()
    inconsistent = tmp_path / "inconsistent.db"
    database = sqlite3.connect(inconsistent)
    try:
        database.executescript("CREATE TABLE accounts (id INTEGER PRIMARY KEY); CREATE TABLE entries (account_id INTEGER REFERENCES accounts(id)); INSERT INTO entries VALUES (99);")
    finally:
        database.close()
    result = copy_database(inconsistent, destination)
    assert result.returncode != 0
    assert "foreign key" in result.stderr
    assert not destination.exists()
