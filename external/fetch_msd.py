#!/usr/bin/env python3
"""Download MSD Manual (默沙东诊疗手册) Chinese consumer pages.

Reads the zh sitemap, keeps /home/<section>/<subsection>/<topic> topic pages
(4-segment paths, excluding multimedia/resources), fetches each page, extracts
the <h1> title, the <main> body text and the "上次更新" date, and writes one
JSON file per page to external/msd_manual/. Idempotent (skips existing files).

Output schema (per page):
  {"url", "path", "title", "content", "updated", "authors"}
"""
import json, re, sys, time, urllib.request, urllib.parse, html as html_mod
from pathlib import Path

# BASE: "home" (consumer) or "professional". Output dir is msd_manual{_prof}.
BASE = sys.argv[1] if len(sys.argv) > 1 else "home"
OUT = Path(__file__).parent / ("msd_manual_prof" if BASE == "professional" else "msd_manual")
SITEMAP = "https://www.msdmanuals.cn/zh/sitemap.xml"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"}
DELAY = 1.2  # seconds between requests (robots.txt asks 5s; we compromise for research volume)

TOPIC_RE = re.compile(rf"^https://www\.msdmanuals\.cn/{BASE}/[^/]+/[^/]+/[^/]+/?$")


def is_topic(u):
    return bool(TOPIC_RE.match(u)) and all(x not in u for x in
        ("resourcespages", "multimedia", "pages-with-widgets", "drug-names",
         "pronunciations", "authors", "quiz", "video", "news"))


def fetch(url):
    # Percent-encode non-ASCII chars (é/ö/ç...) in the path so urlopen works.
    req_url = urllib.parse.quote(url, safe=":/?=&%")
    req = urllib.request.Request(req_url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return r.read().decode("utf-8", "replace")
        except Exception:
            if i == 2:
                raise
            time.sleep(3)


def extract(html, url):
    # Title
    tm = re.search(r"<h1[^>]*>(.*?)</h1>", html, re.S)
    title = re.sub(r"<[^>]+>", "", tm.group(1)).strip() if tm else ""
    # Main body
    mm = re.search(r"<main[^>]*>(.*?)</main>", html, re.S)
    main_html = mm.group(1) if mm else ""
    main_html = re.sub(r"<script[^>]*>.*?</script>", " ", main_html, flags=re.S)
    main_html = re.sub(r"<style[^>]*>.*?</style>", " ", main_html, flags=re.S)
    text = re.sub(r"<[^>]+>", "\n", main_html)
    text = html_mod.unescape(text)
    lines = [l.strip() for l in text.split("\n") if l.strip()]
    content = "\n".join(lines)
    # Updated date: "上次更新: <month> <year>"
    um = re.search(r"上次更新[:：]\s*([A-Za-z]+ \d{4})", content)
    updated = um.group(1) if um else ""
    return {"url": url, "path": url.replace("https://www.msdmanuals.cn/", ""),
            "title": title, "content": content, "updated": updated,
            "source": "professional" if BASE == "professional" else "consumer"}


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    print("下载 sitemap...", file=sys.stderr)
    data = fetch(SITEMAP)
    urls = re.findall(r"<loc>(.*?)</loc>", data)
    topics = [u for u in urls if is_topic(u)]
    print(f"{BASE} 正文页: {len(topics)}", file=sys.stderr)

    ok, fail = 0, []
    for i, u in enumerate(sorted(topics), 1):
        name = u.replace("https://www.msdmanuals.cn/", "").replace("/", "__") + ".json"
        out = OUT / name
        if out.exists():
            ok += 1
            continue
        try:
            html = fetch(u)
            rec = extract(html, u)
            if len(rec["content"]) < 200:
                fail.append((u, "内容过短"))
                print(f"[{i}/{len(topics)}] SKIP {rec['title'][:40]} (内容过短)", file=sys.stderr)
                continue
            out.write_text(json.dumps(rec, ensure_ascii=False), encoding="utf-8")
            ok += 1
            if i % 100 == 0 or i <= 3:
                print(f"[{i}/{len(topics)}] {rec['title'][:40]} ({len(rec['content'])} 字符)", file=sys.stderr)
        except Exception as e:
            fail.append((u, str(e)[:80]))
            print(f"[{i}/{len(topics)}] FAIL {u[:60]}: {str(e)[:80]}", file=sys.stderr)
        time.sleep(DELAY)

    print(f"DONE: {ok}/{len(topics)} 成功, 失败 {len(fail)}", file=sys.stderr)
    for f in fail[:10]:
        print("  失败:", f, file=sys.stderr)


if __name__ == "__main__":
    main()
