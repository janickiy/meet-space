#!/usr/bin/env python3
"""Read-only PDF QA: text bounds, glyphs, exact raster hashes, visual previews.

Creates only reports and PNG previews in --output-dir. Does not modify the PDF,
Markdown, original PDF, or any diagram asset. Poppler is preferred for previews.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import re
import shutil
import subprocess
from collections import Counter
from pathlib import Path

import pdfplumber
from PIL import Image, ImageDraw, ImageFont
from pypdf import PdfReader

POPPLER = shutil.which("pdftoppm")


def file_hash(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def pixel_hash(image):
    rgba = image.convert("RGBA")
    header = f"RGBA:{rgba.width}x{rgba.height}:".encode()
    return hashlib.sha256(header + rgba.tobytes()).hexdigest()


def pdf_images(reader):
    result = []
    seen = {}
    for number, page in enumerate(reader.pages, 1):
        for item in page.images:
            reference = getattr(item, "indirect_reference", None)
            key = (reference.idnum, reference.generation) if reference is not None else item.name
            if key in seen:
                seen[key]["pages"].append(number)
                continue
            record = {"name": item.name, "pages": [number],
                "size": list(item.image.size), "mode": item.image.mode,
                "rgba_sha256": pixel_hash(item.image)}
            result.append(record)
            seen[key] = record
    return result


def text_checks(path):
    pages = []
    bounds = []
    missing = []
    image_bounds = []
    with pdfplumber.open(path) as pdf:
        for number, page in enumerate(pdf.pages, 1):
            chars = page.chars
            body_chars = [c for c in chars if 40 <= c["top"] <= page.height - 40]
            text = page.extract_text() or ""
            for image in page.images:
                if image["x0"] < 46 or image["x1"] > page.width - 46 or image["top"] < 40 or image["bottom"] > page.height - 40:
                    image_bounds.append({"page": number, **{k: image[k] for k in ("x0", "x1", "top", "bottom")}})
            for char in chars:
                if char.get("text", "") in ("\x00", "\ufffd", "\u25a0"):
                    missing.append({"page": number, "text": char["text"],
                        "x": char["x0"], "top": char["top"]})
                # Cover uses a different composition, but still shares safe margins.
                if char["x0"] < 46 or char["x1"] > page.width - 46 or char["top"] < 12 or char["bottom"] > page.height - 12:
                    bounds.append({"page": number, "text": char["text"],
                        "x0": round(char["x0"], 2), "x1": round(char["x1"], 2),
                        "top": round(char["top"], 2), "bottom": round(char["bottom"], 2)})
            pages.append({"page": number, "char_count": len(chars),
                "body_char_count": len(body_chars), "image_count": len(page.images),
                "width": page.width, "height": page.height,
                "image_placements": [{k: image[k] for k in ("x0", "top", "width", "height")} for image in page.images],
                "body_top": round(min((c["top"] for c in body_chars), default=0), 2),
                "body_bottom": round(max((c["bottom"] for c in body_chars), default=0), 2),
                "first_text": text[:240], "text_sha256": hashlib.sha256(text.encode()).hexdigest()})
            # Close cached object graphs to keep memory bounded on 200+ pages.
            page.close()
    return {"pages": pages, "out_of_safe_bounds_count": len(bounds),
        "out_of_safe_bounds": bounds[:200], "missing_glyph_count": len(missing),
        "missing_glyphs": missing[:200],
        "image_out_of_safe_bounds_count": len(image_bounds),
        "images_out_of_safe_bounds": image_bounds,
        "no_content_pages": [p["page"] for p in pages if p["body_char_count"] < 5 and p["image_count"] == 0]}


def image_placements(path):
    result = {}
    with pdfplumber.open(path) as pdf:
        for number, page in enumerate(pdf.pages, 1):
            if page.images:
                result[number] = [{k: image[k] for k in ("x0", "top", "width", "height")} for image in page.images]
            page.close()
    return result


def create_previews(pdf, out, count, pages, all_previews):
    if not POPPLER:
        raise SystemExit("pdftoppm not found. Install Poppler and add it to PATH.")
    preview_dir = out / "previews"
    preview_dir.mkdir(exist_ok=True)
    if all_previews:
        contact_pages = preview_dir / "contact-pages"
        contact_pages.mkdir(exist_ok=True)
        subprocess.run([str(POPPLER), "-r", "36", "-png", str(pdf),
            str(contact_pages / "page")], check=True, stdout=subprocess.DEVNULL)
        files = sorted(contact_pages.glob("page-*.png"),
            key=lambda path: int(path.stem.rsplit("-", 1)[1]))
        columns, rows = 5, 5
        thumb_w, thumb_h = 180, 255
        tile_w, tile_h = 196, 282
        if files:
            with Image.open(files[0]) as first_page:
                if first_page.width > first_page.height:
                    thumb_w, thumb_h = 230, 163
                    tile_w, tile_h = 246, 190
        font = ImageFont.load_default(size=12)
        contact_paths = []
        for begin in range(0, len(files), columns * rows):
            batch = files[begin:begin + columns * rows]
            sheet = Image.new("RGB", (columns * tile_w, math.ceil(len(batch) / columns) * tile_h), "#dce3eb")
            draw = ImageDraw.Draw(sheet)
            for index, path in enumerate(batch):
                with Image.open(path) as page_image:
                    page_image.thumbnail((thumb_w, thumb_h), Image.Resampling.LANCZOS)
                    x = (index % columns) * tile_w + (tile_w - page_image.width) // 2
                    y = (index // columns) * tile_h + 5
                    sheet.paste(page_image.convert("RGB"), (x, y))
                    number = int(path.stem.rsplit("-", 1)[1])
                    draw.text((x, y + thumb_h + 5), str(number), font=font, fill="#243247")
            target = preview_dir / f"contact-{begin + 1:03d}-{begin + len(batch):03d}.png"
            sheet.save(target)
            contact_paths.append(str(target))
    else:
        contact_paths = []
    full_paths = []
    for number in sorted(set(pages)):
        if not 1 <= number <= count:
            continue
        prefix = preview_dir / f"page-{number:03d}"
        subprocess.run([str(POPPLER), "-f", str(number), "-l", str(number),
            "-r", "120", "-singlefile", "-png", str(pdf), str(prefix)],
            check=True, stdout=subprocess.DEVNULL)
        full_paths.append(str(prefix.with_suffix(".png")))
    return {"contacts": contact_paths, "full_pages": full_paths}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pdf", type=Path, required=True)
    parser.add_argument("--manifest", type=Path)
    parser.add_argument("--original", type=Path)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--render", action="store_true", help="Render representative full pages")
    parser.add_argument("--contacts", action="store_true", help="Render low-resolution contact sheets of every page")
    parser.add_argument("--pages", default="", help="Comma-separated additional page numbers")
    args = parser.parse_args()
    pdf = args.pdf.resolve()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    manifest_path = args.manifest or pdf.with_suffix(".render.json")
    manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else {}
    reader = PdfReader(pdf)
    report = {"pdf": str(pdf), "sha256": file_hash(pdf), "page_count": len(reader.pages),
        "metadata": {str(k): str(v) for k, v in (reader.metadata or {}).items()}}
    report["text"] = text_checks(pdf)
    size_counts = Counter((round(float(page.mediabox.width), 4), round(float(page.mediabox.height), 4)) for page in reader.pages)
    report["page_sizes"] = [{"width_pt": size[0], "height_pt": size[1], "pages": count} for size, count in size_counts.items()]
    report["uniform_page_size"] = len(size_counts) == 1
    report["all_pages_landscape_a4"] = all(abs(width - 841.8898) < .001 and abs(height - 595.2756) < .001 for width, height in size_counts)
    report["images"] = pdf_images(reader)
    original_images = []
    original_placements = {}
    if args.original:
        original_reader = PdfReader(args.original)
        original_images = pdf_images(original_reader)
        original_placements = image_placements(args.original)
        report["original"] = {"pdf": str(args.original.resolve()),
            "sha256": file_hash(args.original), "page_count": len(original_reader.pages),
            "images": original_images}
    hashes = {record["rgba_sha256"] for record in report["images"]}
    original_hashes = {record["rgba_sha256"] for record in original_images}
    report["source_assets"] = []
    for asset in manifest.get("assets", []):
        path = Path(asset["path"])
        with Image.open(path) as image:
            digest = pixel_hash(image)
        report["source_assets"].append({"path": str(path), "rgba_sha256": digest,
            "file_unchanged_since_render": file_hash(path) == asset["sha256"],
            "exact_rgba_pixels_in_pdf": digest in hashes,
            "exact_rgba_pixels_in_original": digest in original_hashes if args.original else None})
    report["all_source_pixels_preserved"] = bool(report["source_assets"]) and all(
        entry["exact_rgba_pixels_in_pdf"] and entry["file_unchanged_since_render"]
        for entry in report["source_assets"])
    report["original_image_set_preserved"] = original_hashes == hashes if args.original else None
    report["image_display_comparison"] = []
    original_by_hash = {record["rgba_sha256"]: record for record in original_images}
    page_by_number = {record["page"]: record for record in report["text"]["pages"]}
    for record in report["images"]:
        original = original_by_hash.get(record["rgba_sha256"])
        current_boxes = page_by_number[record["pages"][0]]["image_placements"]
        previous_boxes = original_placements.get(original["pages"][0], []) if original else []
        # All original diagrams occupy a page's single raster image resource.
        if len(current_boxes) == 1 and len(previous_boxes) == 1:
            current, previous = current_boxes[0], previous_boxes[0]
            report["image_display_comparison"].append({"rgba_sha256": record["rgba_sha256"],
                "page": record["pages"][0], "original_page": original["pages"][0],
                "current": current, "original": previous,
                "same_display_size": all(abs(current[key] - previous[key]) < .001 for key in ("width", "height")),
                "same_aspect_ratio": abs(current["width"] / current["height"] - previous["width"] / previous["height"]) < .00001})
    report["original_image_display_sizes_preserved"] = bool(report["image_display_comparison"]) and all(
        item["same_display_size"] and item["same_aspect_ratio"] for item in report["image_display_comparison"])
    report["render_warnings"] = manifest.get("warnings", [])
    report["glyph_substitutions"] = manifest.get("missing_glyph_substitutions", {})
    report["source_unchanged_since_render"] = file_hash(manifest["source"]) == manifest.get("source_sha256") if manifest.get("source") else None
    if args.render or args.contacts:
        representative = {1, 2, 3, 8, 9, len(reader.pages)}
        representative.update(int(p) for p in args.pages.split(",") if p)
        representative.update(record["pages"][0] for record in report["images"])
        densest = sorted(report["text"]["pages"], key=lambda page: page["char_count"], reverse=True)[:5]
        representative.update(page["page"] for page in densest)
        report["previews"] = create_previews(pdf, args.output_dir,
            len(reader.pages), representative if args.render else [], args.contacts)
    report_path = args.output_dir / "qa-report.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    summary = {"report": str(report_path.resolve()), "page_count": report["page_count"],
        "uniform_page_size": report["uniform_page_size"],
        "all_pages_landscape_a4": report["all_pages_landscape_a4"],
        "page_sizes": report["page_sizes"],
        "unique_pdf_images": len(report["images"]),
        "source_images": len(report["source_assets"]),
        "all_source_pixels_preserved": report["all_source_pixels_preserved"],
        "original_image_set_preserved": report["original_image_set_preserved"],
        "original_image_display_sizes_preserved": report["original_image_display_sizes_preserved"],
        "out_of_safe_bounds_count": report["text"]["out_of_safe_bounds_count"],
        "image_out_of_safe_bounds_count": report["text"]["image_out_of_safe_bounds_count"],
        "missing_glyph_count": report["text"]["missing_glyph_count"],
        "no_content_pages": report["text"]["no_content_pages"],
        "source_unchanged_since_render": report["source_unchanged_since_render"]}
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
