#!/usr/bin/env python3
"""Render the separate MeetSpace technical guide with referenced diagram assets."""
from __future__ import annotations

import argparse
import json
import os
import re
from datetime import datetime, timezone
from pathlib import Path

from reportlab.lib import colors
from reportlab.lib.styles import ParagraphStyle
from reportlab.platypus import Flowable, Paragraph

import build_pdf as base

DOCS = Path(__file__).resolve().parents[1]
ASSETS = DOCS / "assets" / "technical-guide"
PURPLE = "#694077"
INK = "#292536"
MUTED = "#6b6677"
TEAL = "#257569"
BLUE = "#315f87"


class GuideTemplate(base.DocTemplate):
    anchor_pages = {}

    def __init__(self,*args,**kwargs):
        kwargs["title"]="MeetSpace - техническое руководство"
        kwargs["subject"]="Архитектура, API, база данных, установка и сопровождение"
        super().__init__(*args,**kwargs)

    def afterFlowable(self, flowable):
        super().afterFlowable(flowable)
        if isinstance(flowable,base.Heading) and flowable.anchor_key:
            self.anchor_pages[flowable.anchor_key]=self.page


class GuideHeadingGroup(base.KeepTogether):
    def split(self, availWidth, availHeight):
        if getattr(self,"_wrapInfo",None)!=(availWidth,availHeight):
            self.wrap(availWidth,availHeight)
        table=self._content[-1]
        if isinstance(table,base.PaginatedTable):
            header=table.repeatRows if isinstance(table.repeatRows,int) else len(table.repeatRows)
            count=min(len(table._rowHeights),header+2)
            minimum=self._H-table._height+sum(table._rowHeights[:count])
            if minimum<=availHeight:
                return self._content[:]
        return super().split(availWidth,availHeight)


def contents_entries(text):
    """Read anchored chapter and subsection titles from the canonical body."""
    body=text[text.index('<a id="guide-about">'):]
    entries=[]
    anchor=None
    for line in body.splitlines():
        match=re.fullmatch(r'<a id="([^"]+)"></a>',line.strip())
        if match:
            anchor=match[1]
            continue
        match=re.fullmatch(r'(#{1,2})\s+(.+)',line.strip())
        if match and anchor:
            entries.append((len(match[1]),match[2],anchor))
            anchor=None
    return entries


class ContentsEntry(Flowable):
    """Keep each linked title and its right-aligned page number on one row."""

    def __init__(self,title,key,level,page,formatter):
        super().__init__()
        self.key=key
        self.indent=0 if level==1 else 13
        self.spaceBefore=5 if level==1 else 0
        self.spaceAfter=3
        self.keepWithNext=level==1
        style=ParagraphStyle("GuideContentsEntry",fontName="DocBold" if level==1 else "Doc",
            fontSize=10 if level==1 else 9.5,leading=13.4,textColor=colors.HexColor("#245f92"))
        self.title=Paragraph(formatter.inline(title),style)
        number_style=ParagraphStyle("GuideContentsNumber",parent=style,alignment=2)
        self.number=Paragraph(str(page),number_style)

    def wrap(self,availWidth,availHeight):
        self.width=availWidth
        _,self.title_height=self.title.wrap(availWidth-self.indent-50,availHeight)
        _,self.number_height=self.number.wrap(32,availHeight)
        self.height=max(self.title_height,self.number_height)
        return self.width,self.height

    def draw(self):
        self.title.drawOn(self.canv,self.indent,self.height-self.title_height)
        self.number.drawOn(self.canv,self.width-32,self.height-self.number_height)
        self.canv.linkRect("",self.key,(self.indent,0,self.width,self.height),relative=1,thickness=0)


class GuideRenderer(base.Renderer):
    inline_figures = frozenset()
    toc_pages = {}
    reduced_figures = frozenset({
        "er-01-identity-overview.png",
        "er-02-conferences-overview.png",
        "er-03-recording-overview.png",
        "er-06-calls-folders-overview.png",
    })

    def figure(self, label, reference):
        """Reduce only the four selected ER diagrams, preserving source pixels."""
        scale = .85 if Path(reference).name in self.reduced_figures else 1.0
        return super().figure(label, reference, display_scale=scale)

    def parse(self, text):
        entries=contents_entries(text)
        story=super().parse(text)
        # Replace the Markdown list with fixed-width page-number rows. Placeholder
        # numbers occupy the same space in pass one, keeping pagination stable.
        toc_start=next(i for i,item in enumerate(story)
            if isinstance(item,base.Heading) and item.getPlainText()=="Содержание")
        toc_end=next(i for i in range(toc_start+1,len(story))
            if isinstance(story[i],base.Heading) and story[i].anchor_key=="guide-about")
        story[toc_start+1:toc_end-1]=[
            ContentsEntry(title,key,level,self.toc_pages.get(key,"000"),self.fmt)
            for level,title,key in entries]
        for i,item in enumerate(story[:-1]):
            if isinstance(item,base.CodeBlock) and item.language=="tree":
                item.size=9.3
                item.leading=13
            if isinstance(item,base.Paragraph) and not isinstance(item,base.Heading):
                plain=item.getPlainText()
                following=story[i+1]
                source_intro=(plain.startswith("Исходники:") and
                    isinstance(following,base.Paragraph) and
                    not isinstance(following,base.Heading))
                if source_intro or (isinstance(following,base.PaginatedTable) and len(plain)<550):
                    item.keepWithNext=1
        grouped=[]
        for item in story:
            if isinstance(item,base.CodeBlock) and item.language=="tree":
                if not grouped or not isinstance(grouped[-1],base.Heading):
                    raise ValueError("Directory tree must follow its section heading")
                heading=grouped.pop()
                grouped.append(base.KeepTogether([heading,item]))
            else:
                grouped.append(item)
        return grouped

    def running_page(self, canvas, doc):
        canvas.saveState()
        w,h=canvas._pagesize
        canvas.setFillColor(base.MUTED)
        canvas.setFont("Doc",8.1)
        toc_end=self.toc_pages.get("guide-about",3)-1
        running_title="СОДЕРЖАНИЕ" if 2<=doc.page<=toc_end else "ТЕХНИЧЕСКОЕ РУКОВОДСТВО"
        canvas.drawString(base.MARGIN,h-26,"MEETSPACE  /  "+running_title)
        canvas.drawRightString(w-base.MARGIN,h-26,"Исходники · "+self.commit[:7])
        canvas.setStrokeColor(base.RULE)
        canvas.setLineWidth(.5)
        canvas.line(base.MARGIN,h-36,w-base.MARGIN,h-36)
        canvas.line(base.MARGIN,36,w-base.MARGIN,36)
        canvas.drawString(base.MARGIN,24,self.date)
        canvas.drawRightString(w-base.MARGIN,24,str(doc.page))
        canvas.restoreState()

    def cover(self, canvas, doc):
        canvas.saveState()
        w,h=canvas._pagesize
        canvas.setFillColor(colors.HexColor("#f6f3f8"))
        canvas.rect(0,0,w,h,fill=1,stroke=0)
        canvas.setFillColor(colors.HexColor("#34203f"))
        canvas.rect(0,0,18,h,fill=1,stroke=0)
        canvas.setFillColor(base.BLUE)
        canvas.setFont("DocBold",12)
        canvas.drawString(52,h-70,"ДЛЯ РАЗРАБОТЧИКА И АДМИНИСТРАТОРА")
        canvas.setFont("DocBold",57)
        canvas.drawString(48,h-163,"MeetSpace")
        canvas.setFillColor(base.INK)
        canvas.setFont("DocBold",32)
        canvas.drawString(52,h-226,"Техническое")
        canvas.drawString(52,h-267,"руководство")
        canvas.setFont("Doc",13)
        canvas.setFillColor(base.MUTED)
        canvas.drawString(52,h-315,"Устройство системы. Контракты. Развёртывание.")
        x=520
        for i,(number,title,caption) in enumerate([
            ("01","Архитектура","Сервисы, WebRTC и запись"),
            ("02","Справочник","API и структура базы данных"),
            ("03","Эксплуатация","Установка, проверка, поддержка"),
        ]):
            y=h-146-i*95
            canvas.setFillColor(colors.white)
            canvas.roundRect(x,y-55,270,77,12,fill=1,stroke=0)
            canvas.setFillColor(base.BLUE)
            canvas.setFont("DocBold",11)
            canvas.drawString(x+16,y-2,number)
            canvas.setFont("DocBold",13)
            canvas.drawString(x+51,y-2,title)
            canvas.setFillColor(base.MUTED)
            canvas.setFont("Doc",10.2)
            canvas.drawString(x+16,y-29,caption)
        canvas.setFillColor(base.MUTED)
        canvas.setFont("Doc",10)
        canvas.drawString(52,106,"Редакция: "+self.date)
        canvas.drawString(52,85,"Репозиторий: github.com/janickiy/meet-space")
        canvas.setFont("Mono",8.7)
        canvas.drawString(52,64,"Commit: "+self.commit)
        canvas.linkURL("https://github.com/janickiy/meet-space",(52,82,320,98),relative=0)
        canvas.restoreState()


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source",type=Path,default=DOCS/"MEETSPACE_TECHNICAL_GUIDE_RU.md")
    parser.add_argument("--output",type=Path,default=DOCS/"MEETSPACE_TECHNICAL_GUIDE_RU.pdf")
    parser.add_argument("--commit",default="b9677392fbe41be448d45be5e6ea8dbe2f7820b7")
    parser.add_argument("--date",default="29 октября 2026 г.")
    parser.add_argument("--metadata-date",default="2026-10-29")
    parser.add_argument("--font-dir",type=Path)
    args=parser.parse_args()
    stamp=datetime.strptime(args.metadata_date,"%Y-%m-%d").replace(tzinfo=timezone.utc)
    os.environ["SOURCE_DATE_EPOCH"]=str(int(stamp.timestamp()))
    base.BLUE=colors.HexColor(PURPLE)
    base.INK=colors.HexColor(INK)
    base.MUTED=colors.HexColor(MUTED)
    base.PALE=colors.HexColor("#f6f3f8")
    base.RULE=colors.HexColor("#ded7e4")
    fonts=base.register_fonts(args.font_dir)
    base.DocTemplate=GuideTemplate
    base.HeadingGroup=GuideHeadingGroup
    GuideTemplate.anchor_pages={}
    GuideRenderer(args.source,args.output,args.commit,args.date,True,fonts).render()
    page_map=dict(GuideTemplate.anchor_pages)
    GuideRenderer.toc_pages=page_map
    GuideTemplate.anchor_pages={}
    GuideRenderer(args.source,args.output,args.commit,args.date,True,fonts).render()
    if page_map != GuideTemplate.anchor_pages:
        raise RuntimeError("Contents changed page layout between rendering passes")
    manifest_path=args.output.with_suffix(".render.json")
    manifest=json.loads(manifest_path.read_text())
    manifest["anchor_pages"]=page_map
    manifest["contents_entries"]=len(contents_entries(args.source.read_text(encoding="utf-8")))
    manifest_path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2),encoding="utf-8")


if __name__ == "__main__":
    main()
