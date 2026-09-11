#!/usr/bin/env python3
"""Prepare and audit local image candidates; this does not create playable packs.

Requires Pillow with WebP support. No network or generation calls are performed.
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
import re
import tempfile
import warnings
from datetime import datetime, timezone
from pathlib import Path

from PIL import Image, ImageOps, __version__ as pillow_version


MAX_SIDE = 720
QUALITY = 80
ID_PATTERN = re.compile(r"[A-Za-z0-9][A-Za-z0-9_-]{0,95}\Z")


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _id(value: str) -> str:
    if not isinstance(value, str) or not ID_PATTERN.fullmatch(value):
        raise ValueError("candidate id must be 1–96 letters, digits, underscores or hyphens")
    return value


def _inside(root: Path, relative: str) -> Path:
    path = Path(relative)
    if path.is_absolute() or ".." in path.parts:
        raise ValueError(f"path outside batch: {relative}")
    candidate = root / path
    if not candidate.resolve().is_relative_to(root.resolve()):
        raise ValueError(f"symlink points outside batch: {relative}")
    return candidate


def _generation(provenance: dict) -> dict:
    if not isinstance(provenance, dict):
        raise ValueError("provenance must be a JSON object")
    for field in ("tool", "prompt"):
        if not isinstance(provenance.get(field), str) or not provenance[field].strip():
            raise ValueError(f"provenance requires exact {field}")
    allowed = {"tool", "prompt", "model", "parameters", "seed"}
    if set(provenance) - allowed:
        raise ValueError("unsupported provenance fields: " + ", ".join(sorted(set(provenance) - allowed)))
    if provenance.get("model") is not None and not isinstance(provenance["model"], str):
        raise ValueError("reported model must be a string or null")
    if provenance.get("parameters") is not None and not isinstance(provenance["parameters"], dict):
        raise ValueError("reported parameters must be an object or null")
    return {key: provenance.get(key) for key in ("tool", "prompt", "model", "parameters", "seed")}


def _image_info(data: bytes) -> dict:
    with warnings.catch_warnings():
        warnings.simplefilter("error", Image.DecompressionBombWarning)
        with Image.open(io.BytesIO(data)) as picture:
            picture.load()
            if getattr(picture, "n_frames", 1) != 1:
                raise ValueError("animated images require the separate GIF pipeline")
            return {"format": picture.format, "width": picture.width, "height": picture.height}


def _write_once(path: Path, data: bytes) -> None:
    """Install complete bytes atomically, without replacing an existing file."""
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=".preparing-", dir=path.parent)
    try:
        with os.fdopen(descriptor, "wb") as output:
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
        try:
            os.link(temporary, path)
        except FileExistsError:
            if path.read_bytes() != data:
                raise ValueError(f"existing file conflict: {path}") from None
    finally:
        os.unlink(temporary)


def _validate_receipt(batch: Path, receipt: dict, expected_id: str) -> None:
    if receipt["id"] != _id(expected_id) or receipt["state"] != "prepared_candidate":
        raise ValueError("receipt id/state mismatch")
    if receipt["schema_version"] != 1:
        raise ValueError("unsupported receipt schema")
    _generation(receipt["generation"])
    for kind in ("source", "asset"):
        item = receipt[kind]
        path = _inside(batch, item["path"])
        data = path.read_bytes()
        if _sha256(data) != item["sha256"] or len(data) != item["bytes"]:
            raise ValueError(f"{kind} checksum/size mismatch")
        info = _image_info(data)
        if any(info[field] != item[field] for field in ("format", "width", "height")):
            raise ValueError(f"{kind} image metadata mismatch")
        if kind == "asset":
            if info["format"] != "WEBP" or max(info["width"], info["height"]) > MAX_SIDE:
                raise ValueError("asset must be WebP with longest side at most 720px")
            with Image.open(io.BytesIO(data)) as picture:
                if picture.getexif():
                    raise ValueError("asset retains EXIF metadata")


def prepare(batch_dir: Path, image: Path, candidate_id: str, provenance: dict) -> dict:
    """Preserve an original and transcode one static candidate, never approve it."""
    batch = Path(batch_dir).resolve()
    candidate_id = _id(candidate_id)
    generation = _generation(provenance)
    source_data = Path(image).read_bytes()
    source_hash = _sha256(source_data)
    receipt_path = _inside(batch, f"generated/receipts/{candidate_id}.json")
    if receipt_path.exists():
        receipt = json.loads(receipt_path.read_text(encoding="utf-8"))
        if receipt.get("source", {}).get("sha256") != source_hash or receipt.get("generation") != generation:
            raise ValueError(f"candidate id conflict: {candidate_id}; use a new revision id")
        _validate_receipt(batch, receipt, candidate_id)
        return receipt

    source_info = _image_info(source_data)
    extension = {"JPEG": "jpg", "PNG": "png", "WEBP": "webp", "GIF": "gif"}.get(source_info["format"])
    if extension is None:
        raise ValueError("input must be a static PNG, JPEG, WebP or GIF")
    with Image.open(io.BytesIO(source_data)) as original:
        picture = ImageOps.exif_transpose(original)
        picture = picture.convert("RGBA" if "A" in picture.getbands() or "transparency" in picture.info else "RGB")
        picture.thumbnail((MAX_SIDE, MAX_SIDE), Image.Resampling.LANCZOS)
        # A fresh pixel image removes inherited EXIF, XMP and ICC metadata.
        clean = Image.new(picture.mode, picture.size)
        clean.paste(picture)
        stream = io.BytesIO()
        clean.save(stream, format="WEBP", quality=QUALITY, method=6)
    asset_data = stream.getvalue()
    asset_hash = _sha256(asset_data)
    source_relative = f"generated/sources/{candidate_id}.{extension}"
    asset_relative = f"generated/assets/{asset_hash}.webp"
    source_path = _inside(batch, source_relative)
    asset_path = _inside(batch, asset_relative)
    receipt = {
        "schema_version": 1,
        "id": candidate_id,
        "state": "prepared_candidate",
        "prepared_at": datetime.now(timezone.utc).isoformat(),
        "generation": generation,
        "processing": {"tool": "Pillow", "version": pillow_version, "max_side": MAX_SIDE,
                       "format": "WEBP", "quality": QUALITY, "method": 6},
        "source": {"path": source_relative, "sha256": source_hash, "bytes": len(source_data), **source_info},
        "asset": {"path": asset_relative, "sha256": asset_hash, "bytes": len(asset_data), **_image_info(asset_data)},
    }
    _write_once(source_path, source_data)
    _write_once(asset_path, asset_data)
    _write_once(receipt_path, (json.dumps(receipt, ensure_ascii=False, indent=2, allow_nan=False) + "\n").encode("utf-8"))
    return receipt


def validate(batch_dir: Path, candidate_files: list[Path] | None = None) -> dict:
    """Verify receipts/files and explicitly count candidates without valid images."""
    batch = Path(batch_dir).resolve()
    errors = []
    expected = set()
    files = candidate_files if candidate_files is not None else sorted(batch.glob("candidates*.jsonl"))
    for file in files:
        try:
            for number, line in enumerate(Path(file).read_text(encoding="utf-8").splitlines(), 1):
                if not line.strip():
                    continue
                record = json.loads(line)
                ident = _id(record.get("id", record.get("candidate_id")))
                if ident in expected:
                    errors.append(f"{Path(file).name}:{number}: duplicate candidate id {ident}")
                expected.add(ident)
        except (OSError, ValueError, TypeError, AttributeError) as error:
            errors.append(f"{Path(file).name}: {error}")
    valid = []
    receipt_count = 0
    try:
        directory = _inside(batch, "generated/receipts")
        for file in sorted(directory.glob("*.json")):
            receipt_count += 1
            try:
                _inside(batch, str(file.relative_to(batch)))
                receipt = json.loads(file.read_text(encoding="utf-8"))
                _validate_receipt(batch, receipt, file.stem)
                valid.append(file.stem)
            except (OSError, ValueError, TypeError, KeyError, Image.DecompressionBombWarning) as error:
                errors.append(f"{file.name}: {error}")
    except ValueError as error:
        errors.append(str(error))
    missing = sorted(expected - set(valid))
    return {
        "state": "candidate_preparation_audit",
        "expected_images": len(expected) if files else None,
        "receipt_count": receipt_count,
        "prepared_images": len(valid),
        "missing_images": missing,
        "unmapped_images": sorted(set(valid) - expected) if files else [],
        "errors": errors,
        "complete": bool(files) and bool(expected) and not missing and not errors,
        "release_checks": "not run; no screening, rights clearance, human review, playtests, embeddings, certification or activation implied",
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    prepare_parser = commands.add_parser("prepare", help="preserve and transcode one image")
    prepare_parser.add_argument("--batch-dir", required=True, type=Path)
    prepare_parser.add_argument("--image", required=True, type=Path)
    prepare_parser.add_argument("--id", required=True)
    prepare_parser.add_argument("--provenance", required=True, type=Path,
                                help="JSON containing exact tool and prompt; model, parameters and seed optional")
    for command in ("validate", "report"):
        report_parser = commands.add_parser(command, help="audit all receipts and candidate JSONL")
        report_parser.add_argument("--batch-dir", required=True, type=Path)
        report_parser.add_argument("--candidates", action="append", type=Path,
                                   help="candidate JSONL; defaults to batch candidates*.jsonl")
    args = parser.parse_args(argv)
    try:
        if args.command == "prepare":
            provenance = json.loads(args.provenance.read_text(encoding="utf-8"))
            result = prepare(args.batch_dir, args.image, args.id, provenance)
            status = 0
        else:
            result = validate(args.batch_dir, args.candidates)
            status = 0 if result["complete"] else 1
        print(json.dumps(result, ensure_ascii=False, indent=2, allow_nan=False))
        return status
    except (OSError, ValueError, TypeError, KeyError, Image.DecompressionBombWarning) as error:
        parser.exit(2, f"error: {error}\n")


if __name__ == "__main__":
    raise SystemExit(main())
