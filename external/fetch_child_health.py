#!/usr/bin/env python3
"""Fetch child-health查漏 official documents into plain-text caches.

Sources (gov.cn 政策库, direct HTTP):
  - 0~6岁儿童孤独症筛查干预服务规范(试行) (国卫办妇幼发〔2022〕12号)
    — full text in the notice HTML (筛查流程/预警征象/初筛复筛).

腰椎间盘突出: no authoritative open full-text exists outside paywalled
journal guidelines; coverage is left to the existing MSD 中文 layer.

Output: external/child/{slug}.txt  (first line = title, blank, body)
Idempotent: skips existing files.

  python3 external/fetch_child_health.py [--refresh]
"""
import argparse
import html as htmlmod
import re
import sys
import urllib.request
from pathlib import Path

OUT = Path(__file__).parent / "child"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}

DOCS = {
    "autism-screening-2022": (
        "0~6岁儿童孤独症筛查干预服务规范(试行)(国卫办妇幼发〔2022〕12号)",
        "https://www.gov.cn/zhengce/zhengceku/2022-09/23/content_5711379.htm",
        [r'(?s)<div[^>]*id="UCAP-CONTENT"[^>]*>(.*?)</div>\s*(?:<div|<!--)',
         r'(?s)class="pages_content"[^>]*>(.*?)</div>']),
}


def get(url: str, timeout: int = 30) -> str:
    req = urllib.request.Request(url, headers=UA)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return r.read().decode("utf-8", "replace")


def html_to_text(fragment: str) -> str:
    fragment = re.sub(r"<script.*?</script>", " ", fragment, flags=re.S)
    fragment = re.sub(r"<style.*?</style>", " ", fragment, flags=re.S)
    fragment = re.sub(r"</(?:p|div|tr|li|h[1-6]|table)>", "\n", fragment)
    fragment = re.sub(r"<br\s*/?>", "\n", fragment)
    fragment = re.sub(r"<[^>]+>", "", fragment)
    fragment = htmlmod.unescape(fragment)
    fragment = re.sub(r"[ \t　]+", " ", fragment)
    fragment = re.sub(r"\n\s*\n+", "\n\n", fragment)
    return fragment.strip()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--refresh", action="store_true")
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)

    ok, fail = 0, []
    for slug, (title, url, patterns) in DOCS.items():
        out = OUT / f"{slug}.txt"
        if out.exists() and not args.refresh:
            print(f"SKIP {slug}", file=sys.stderr)
            ok += 1
            continue
        try:
            html = get(url)
            body = ""
            for pat in patterns:
                m = re.search(pat, html)
                if m:
                    body = html_to_text(m.group(1))
                    if len(body) > 500:
                        break
            if len(body) < 500:
                body = html_to_text(html)
            if len(body) < 800:
                raise ValueError(f"正文过短 {len(body)} 字")
            if "预警征象" not in body:
                raise ValueError("正文缺少预警征象内容")
            out.write_text(f"{title}\n\n{body}\n", encoding="utf-8")
            ok += 1
            print(f"✅ {slug} ({len(body)} 字)", file=sys.stderr)
        except Exception as e:
            fail.append((slug, str(e)[:100]))
            print(f"❌ {slug}: {str(e)[:100]}", file=sys.stderr)
    print(f"DONE: ok={ok} fail={len(fail)}", file=sys.stderr)
    return 0 if not fail else 1


if __name__ == "__main__":
    sys.exit(main())
