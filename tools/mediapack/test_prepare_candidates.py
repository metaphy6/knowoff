"""Executable refusal proofs for the retired playable-image preparation tool."""
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("prepare_candidates.py").resolve()


class PreparationTests(unittest.TestCase):
    def setUp(self):
        run_root = Path("/tmp/agent-runs")
        run_root.mkdir(exist_ok=True)
        self.tmp = tempfile.TemporaryDirectory(prefix="retired-image-tests-", dir=run_root)
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.batch = self.root / "batch"
        self.source = self.root / "source.png"
        self.source.write_bytes(base64.b64decode(
            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a7X8AAAAASUVORK5CYII="))
        self.provenance = self.root / "provenance.json"
        self.provenance.write_text(json.dumps({"tool": "image_gen.imagegen", "prompt": "Exact retained scene prompt."}))

    def run_retired(self, *args):
        result = subprocess.run([sys.executable, "-I", "-S", str(SCRIPT), *map(str, args)],
                                capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("retired", result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertNotIn(str(self.root), result.stderr)
        self.assertFalse(self.batch.exists())
        return result

    def prepare(self, source=None, candidate_id="img-0001", provenance=None):
        return self.run_retired("prepare", "--batch-dir", self.batch,
                                "--image", self.source if source is None else source,
                                "--id", candidate_id, "--provenance",
                                self.provenance if provenance is None else provenance)

    def test_real_image_and_exact_provenance_remain_unchanged(self):
        before = self.source.read_bytes(), self.provenance.read_bytes()
        self.prepare()
        self.prepare()
        self.assertEqual((self.source.read_bytes(), self.provenance.read_bytes()), before)

    def test_all_former_commands_and_unknown_input_refuse_without_site_packages(self):
        for command in ("prepare", "validate", "report", "unknown", "--help"):
            with self.subTest(command=command):
                self.run_retired(command, "--batch-dir", self.batch)
        self.run_retired()

    def test_ids_and_symlinks_cannot_escape_or_mutate_a_batch(self):
        target = self.root / "retained"
        target.mkdir()
        sentinel = target / "keep"
        sentinel.write_bytes(b"retained bytes")
        linked = self.root / "linked.png"
        linked.symlink_to(self.source)
        self.prepare(source=linked, candidate_id="../../escape")
        self.assertEqual(sentinel.read_bytes(), b"retained bytes")
        self.assertFalse((self.root / "escape").exists())

    def test_missing_and_corrupt_provenance_is_never_interpreted(self):
        for data in (b"{}", b"not JSON", b'{"prompt":"different captured words"}'):
            with self.subTest(data=data):
                self.provenance.write_bytes(data)
                self.prepare()
                self.assertEqual(self.provenance.read_bytes(), data)
        self.prepare(provenance=self.root / "missing.json")

    def test_static_animated_and_malformed_binary_inputs_are_all_retired(self):
        blobs = {
            "png": self.source.read_bytes(),
            "gif": base64.b64decode("R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw=="),
            "webp": base64.b64decode("UklGRhwAAABXRUJQVlA4TA8AAAAvAUAAAAcQ/Y/+ByKi/wEA"),
            "animated-webp-marker": b"RIFF\x00\x00\x00\x00WEBPANIM",
            "apng-marker": b"\x89PNG\r\n\x1a\nacTL",
            "jpeg-marker": b"\xff\xd8\xff\xd9",
            "malformed": b"not an image",
        }
        for name, blob in blobs.items():
            with self.subTest(name=name):
                source = self.root / name
                source.write_bytes(blob)
                before = hashlib.sha256(blob).digest()
                self.prepare(source=source)
                self.assertEqual(hashlib.sha256(source.read_bytes()).digest(), before)

    def test_fifo_source_is_not_opened_and_no_preparation_blocks(self):
        source = self.root / "blocking-source"
        os.mkfifo(source)
        self.prepare(source=source)

    def test_retained_receipts_and_corrupt_candidate_reports_are_not_rewritten(self):
        retained = self.root / "historical-batch"
        (retained / "generated/receipts").mkdir(parents=True)
        receipt = retained / "generated/receipts/img-0001.json"
        receipt.write_text('{"source":{"path":"../source.png"},"state":"prepared_candidate"}')
        candidates = retained / "candidates-a.jsonl"
        candidates.write_text('{"id":"img-0001"}\n{"id":"img-0001"}\n')
        before = receipt.read_bytes(), candidates.read_bytes()
        for command in ("validate", "report"):
            self.run_retired(command, "--batch-dir", retained)
        self.assertEqual((receipt.read_bytes(), candidates.read_bytes()), before)

    def test_entrypoint_cannot_open_inputs_or_make_network_calls(self):
        audit = """import runpy, sys
script, source, provenance, batch = sys.argv[1:]
def audit(event, args):
    if event == 'open' and str(args[0]) in (source, provenance, batch):
        raise AssertionError('retired input was opened')
    if event.startswith('socket.'):
        raise AssertionError('retired network access')
sys.addaudithook(audit)
sys.argv = [script, 'prepare', '--image', source, '--provenance', provenance, '--batch-dir', batch, '--id', 'img-0001']
runpy.run_path(script, run_name='__main__')
"""
        result = subprocess.run([sys.executable, "-I", "-S", "-c", audit, str(SCRIPT),
                                 str(self.source), str(self.provenance), str(self.batch)],
                                capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("retired", result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertFalse(self.batch.exists())


if __name__ == "__main__":
    unittest.main()
