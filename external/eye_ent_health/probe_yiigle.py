#!/usr/bin/env python3
import re, subprocess, sys, urllib.request, urllib.parse
UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
HOSTS = ["https://cmab.yiigle.com", "https://seleguide.yiigle.com", "https://training.yiigle.com"]

def get(url):
    req = urllib.request.Request(url, headers={"User-Agent": UA, "Accept-Language": "zh-CN,zh;q=0.9"})
    try:
        r = urllib.request.urlopen(req, timeout=40)
        return r.status, r.read()
    except Exception as e:
        code = getattr(e, "code", None)
        return code, str(e).encode()

titles = sys.argv[1:]
for t in titles:
    for h in HOSTS:
        for name in {t, t.replace("（", "(").replace("）", ")"), t.replace("，", ","), t.replace("（", " (").replace("）", ")")}:
            u = h + "/uploads/guide_html/" + urllib.parse.quote(name) + ".html"
            s, b = get(u)
            err = "系统发生错误".encode("utf-8") not in b and b"Thlike" in b
            print(f"{s} {len(b):7d} err={int(bool(err))} {u}")
            if s == 200 and not err and len(b) > 20000:
                fn = "/tmp/yy_" + re.sub(r"\W+", "_", name)[:40] + ".html"
                open(fn, "wb").write(b)
                print("   saved", fn)
                break
