import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from crs_go import download_module, verify_file


class OverlayIntegrityTest(unittest.TestCase):
    manifest = {"module": "example.invalid/reviewed", "version": "v1.2.3"}

    def test_cold_cache_downloads_selected_version_before_resolving_directory(self):
        selected = {"Path": self.manifest["module"], "Version": "v1.2.3"}
        downloaded = {**selected, "Dir": "/private/module-cache/reviewed"}
        responses = [json.dumps(selected), json.dumps(downloaded)]
        with patch("crs_go.subprocess.check_output", side_effect=responses) as run:
            self.assertEqual(download_module("go", self.manifest), Path(downloaded["Dir"]))
        self.assertEqual(run.call_args_list[1].args[0],
                         ["go", "mod", "download", "-json", "example.invalid/reviewed@v1.2.3"])

    def test_changed_graph_rejects_before_download(self):
        changed = [{"Version": "v1.2.4"}, {"Version": "v1.2.3", "Replace": {"Dir": "/local"}}]
        for selected in changed:
            with self.subTest(selected=selected), patch(
                    "crs_go.subprocess.check_output", return_value=json.dumps(selected)) as run:
                with self.assertRaisesRegex(ValueError, "version/replacement"):
                    download_module("go", self.manifest)
                self.assertEqual(run.call_count, 1)

    def test_download_failure_or_wrong_identity_cannot_supply_overlay(self):
        selected = json.dumps({"Version": "v1.2.3"})
        valid = {"Path": self.manifest["module"], "Version": "v1.2.3", "Dir": "/cache/reviewed"}
        changes = [{"Path": "example.invalid/other"}, {"Version": "v1.2.4"},
                   {"Dir": ""}, {"Error": "failed"}]
        for changed in changes:
            responses = [selected, json.dumps({**valid, **changed})]
            with self.subTest(changed=changed), patch(
                    "crs_go.subprocess.check_output", side_effect=responses):
                with self.assertRaisesRegex(ValueError, "downloaded module"):
                    download_module("go", self.manifest)
        failure = subprocess.CalledProcessError(1, "go")
        with patch("crs_go.subprocess.check_output", side_effect=[selected, failure]):
            with self.assertRaises(subprocess.CalledProcessError):
                download_module("go", self.manifest)

    def test_original_content_must_match_reviewed_hash(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "log.go"
            source.write_bytes(b"reviewed source")
            expected = hashlib.sha256(source.read_bytes()).hexdigest()
            verify_file(source, expected)
            source.write_bytes(b"changed source")
            with self.assertRaisesRegex(ValueError, "SHA256 mismatch"):
                verify_file(source, expected)


if __name__ == "__main__":
    unittest.main()
