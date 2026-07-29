from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
PREPARE_CONTROLLER = REPOSITORY_ROOT / "scripts" / "prepare_gateway_controller.py"
LOCK_PATH = REPOSITORY_ROOT / ".mprlab" / "deploy" / "gateway-controller.lock.json"
ARCHIVE_PATH = (
    REPOSITORY_ROOT
    / ".mprlab"
    / "deploy"
    / "mprlab-gateway-deploy-bundle-v1.3.0.tar.gz"
)


class GatewayControllerTests(unittest.TestCase):
    def test_committed_controller_verifies_and_extracts(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            extract_root = Path(temporary_directory) / "controller"
            extract_root.mkdir()

            result = self.prepare(LOCK_PATH, ARCHIVE_PATH, extract_root)

            self.assertEqual(result.returncode, 0, result.stderr)
            controller = extract_root / "bin" / "mprlab-gateway-deploy-target"
            self.assertTrue(controller.is_file())
            self.assertTrue(os.access(controller, os.X_OK))

    def test_controller_rejects_lock_digest_tampering(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            temporary_root = Path(temporary_directory)
            extract_root = temporary_root / "controller"
            extract_root.mkdir()
            lock = json.loads(LOCK_PATH.read_text(encoding="utf-8"))
            lock["contentDigest"] = "0" * 64
            tampered_lock = temporary_root / "gateway-controller.lock.json"
            tampered_lock.write_text(
                json.dumps(lock) + "\n",
                encoding="utf-8",
            )

            result = self.prepare(tampered_lock, ARCHIVE_PATH, extract_root)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                "controller archive content does not match the pinned digest",
                result.stderr,
            )
            self.assertEqual(list(extract_root.iterdir()), [])

    def test_controller_rejects_archive_tampering(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            temporary_root = Path(temporary_directory)
            extract_root = temporary_root / "controller"
            extract_root.mkdir()
            tampered_archive = temporary_root / ARCHIVE_PATH.name
            shutil.copyfile(ARCHIVE_PATH, tampered_archive)
            payload = bytearray(tampered_archive.read_bytes())
            payload[len(payload) // 2] ^= 0x01
            tampered_archive.write_bytes(payload)

            result = self.prepare(LOCK_PATH, tampered_archive, extract_root)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("controller archive", result.stderr)
            self.assertEqual(list(extract_root.iterdir()), [])

    def prepare(
        self,
        lock_path: Path,
        archive_path: Path,
        extract_root: Path,
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(PREPARE_CONTROLLER),
                "--lock",
                str(lock_path),
                "--extract-root",
                str(extract_root),
                "--archive",
                str(archive_path),
            ],
            check=False,
            capture_output=True,
            text=True,
        )


if __name__ == "__main__":
    unittest.main()
