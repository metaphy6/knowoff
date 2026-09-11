"""Behavior checks for the local, non-loadable image candidate preparation tool."""

import base64
import hashlib
import io
import json
import struct
import tempfile
import unittest
import zlib
from pathlib import Path

from PIL import Image

import prepare_candidates as preparation


class PreparationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.batch = self.root / "batch"
        self.source = self.root / "source.png"
        picture = Image.new("RGB", (1200, 800), (10, 180, 80))
        picture.save(self.source, pnginfo=None)
        self.provenance = {"tool": "image_gen.imagegen", "prompt": "Exact scene prompt."}

    def prepare(self, **kwargs):
        return preparation.prepare(
            self.batch, kwargs.get("image", self.source),
            kwargs.get("candidate_id", "img-0001"),
            kwargs.get("provenance", self.provenance),
        )

    def test_real_image_is_resized_and_original_and_hashes_are_retained(self):
        receipt = self.prepare()
        self.assertEqual(receipt["state"], "prepared_candidate")
        self.assertIsNone(receipt["generation"]["model"])
        self.assertEqual(receipt["generation"]["prompt"], "Exact scene prompt.")
        original = self.batch / receipt["source"]["path"]
        self.assertEqual(original.read_bytes(), self.source.read_bytes())
        asset = self.batch / receipt["asset"]["path"]
        self.assertEqual(hashlib.sha256(asset.read_bytes()).hexdigest(), receipt["asset"]["sha256"])
        with Image.open(asset) as converted:
            self.assertEqual(converted.format, "WEBP")
            self.assertEqual(converted.size, (720, 480))
            self.assertFalse(converted.getexif())
        self.assertEqual(receipt["asset"]["bytes"], asset.stat().st_size)
        self.assertFalse((self.batch / "manifest.json").exists())

    def test_same_input_is_idempotent_but_revised_input_never_overwrites(self):
        first = self.prepare()
        second = self.prepare()
        self.assertEqual(first, second)
        receipt_path = self.batch / "generated/receipts/img-0001.json"
        receipt_before = receipt_path.read_bytes()
        with self.assertRaisesRegex(ValueError, "conflict"):
            self.prepare(provenance={"tool": "image_gen.imagegen", "prompt": "Changed prompt"})
        self.assertEqual(receipt_path.read_bytes(), receipt_before)
        Image.new("RGB", (30, 20), "blue").save(self.source)
        with self.assertRaisesRegex(ValueError, "conflict"):
            self.prepare()
        self.assertEqual(receipt_path.read_bytes(), receipt_before)

    def test_ids_and_symlinks_cannot_escape_batch(self):
        with self.assertRaisesRegex(ValueError, "id"):
            self.prepare(candidate_id="../../escape")
        (self.batch / "generated").mkdir(parents=True)
        (self.batch / "generated/sources").symlink_to(self.root, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, "outside|symlink"):
            self.prepare()
        self.assertFalse((self.root / "img-0001.png").exists())

    def test_animated_media_and_missing_prompt_are_rejected(self):
        with self.assertRaisesRegex(ValueError, "prompt"):
            self.prepare(provenance={"tool": "image_gen.imagegen"})
        gif = self.root / "animated.gif"
        Image.new("RGB", (20, 20), "red").save(
            gif, save_all=True, append_images=[Image.new("RGB", (20, 20), "blue")],
            duration=100, loop=0,
        )
        with self.assertRaisesRegex(ValueError, "animated"):
            self.prepare(image=gif)

    def test_static_gif_and_animated_webp_or_png_cannot_enter_image_pipeline(self):
        for format_name, animated in [("GIF", False), ("WEBP", True), ("PNG", True)]:
            with self.subTest(format=format_name):
                # A misleading extension must not bypass inspection of the bytes.
                source = self.root / "disguised.png"
                Image.new("RGB", (20, 20), "red").save(
                    source, format=format_name, save_all=animated,
                    append_images=[Image.new("RGB", (20, 20), "blue")] if animated else [],
                    duration=100, loop=0,
                )
                with self.assertRaisesRegex(ValueError, "static|animated"):
                    self.prepare(image=source)
                self.assertFalse((self.batch / "generated").exists())

    def test_single_frame_animation_containers_are_rejected(self):
        # One-frame APNGs and animated WebPs report n_frames=1 and
        # is_animated=False in Pillow, but their containers still carry animation.
        def png_chunk(kind, payload):
            return (struct.pack(">I", len(payload)) + kind + payload
                    + struct.pack(">I", zlib.crc32(kind + payload)))

        png = io.BytesIO()
        Image.new("RGB", (2, 2), "red").save(png, format="PNG")
        original = png.getvalue()
        apng = (original[:33]
                + png_chunk(b"acTL", struct.pack(">II", 1, 0))
                + png_chunk(b"fcTL", struct.pack(">IIIIIHHBB", 0, 2, 2, 0, 0, 100, 1000, 0, 0))
                + original[33:])
        two_frames = base64.b64decode(
            "UklGRoQAAABXRUJQVlA4WAoAAAACAAAAAQAAAQAAQU5JTQYAAAAAAAAAAABBTk1GKAAAAAAAAAAAAAEAAAEAAGQAAAJWUDhMDwAAAC8BQAAABxD9j/4HIqL/AQBBTk1GKAAAAAAAAAAAAAEAAAEAAGQAAABWUDhMDwAAAC8BQAAABxDR//4HIqL/AQA=")
        chunks, offset = [], 12
        while offset < len(two_frames):
            size = int.from_bytes(two_frames[offset + 4:offset + 8], "little")
            end = offset + 8 + size + size % 2
            chunks.append(two_frames[offset:end])
            if two_frames[offset:offset + 4] == b"ANMF":
                break
            offset = end
        payload = b"WEBP" + b"".join(chunks)
        animated_webp = b"RIFF" + struct.pack("<I", len(payload)) + payload

        for name, data in [("APNG", apng), ("WEBP", animated_webp)]:
            with self.subTest(format=name):
                with Image.open(io.BytesIO(data)) as picture:
                    picture.load()
                    self.assertEqual(picture.n_frames, 1)
                    self.assertFalse(picture.is_animated)
                source = self.root / f"{name}.png"
                source.write_bytes(data)
                with self.assertRaisesRegex(ValueError, "animated"):
                    self.prepare(image=source, candidate_id=name)
                self.assertFalse((self.batch / f"generated/receipts/{name}.json").exists())

    def test_static_png_jpeg_webp_remain_supported(self):
        for format_name in ["PNG", "JPEG", "WEBP"]:
            with self.subTest(format=format_name):
                source = self.root / f"static-{format_name}"
                Image.new("RGB", (2, 2), "red").save(source, format=format_name)
                receipt = self.prepare(image=source, candidate_id=format_name)
                self.assertEqual(receipt["source"]["format"], format_name)
                self.assertEqual(receipt["asset"]["format"], "WEBP")

    def test_report_exposes_missing_images_and_detects_corruption(self):
        receipt = self.prepare()
        candidates = self.batch / "candidates-a.jsonl"
        candidates.write_text('\n'.join(json.dumps({"id": ident}) for ident in ["img-0001", "img-0002"]) + '\n')
        report = preparation.validate(self.batch)
        self.assertEqual(report["expected_images"], 2)
        self.assertEqual(report["prepared_images"], 1)
        self.assertEqual(report["missing_images"], ["img-0002"])
        self.assertFalse(report["complete"])
        self.assertEqual(report["errors"], [])
        (self.batch / receipt["asset"]["path"]).write_bytes(b"broken image")
        corrupted = preparation.validate(self.batch)
        self.assertTrue(corrupted["errors"])
        self.assertEqual(corrupted["prepared_images"], 0)
        self.assertIn("img-0001", corrupted["missing_images"])

    def test_report_rejects_receipt_path_traversal_and_duplicate_candidates(self):
        self.prepare()
        receipt_path = self.batch / "generated/receipts/img-0001.json"
        receipt = json.loads(receipt_path.read_text())
        receipt["source"]["path"] = "../source.png"
        receipt_path.write_text(json.dumps(receipt))
        candidates = self.batch / "candidates-a.jsonl"
        candidates.write_text('{"id":"img-0001"}\n{"id":"img-0001"}\n')
        report = preparation.validate(self.batch)
        self.assertTrue(any("outside" in error for error in report["errors"]))
        self.assertTrue(any("duplicate" in error for error in report["errors"]))


if __name__ == "__main__":
    unittest.main()
