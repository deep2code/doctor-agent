import os
import re

from pypdf import PdfReader

RAW = os.path.join(os.path.dirname(os.path.abspath(__file__)), "raw")
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "sections")
os.makedirs(OUT, exist_ok=True)

START = re.compile(r"食养原则和建议")
END = re.compile(r"附录\s*1")

for name in sorted(os.listdir(RAW)):
    if not name.endswith(".pdf"):
        continue
    pages = PdfReader(os.path.join(RAW, name)).pages
    hits = []
    for i, p in enumerate(pages):
        if START.search(p.extract_text() or ""):
            hits.append(i)
    if not hits:
        print(name, "NO START")
        continue
    # TOC pages list the heading too; the real section is the last page that
    # starts with it (skip TOC by requiring the page to be after page 1)
    start = [i for i in hits if i >= 2]
    start = start[0] if start else hits[-1]
    buf = []
    for j in range(start, len(pages)):
        t = pages[j].extract_text() or ""
        if j > start and END.search(t):
            break
        buf.append(t)
    text = "\n".join(buf)
    end = END.search(text)
    if end and end.start() > 500:
        text = text[: end.start()]
    with open(os.path.join(OUT, name.replace(".pdf", ".txt")), "w") as f:
        f.write(text)
    print(name, "pages", len(pages), "start", start, "section_chars", len(text))
