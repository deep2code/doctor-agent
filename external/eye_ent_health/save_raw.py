#!/usr/bin/env python3
"""Helpers to turn a saved nhc/gov html page into a raw/*.txt deliverable."""
import importlib.util, os, re, sys
from pathlib import Path

spec = importlib.util.spec_from_file_location("f", Path(__file__).parent / "fetch_nhc.py")
F = importlib.util.module_from_spec(spec); spec.loader.exec_module(F)

RAW = Path(__file__).parent / "raw"


JUNK = re.compile(
    r'^(上一条|下一条|【大中小】|阅读量|发布时间|来源[:：]|您现在所在位置|首页$|政策文件$|'
    r'工作动态$|关于我们$|返回主站|有效性[:：]|分类[:：]|索\s*引|发文机构|发布日期|'
    r'发文字号|中文$|英文$|-->&?$|相关链接|\.\w+司$)')


def drop_junk(text):
    return "\n".join(l for l in text.splitlines() if not JUNK.match(l))


def trim(text):
    """Drop nav/metadata preamble: keep from first substantive line."""
    lines = text.splitlines()
    pat = re.compile(r'^(国卫|国疾控|各省、|["“《]|第[一二三四五六七八九十]+、|[一二三四五六七八九十]+、)')
    for i, l in enumerate(lines):
        if pat.match(l) and len(l) > 6:
            return "\n".join(lines[i:])
    return text


def save(html_path, out_name, source_line, do_trim=True):
    html = Path(html_path).read_text(encoding="utf-8", errors="replace")
    t = drop_junk(trim(F.content_body(html)) if do_trim else F.content_body(html))
    RAW.mkdir(parents=True, exist_ok=True)
    (RAW / out_name).write_text(source_line + "\n" + t + "\n", encoding="utf-8")
    print(f"{out_name}: {len(t)} chars")
    return t


TAIL_CUT = ("分享到", "读者俱乐部会员权益", "上一条：", "下一条：", "相关附件",
             "相关稿件", "政府网站", "站点地图", "【大中小】")


def clean(text):
    """Drop the page-title echo line and everything after the article body."""
    lines = text.splitlines()
    if lines and " - 中华" in lines[0]:
        lines = lines[1:]
    for i, l in enumerate(lines):
        if any(c in l for c in TAIL_CUT):
            lines = lines[:i]
            break
    return drop_junk("\n".join(lines)).strip()


def save_text(out_name, source_line, text):
    RAW.mkdir(parents=True, exist_ok=True)
    (RAW / out_name).write_text(source_line + "\n\n" + clean(text) + "\n", encoding="utf-8")
    print(f"{out_name}: {len(clean(text))} chars")


if __name__ == "__main__":
    print(F.content_body(Path(sys.argv[1]).read_text(encoding='utf-8', errors='replace'))[:2000])
