#!/usr/bin/env python3
"""Build the complete MeetSpace reference from Markdown and unchanged PNG assets.

Run from any directory with Python 3.10+ and requirements-pdf.txt installed.
The default layout is uniform landscape A4. Diagnostics accompany the PDF.
"""
from __future__ import annotations

import argparse
import hashlib
import html
import json
import math
import os
import re
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path

from PIL import Image as PILImage
from reportlab import rl_config
from reportlab.lib import colors
from reportlab.lib.enums import TA_LEFT
from reportlab.lib.pagesizes import A4, landscape
from reportlab.lib.styles import ParagraphStyle
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (
    BaseDocTemplate, Flowable, Frame, Image, KeepTogether, NextPageTemplate,
    PageBreak, PageTemplate, Paragraph, Spacer, Table, TableStyle,
)

PAGE_W, PAGE_H = A4
MARGIN = 48
BODY_W = PAGE_W - 2 * MARGIN
BLUE = colors.HexColor("#254e74")
INK = colors.HexColor("#243247")
MUTED = colors.HexColor("#64748b")
PALE = colors.HexColor("#f1f5f9")
RULE = colors.HexColor("#d3deea")
DOCS = Path(__file__).resolve().parents[1]
SOURCE_COMMIT = "189631b5a50665c8c6b8725cfa21f3d46e895e52"
rl_config.invariant = 1


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def register_fonts(font_dir=None):
    """Register a complete Cyrillic font family and return exact font hashes."""
    arial = {
        "Doc": "Arial.ttf", "DocBold": "Arial Bold.ttf",
        "DocItalic": "Arial Italic.ttf", "DocBoldItalic": "Arial Bold Italic.ttf",
        "Mono": "Courier New.ttf", "MonoBold": "Courier New Bold.ttf",
        "MonoItalic": "Courier New Italic.ttf",
        "MonoBoldItalic": "Courier New Bold Italic.ttf",
        "Unicode": "Arial Unicode.ttf",
    }
    dejavu = {
        "Doc": "DejaVuSans.ttf", "DocBold": "DejaVuSans-Bold.ttf",
        "DocItalic": "DejaVuSans-Oblique.ttf",
        "DocBoldItalic": "DejaVuSans-BoldOblique.ttf",
        "Mono": "DejaVuSansMono.ttf", "MonoBold": "DejaVuSansMono-Bold.ttf",
        "MonoItalic": "DejaVuSansMono-Oblique.ttf",
        "MonoBoldItalic": "DejaVuSansMono-BoldOblique.ttf",
        "Unicode": "DejaVuSans.ttf",
    }
    roots = [Path(font_dir)] if font_dir else [
        Path("/System/Library/Fonts/Supplemental"),
        Path("/usr/share/fonts/truetype/dejavu"),
        Path("/usr/local/share/fonts/dejavu"),
        Path("C:/Windows/Fonts"),
    ]
    selected = None
    for root in roots:
        for family in (arial, dejavu):
            if all((root / filename).is_file() for filename in family.values()):
                selected = {name: root / filename for name, filename in family.items()}
                break
        if selected:
            break
    if not selected:
        raise SystemExit(
            "Cyrillic fonts not found. Install fonts-dejavu-core or pass --font-dir "
            "with the complete DejaVu Sans / Sans Mono family (see DOCUMENTATION_BUILD.md)."
        )
    for name, path in selected.items():
        pdfmetrics.registerFont(TTFont(name, str(path)))
    pdfmetrics.registerFontFamily("Doc", normal="Doc", bold="DocBold",
        italic="DocItalic", boldItalic="DocBoldItalic")
    pdfmetrics.registerFontFamily("Mono", normal="Mono", bold="MonoBold",
        italic="MonoItalic", boldItalic="MonoBoldItalic")
    return {name: {"filename": path.name, "sha256": sha256(path)}
            for name, path in selected.items()}


def styles():
    normal = ParagraphStyle("Body", fontName="Doc", fontSize=10.1, leading=14.1,
        textColor=INK, spaceAfter=8, allowWidows=0, allowOrphans=0,
        splitLongWords=1, embeddedHyphenation=0, hyphenationLang=None)
    out = {"body": normal}
    out["h1"] = ParagraphStyle("H1", parent=normal, fontName="DocBold",
        fontSize=21, leading=26, textColor=BLUE, spaceAfter=14,
        spaceBefore=8, keepWithNext=1)
    out["h2"] = ParagraphStyle("H2", parent=normal, fontName="DocBold",
        fontSize=15.3, leading=20, textColor=BLUE, spaceBefore=15,
        spaceAfter=9, keepWithNext=1)
    out["h3"] = ParagraphStyle("H3", parent=normal, fontName="DocBold",
        fontSize=11.9, leading=16, textColor=BLUE, spaceBefore=11,
        spaceAfter=6, keepWithNext=1)
    out["h4"] = ParagraphStyle("H4", parent=out["h3"], fontSize=10.5,
        leading=14.1)
    out["caption"] = ParagraphStyle("Caption", parent=normal, fontSize=8.6,
        leading=11.7, textColor=MUTED, spaceBefore=5, spaceAfter=12)
    out["toc"] = ParagraphStyle("TOC", parent=normal, fontSize=9.5,
        leading=12.6, spaceAfter=3, textColor=BLUE)
    out["list"] = ParagraphStyle("List", parent=normal, leftIndent=13,
        firstLineIndent=-10, spaceAfter=5)
    out["quote"] = ParagraphStyle("Quote", parent=normal, leftIndent=12,
        borderColor=RULE, borderWidth=0.6, borderPadding=8, backColor=PALE)
    return out


class Formatter:
    def __init__(self):
        self.missing = Counter()
        self.fallback = Counter()

    def plain(self, value, font="Doc", normalize=True):
        value = html.unescape(value)
        if normalize:
            value = value.translate(str.maketrans({c: "-" for c in "‐‑‒–—"}))
        glyphs = pdfmetrics.getFont(font).face.charToGlyph
        extra = pdfmetrics.getFont("Unicode").face.charToGlyph
        result = []
        for ch in value:
            cp = ord(ch)
            if ch in "\n\r\t":
                result.append(ch)
            elif cp in glyphs:
                result.append(html.escape(ch, quote=False))
            elif cp in extra:
                self.fallback[f"U+{cp:04X}"] += 1
                result.append('<font name="Unicode">' + html.escape(ch, quote=False) + '</font>')
            else:
                self.missing[f"U+{cp:04X}"] += 1
                result.append(f"U+{cp:04X}")
        return "".join(result)

    def inline(self, text):
        """Small deterministic inline parser; code spans take precedence."""
        pieces = []
        i = 0
        literal = []

        def flush():
            if literal:
                pieces.append(self.plain("".join(literal)))
                literal.clear()

        while i < len(text):
            if text[i] == "\\" and i + 1 < len(text) and text[i + 1] in "|`*_[]\\":
                literal.append(text[i + 1]); i += 2; continue
            if text[i] == "`":
                run = len(text[i:]) - len(text[i:].lstrip("`"))
                end = text.find("`" * run, i + run)
                if end >= 0:
                    flush()
                    code = text[i + run:end].replace("\\|", "|")
                    pieces.append('<font name="Mono" size="9">' +
                        self.plain(code, "Mono", normalize=False) + '</font>')
                    i = end + run; continue
            br = re.match(r"<br\s*/?>", text[i:], flags=re.I)
            if br:
                flush(); pieces.append("<br/>"); i += len(br.group()); continue
            if text.startswith("**", i):
                end = text.find("**", i + 2)
                if end >= 0:
                    flush(); pieces.append("<b>" + self.inline(text[i + 2:end]) + "</b>")
                    i = end + 2; continue
            if text[i] == "*" and (i == 0 or text[i - 1] in " \t\n([{ "):
                emphasis = re.match(r"\*([^*\s](?:[^*]*[^*\s])?)\*(?![\w*])", text[i:])
                if emphasis:
                    flush(); pieces.append("<i>" + self.inline(emphasis.group(1)) + "</i>")
                    i += len(emphasis.group()); continue
            if text[i] == "[":
                link = re.match(r"\[([^\]]+)\]\(([^\s)]+)(?:\s+\"[^\"]*\")?\)", text[i:])
                if link:
                    flush()
                    label, target = link.groups()
                    target = html.escape(html.unescape(target), quote=True)
                    pieces.append(f'<a href="{target}" color="#245f92">' +
                        self.inline(label) + '</a>')
                    i += len(link.group()); continue
            literal.append(text[i]); i += 1
        flush()
        return "".join(pieces)

    def code_plain(self, text):
        glyphs = pdfmetrics.getFont("Mono").face.charToGlyph
        result = []
        for ch in text:
            if ord(ch) in glyphs or ch in "\r\n\t":
                result.append(ch)
            else:
                self.missing[f"U+{ord(ch):04X}"] += 1
                result.append(f"U+{ord(ch):04X}")
        return "".join(result)


def split_table_row(line):
    """Pipes in code spans and backslash-escaped pipes are not delimiters."""
    text = line.strip()
    if text.startswith("|"):
        text = text[1:]
    # The last bar is a delimiter only when it is not escaped.
    if text.endswith("|") and not text.endswith("\\|"):
        text = text[:-1]
    cells, buf = [], []
    ticks = 0
    i = 0
    while i < len(text):
        ch = text[i]
        if ch == "\\" and i + 1 < len(text):
            buf.extend(text[i:i + 2]); i += 2; continue
        if ch == "`":
            n = len(text[i:]) - len(text[i:].lstrip("`"))
            if ticks == 0:
                ticks = n
            elif ticks == n:
                ticks = 0
            buf.append("`" * n); i += n; continue
        if ch == "|" and ticks == 0:
            cells.append("".join(buf).strip()); buf = []
        else:
            buf.append(ch)
        i += 1
    cells.append("".join(buf).strip())
    return cells


class CodeBlock(Flowable):
    """Splittable monospace code, with no rewriting or discarded long lines."""
    font = "Mono"
    size = 8.25
    leading = 11.3
    padding = 9

    def __init__(self, text=None, rows=None, language="", continued=False):
        super().__init__()
        self.raw = text
        self.rows = rows
        self.language = language
        self.continued = continued
        self.spaceBefore = 3
        self.spaceAfter = 9
        self._rows = []

    def wrap(self, availWidth, availHeight):
        self.width = availWidth
        if self.rows is None:
            unit = pdfmetrics.stringWidth("M", self.font, self.size)
            count = max(8, int((availWidth - self.padding * 2) / unit))
            self._rows = []
            for line in self.raw.split("\n"):
                line = line.expandtabs(4)
                self._rows.extend([line[j:j + count] for j in range(0, len(line), count)] or [""])
        else:
            self._rows = self.rows
        self.height = self.leading * len(self._rows) + 2 * self.padding
        return self.width, self.height

    def split(self, availWidth, availHeight):
        self.wrap(availWidth, availHeight)
        count = int((availHeight - 2 * self.padding) / self.leading)
        if count < 2:
            return []
        if count >= len(self._rows):
            return [self]
        return [CodeBlock(rows=self._rows[:count], language=self.language,
                          continued=self.continued),
                CodeBlock(rows=self._rows[count:], language=self.language, continued=True)]

    def draw(self):
        c = self.canv
        c.setFillColor(PALE)
        c.setStrokeColor(RULE)
        c.roundRect(0, 0, self.width, self.height, 3, stroke=1, fill=1)
        c.setFillColor(INK)
        c.setFont(self.font, self.size)
        y = self.height - self.padding - self.size
        for row in self._rows:
            c.drawString(self.padding, y, row)
            y -= self.leading


class Heading(Paragraph):
    def __init__(self, markup, style, key=None, title=None, level=0):
        super().__init__(markup, style)
        self.anchor_key = key
        self.outline_title = title
        self.outline_level = level


class PaginatedTable(Table):
    """A continued table starts on a new page, never twice on one page.

    ReportLab can otherwise fit the first complete-row fragment, repeat its
    header again in the remaining space, then split the next row. Explicit page
    boundaries preserve in-row splitting for genuinely tall rows without that
    repeated-header visual artifact.
    """
    def split(self, availWidth, availHeight):
        parts = super().split(availWidth, availHeight)
        if not parts and not self.splitInRow:
            # Ordinary rows stay intact. Only a row taller than an entire page
            # may split within a cell, otherwise short tails become orphans.
            frame_height = getattr(getattr(self, "_frame", None), "_height", PAGE_W - 100)
            header_rows = self.repeatRows if isinstance(self.repeatRows, int) else len(self.repeatRows)
            if len(self._rowHeights) > header_rows:
                first_height = sum(self._rowHeights[:header_rows + 1])
                if first_height > frame_height:
                    self.splitInRow = 1
                    try:
                        parts = super().split(availWidth, availHeight)
                    finally:
                        self.splitInRow = 0
        if len(parts) == 2:
            return [parts[0], PageBreak(), parts[1]]
        return parts


class HeadingGroup(KeepTogether):
    """Keep headings with the first table row, while allowing long tables to flow."""

    def split(self, availWidth, availHeight):
        if getattr(self, "_wrapInfo", None) != (availWidth, availHeight):
            self.wrap(availWidth, availHeight)
        table = self._content[-1]
        if isinstance(table, PaginatedTable):
            header_rows = table.repeatRows if isinstance(table.repeatRows, int) else len(table.repeatRows)
            first_rows_height = sum(table._rowHeights[:header_rows + 1])
            minimum_height = self._H - table._height + first_rows_height
            if minimum_height <= availHeight:
                return self._content[:]
        return super().split(availWidth, availHeight)


class DocTemplate(BaseDocTemplate):
    def afterFlowable(self, flowable):
        if isinstance(flowable, Heading) and flowable.anchor_key:
            self.canv.bookmarkPage(flowable.anchor_key)
            self.canv.addOutlineEntry(flowable.outline_title,
                flowable.anchor_key, flowable.outline_level, closed=True)


class Renderer:
    inline_figures = frozenset({"media-flow.png"})

    def __init__(self, source, target, commit, date, uniform_landscape=True, fonts=None):
        self.source = Path(source).resolve()
        self.target = Path(target).resolve()
        self.commit = commit
        self.date = date
        self.uniform_landscape = uniform_landscape
        self.fonts = fonts or {}
        self.fmt = Formatter()
        self.st = styles()
        self.warnings = []
        self.assets = []
        self.anchors = []
        self.stats = Counter()
        self.layout_width = PAGE_H - 2 * MARGIN if uniform_landscape else BODY_W

    def paragraph(self, text, style="body"):
        return Paragraph(self.fmt.inline(text), self.st[style])

    def table(self, lines, source_line):
        rows = [split_table_row(s) for s in lines]
        headers = rows[0]
        if len(rows) > 1 and all(re.fullmatch(r":?-{2,}:?", c.replace(" ", "")) for c in rows[1]):
            rows.pop(1)
        n = max(len(row) for row in rows)
        if n > len(headers):
            self.warnings.append({"line": source_line, "kind": "extra_table_cells", "columns": n})
        for offset, row in enumerate(rows):
            if len(row) != len(headers):
                self.warnings.append({"line": source_line + offset + (offset > 0),
                    "kind": "table_cell_count", "expected": len(headers), "actual": len(row)})
            row.extend([""] * (n - len(row)))
        # Give endpoint/name cells predictable room while keeping descriptions readable.
        if n == 2:
            ratios = [0.38, 0.62]
            if re.search(r"Статус|CHECK|Тип", headers[0]):
                ratios = [0.22, 0.78]
            elif "Индекс" in headers[0]:
                ratios = [0.56, 0.44]
        elif n == 3:
            ratios = [0.28, 0.37, 0.35]
        elif n == 4:
            ratios = [0.24, 0.25, 0.23, 0.28]
        elif n == 5:
            ratios = [0.22, 0.17, 0.13, 0.22, 0.26]
        elif n == 6:
            ratios = [0.19, 0.14, 0.13, 0.14, 0.20, 0.20]
        else:
            ratios = [1 / n] * n
        wide = self.layout_width > BODY_W
        font_size = (9.6 if n <= 3 else (9.3 if n == 4 else 8.9)) if wide else (9.25 if n <= 3 else (8.9 if n == 4 else 8.25))
        cell_style = ParagraphStyle("Cell", parent=self.st["body"], fontSize=font_size,
            leading=font_size * 1.30, spaceBefore=0, spaceAfter=0,
            allowWidows=1, allowOrphans=1)
        head_style = ParagraphStyle("CellHead", parent=cell_style, fontName="DocBold",
            textColor=colors.white, fontSize=font_size, leading=font_size * 1.3)
        cells = []
        for row_index, row in enumerate(rows):
            rendered = []
            for cell in row:
                markup = self.fmt.inline(cell)
                # Keep monospace text proportional to the table, not body text.
                markup = markup.replace('size="9"', f'size="{font_size * .95:.2f}"')
                if row_index == 0:
                    markup = re.sub(r' color="#[0-9a-fA-F]+"', ' color="#ffffff"', markup)
                rendered.append(Paragraph(markup or "&#160;", head_style if row_index == 0 else cell_style))
            cells.append(rendered)
        table = PaginatedTable(cells, colWidths=[self.layout_width * x for x in ratios], repeatRows=1,
            hAlign="LEFT", splitByRow=1, splitInRow=0, spaceBefore=4, spaceAfter=11)
        table.setStyle(TableStyle([
            ("BACKGROUND", (0, 0), (-1, 0), BLUE),
            ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, PALE]),
            ("VALIGN", (0, 0), (-1, -1), "TOP"),
            ("LEFTPADDING", (0, 0), (-1, -1), 7),
            ("RIGHTPADDING", (0, 0), (-1, -1), 7),
            ("TOPPADDING", (0, 0), (-1, -1), 5),
            ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
            ("LINEBELOW", (0, 0), (-1, -1), 0.35, RULE),
        ]))
        self.stats["tables"] += 1
        self.stats["table_rows"] += len(rows)
        return table

    def figure(self, label, reference, display_scale=1.0):
        path = (self.source.parent / reference).resolve()
        if not path.is_file():
            raise FileNotFoundError(f"Missing diagram: {path}")
        with PILImage.open(path) as image:
            width, height = image.size
        max_height = 420 if self.layout_width > BODY_W else 588
        figure_width = self.layout_width
        if self.uniform_landscape and Path(reference).name in self.inline_figures:
            # This previously inline portrait diagram already fits the shorter
            # landscape page: retain its original physical display dimensions.
            figure_width = BODY_W
        scale = min(figure_width / width, max_height / height) * display_scale
        # ReportLab embeds the original full-resolution raster without resampling.
        figure = Image(str(path), width=width * scale, height=height * scale,
            kind="direct", mask="auto", lazy=2)
        figure.hAlign = "CENTER"
        self.assets.append({"path": str(path), "reference": reference,
            "label": label, "sha256": sha256(path), "width": width,
            "height": height, "display_width": width * scale, "display_height": height * scale})
        self.stats["figures"] += 1
        return KeepTogether([Spacer(1, 5), figure, self.paragraph(label, "caption")])

    def keep_table_tail(self, story):
        """Balance short reference-table endings before a dedicated diagram page."""
        start = len(story)
        height = 0
        has_table = False
        while start > 0:
            item = story[start - 1]
            if not isinstance(item, (Paragraph, PaginatedTable)) or isinstance(item, Heading):
                break
            _, item_height = item.wrap(self.layout_width, PAGE_W - 100)
            item_height += item.getSpaceBefore() + item.getSpaceAfter()
            if height + item_height > 280:
                break
            height += item_height
            has_table |= isinstance(item, PaginatedTable)
            start -= 1
            preceding_label = start > 0 and bool(getattr(story[start - 1], "keepWithNext", False))
            if height >= 160 and not preceding_label:
                break
        if has_table and height >= 100:
            story[start:] = [KeepTogether(story[start:])]

    def parse(self, text):
        lines = text.splitlines()
        # The opening metadata becomes the designed cover, preserving the source body.
        start = next((i for i, line in enumerate(lines) if line.strip() == "## Содержание"), 0)
        story = []
        pending_anchor = None
        i = start
        in_toc = False
        chapter_started = False
        outline_root_exists = False
        active_layout = "wide" if self.uniform_landscape else "body"
        chapter_layout = active_layout
        last_block_figure = False
        while i < len(lines):
            raw = lines[i]
            line = raw.strip()
            if not line:
                i += 1; continue
            anchor = re.fullmatch(r'<a\s+(?:id|name)="([^"]+)"\s*></a>', line)
            if anchor:
                pending_anchor = anchor.group(1)
                self.anchors.append(pending_anchor)
                in_toc = False
                i += 1; continue
            is_image = re.fullmatch(r"!\[([^\]]*)\]\(([^)]+)\)", line)
            is_chapter = re.match(r"^#\s+", line)
            if last_block_figure and not is_image and not is_chapter:
                if active_layout != chapter_layout:
                    story.append(NextPageTemplate(chapter_layout))
                story.append(PageBreak())
                active_layout = chapter_layout
                self.layout_width = PAGE_H - 2 * MARGIN if active_layout == "wide" else BODY_W
                last_block_figure = False
            if line.startswith("```"):
                language = line[3:].strip()
                code = []
                i += 1
                while i < len(lines) and not lines[i].strip().startswith("```"):
                    code.append(lines[i]); i += 1
                story.append(CodeBlock(self.fmt.code_plain("\n".join(code)), language=language))
                self.stats["code_blocks"] += 1
                i += 1; continue
            heading = re.match(r"^(#{1,6})\s+(.+)$", line)
            if heading:
                level, title = len(heading.group(1)), heading.group(2)
                if title == "Содержание":
                    in_toc = True
                if level == 1:
                    number = re.match(r"(\d+)\.", title)
                    # Match the original's chapter-oriented page layouts.
                    chapter_layout = "wide" if self.uniform_landscape or (number and int(number.group(1)) in {2, 4, 5, 6}) else "body"
                    story.append(NextPageTemplate(chapter_layout))
                    story.append(PageBreak())
                    active_layout = chapter_layout
                    self.layout_width = PAGE_H - 2 * MARGIN if active_layout == "wide" else BODY_W
                    last_block_figure = False
                    chapter_started = True
                elif title == "Об этом документе":
                    story.append(PageBreak())
                outline_level = 0 if level == 1 or not chapter_started else 1
                if pending_anchor:
                    outline_root_exists = True
                story.append(Heading(self.fmt.inline(title), self.st[f"h{min(level, 4)}"],
                    pending_anchor, title, outline_level))
                pending_anchor = None
                self.stats["headings"] += 1
                i += 1; continue
            image = is_image
            if image:
                label, reference = image.groups()
                # Keep the original inline figure policy unless a guide opts
                # for dedicated landscape pages for every diagram.
                if Path(reference).name not in self.inline_figures:
                    self.keep_table_tail(story)
                    if active_layout != "wide":
                        story.append(NextPageTemplate("wide"))
                    story.append(PageBreak())
                    active_layout = "wide"
                    self.layout_width = PAGE_H - 2 * MARGIN
                    last_block_figure = True
                story.append(self.figure(label, reference)); i += 1; continue
            if line.startswith("|"):
                begin = i
                table_lines = []
                while i < len(lines) and lines[i].strip().startswith("|"):
                    table_lines.append(lines[i]); i += 1
                story.append(self.table(table_lines, begin + 1)); continue
            bullet = re.match(r"^(\s*)([-*+]\s+|\d+[.)]\s+)(.+)$", raw)
            if bullet:
                indent, marker, text_value = bullet.groups()
                if in_toc:
                    style = ParagraphStyle("TOCItem", parent=self.st["toc"],
                        leftIndent=12 if len(indent) >= 2 else 0,
                        fontName="Doc" if len(indent) >= 2 else "DocBold")
                    story.append(Paragraph(self.fmt.inline(text_value), style))
                else:
                    prefix = "-" if marker.strip() in "-*+" else marker.strip()
                    style = ParagraphStyle("ListItem", parent=self.st["list"],
                        leftIndent=13 + len(indent) * 3)
                    story.append(Paragraph(self.fmt.inline(prefix + " " + text_value), style))
                self.stats["list_items"] += 1
                i += 1; continue
            if re.fullmatch(r"[-*_]{3,}", line):
                story.append(Spacer(1, 9)); i += 1; continue
            if line.startswith(">"):
                story.append(self.paragraph(line.lstrip("> "), "quote")); i += 1; continue
            paragraph_lines = [raw]
            i += 1
            while i < len(lines) and lines[i].strip():
                next_line = lines[i].strip()
                if re.match(r"^(#|```|\||!\[|<a |>|[-*+]\s|\d+[.)]\s)", next_line):
                    break
                paragraph_lines.append(lines[i]); i += 1
            joined = ""
            for pos, part in enumerate(paragraph_lines):
                joined += part.strip()
                if pos < len(paragraph_lines) - 1:
                    joined += "<br/>" if part.endswith("  ") else " "
            paragraph = self.paragraph(joined)
            if re.fullmatch(r"\*\*[^*]+\*\*", joined):
                paragraph.keepWithNext = 1
            story.append(paragraph)
            self.stats["paragraphs"] += 1
        return story

    def running_page(self, canvas, doc):
        canvas.saveState()
        page_w, page_h = canvas._pagesize
        canvas.setFillColor(MUTED)
        canvas.setFont("Doc", 8.1)
        canvas.drawString(MARGIN, page_h - 26, "MEETSPACE  /  ТЕХНИЧЕСКАЯ ДОКУМЕНТАЦИЯ")
        canvas.drawRightString(page_w - MARGIN, page_h - 26, "Исходный срез · " + self.commit[:7])
        canvas.setStrokeColor(RULE)
        canvas.setLineWidth(0.5)
        canvas.line(MARGIN, page_h - 36, page_w - MARGIN, page_h - 36)
        canvas.line(MARGIN, 36, page_w - MARGIN, 36)
        canvas.setFont("Doc", 8.1)
        canvas.drawString(MARGIN, 24, "Редакция текста · " + self.date.replace(" г.", ""))
        canvas.drawRightString(page_w - MARGIN, 24, str(doc.page))
        canvas.restoreState()

    def cover(self, canvas, doc):
        canvas.saveState()
        page_w, page_h = canvas._pagesize
        canvas.setFillColor(PALE)
        canvas.rect(0, 0, page_w, page_h, fill=1, stroke=0)
        canvas.setFillColor(BLUE)
        canvas.rect(0, page_h - 16, page_w, 16, fill=1, stroke=0)
        canvas.setFillColor(MUTED)
        canvas.setFont("DocBold", 10.5)
        canvas.drawString(MARGIN, page_h - 83, "РУКОВОДСТВО")
        canvas.setFillColor(BLUE)
        canvas.setFont("DocBold", 47)
        canvas.drawString(MARGIN, page_h - 160, "MeetSpace")
        canvas.setFont("DocBold", 31)
        canvas.drawString(MARGIN, page_h - 219, "Техническая")
        canvas.drawString(MARGIN, page_h - 258, "документация")
        metadata_x = 460 if self.uniform_landscape else MARGIN
        y = page_h - (176 if self.uniform_landscape else 417)
        repository_y = y
        if self.uniform_landscape:
            canvas.setStrokeColor(RULE)
            canvas.setLineWidth(0.8)
            canvas.line(426, page_h - 350, 426, page_h - 143)
        for label, value in [
            ("РЕПОЗИТОРИЙ", "github.com/janickiy/meet-space"),
            ("ВЕТКА / COMMIT", "main / " + self.commit),
            ("ДАТА ОБНОВЛЕНИЯ ТЕКСТА", self.date),
        ]:
            canvas.setFillColor(MUTED)
            canvas.setFont("DocBold", 8.4)
            canvas.drawString(metadata_x, y, label)
            canvas.setFillColor(INK)
            canvas.setFont("Doc", 10.8)
            canvas.drawString(metadata_x, y - 21, value)
            y -= 73
        canvas.linkURL("https://github.com/janickiy/meet-space",
            (metadata_x, repository_y - 24, metadata_x + 230, repository_y - 9), relative=0)
        canvas.setFillColor(MUTED)
        canvas.setFont("Doc", 8.7)
        canvas.drawString(MARGIN, 62, "Зафиксированный срез реализации · справочник разработчика и администратора")
        canvas.restoreState()

    def render(self):
        text = self.source.read_text(encoding="utf-8")
        self.target.parent.mkdir(parents=True, exist_ok=True)
        body = self.parse(text)
        internal_refs = set(re.findall(r"\]\(#([^\)]+)\)", text))
        unresolved = sorted(internal_refs - set(self.anchors))
        if unresolved:
            raise ValueError(f"Unresolved internal links: {unresolved}")
        default_size = landscape(A4) if self.uniform_landscape else A4
        default_w, default_h = default_size
        frame = Frame(MARGIN, 48, default_w - 2 * MARGIN, default_h - 100, leftPadding=0,
            rightPadding=0, topPadding=0, bottomPadding=0, id="body")
        cover_frame = Frame(MARGIN, 48, default_w - 2 * MARGIN, default_h - 100, id="cover")
        wide_frame = Frame(MARGIN, 48, PAGE_H - 2 * MARGIN, PAGE_W - 100,
            leftPadding=0, rightPadding=0, topPadding=0, bottomPadding=0, id="wide")
        doc = DocTemplate(str(self.target), pagesize=default_size, leftMargin=MARGIN,
            rightMargin=MARGIN, topMargin=52, bottomMargin=48,
            title="MeetSpace - техническая документация",
            author="MeetSpace", subject="Редакция документации; исходный срез " + self.commit,
            pageCompression=1, allowSplitting=1, keepTogetherClass=HeadingGroup)
        doc.addPageTemplates([
            PageTemplate(id="cover", frames=cover_frame, onPage=self.cover, pagesize=default_size),
            PageTemplate(id="body", frames=frame, onPage=self.running_page, pagesize=default_size),
            PageTemplate(id="wide", frames=wide_frame, onPage=self.running_page, pagesize=landscape(A4)),
        ])
        first_template = "wide" if self.uniform_landscape else "body"
        doc.build([NextPageTemplate(first_template), Spacer(1, 1), PageBreak()] + body)
        manifest = {
            "source": str(self.source), "source_sha256": sha256(self.source),
            "pdf": str(self.target), "pdf_sha256": sha256(self.target),
            "commit": self.commit, "date": self.date,
            "fonts": self.fonts,
            "uniform_landscape": self.uniform_landscape,
            "stats": dict(self.stats), "assets": self.assets,
            "anchors": self.anchors, "unresolved_internal_links": unresolved,
            "warnings": self.warnings,
            "missing_glyph_substitutions": dict(self.fmt.missing),
            "fallback_glyphs": dict(self.fmt.fallback),
        }
        manifest_path = self.target.with_suffix(".render.json")
        manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({"pdf": str(self.target), "manifest": str(manifest_path),
            "stats": dict(self.stats), "warning_count": len(self.warnings),
            "missing_glyph_substitutions": dict(self.fmt.missing)}, ensure_ascii=False, indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path,
        default=DOCS / "MEETSPACE_TECHNICAL_DOCUMENTATION_RU.md")
    parser.add_argument("--output", type=Path,
        default=DOCS / "MEETSPACE_TECHNICAL_DOCUMENTATION_RU.pdf")
    parser.add_argument("--commit", default=SOURCE_COMMIT)
    parser.add_argument("--date", default="28 октября 2026 г.")
    parser.add_argument("--metadata-date", default="2026-10-28",
        help="Fixed ISO date for reproducible PDF metadata, YYYY-MM-DD")
    parser.add_argument("--uniform-landscape", action="store_true", default=True,
        help="Use landscape A4 for every page (the default)")
    parser.add_argument("--font-dir", type=Path,
        help="Directory with a complete Arial/Courier New or DejaVu family")
    args = parser.parse_args()
    try:
        stamp = datetime.strptime(args.metadata_date, "%Y-%m-%d").replace(tzinfo=timezone.utc)
    except ValueError:
        parser.error("--metadata-date must use YYYY-MM-DD")
    os.environ["SOURCE_DATE_EPOCH"] = str(int(stamp.timestamp()))
    fonts = register_fonts(args.font_dir)
    # Regression cases: embedded and escaped pipes must remain inside a cell.
    assert split_table_row(r"| a | `a|b` | c |") == ["a", "`a|b`", "c"]
    assert split_table_row(r"| a | `a\|b` | c |") == ["a", "`a\\|b`", "c"]
    assert split_table_row(r"| a | one\|two | c |") == ["a", "one\\|two", "c"]
    Renderer(args.source, args.output, args.commit, args.date,
             args.uniform_landscape, fonts).render()


if __name__ == "__main__":
    main()
