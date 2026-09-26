#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Fetch original Chinese popular-science texts for the KB supplement batch:

1. 中国睡眠研究会 (Chinese Sleep Research Society) — www.zgsmyjh.org
   NOTE: the brief listed www.cssr.org.cn, but that domain actually serves the
   Chinese Society of Space Research (空间科学). The real sleep society site is
   zgsmyjh.org (同域名主体“中国睡眠研究会”). Its 大众科普 column (col.jsp?id=107)
   holds the society's own science articles.
2. 上海市儿童医院 — www.shchildren.com.cn 科普教育 channel (channels/640.html).
   All mp.weixin.qq.com external links are skipped (unreachable from this host).
3. 中国红十字会 — www.redcross.org.cn 生命健康 column
   (autoweb/hlwmh/second/hlwmh_hlwmhsmjk.html, articles under autoweb/hlwmh/third/).

Output: external/sleep_child_redcross/raw/*.txt + MANIFEST.txt
Text is copied verbatim (no rewriting). Articles whose body is image-only
(posters) or under 400 Chinese characters are discarded and recorded in MANIFEST.
"""
import re
import sys
import time
import json
import warnings
from pathlib import Path

import requests
from bs4 import BeautifulSoup

warnings.filterwarnings("ignore")
try:
    import urllib3

    urllib3.disable_warnings()
except Exception:
    pass

BASE = Path(__file__).resolve().parent / "sleep_child_redcross" / "raw"
UA = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)
DELAY = 0.9
MIN_ZH = 400


def zh_count(text: str) -> int:
    return len(re.findall(r"[\u4e00-\u9fff]", text))


def get(session, url, retries=3, **kw):
    for attempt in range(retries):
        try:
            r = session.get(url, timeout=25, **kw)
            if r.status_code == 200:
                return r
            if r.status_code == 204 and attempt < retries - 1:
                time.sleep(2.0)
                continue
            return r
        except requests.RequestException:
            if attempt < retries - 1:
                time.sleep(2.0)
            else:
                raise
    return None


def clean_lines(text: str) -> str:
    lines = [ln.strip() for ln in text.splitlines()]
    out = []
    for ln in lines:
        if out and not out[-1] and not ln:
            continue
        out.append(ln)
    return "\n".join(out).strip()


def block_text(node) -> str:
    """Extract text keeping each paragraph on one line.

    CMS bodies wrap every digit run in its own <span>; a global
    get_text("\\n") would split numbers like 2023 into "202\\n3". Here <br>
    becomes a newline, every leaf block gets a trailing newline, and the
    whole subtree is then read with an empty separator so inline spans
    concatenate verbatim.
    """
    for br in node.find_all("br"):
        br.replace_with("\n")
    blocks = node.find_all(
        ["p", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "blockquote", "pre", "div"]
    )
    for b in blocks:
        if not b.find(["p", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "blockquote", "pre", "div"]):
            b.append("\n")
    text = node.get_text("")
    lines = []
    for ln in text.splitlines():
        ln = re.sub(r"[ \t\u3000]+", " ", ln).strip()
        if ln:
            lines.append(ln)
    return "\n".join(lines)


def save(fname: str, url: str, title: str, meta: str, body: str) -> bool:
    if zh_count(body) < MIN_ZH:
        return False
    content = f"# URL: {url}\n# 标题: {title}\n# 发布方/日期: {meta}\n\n{clean_lines(body)}\n"
    path = BASE / fname
    path.write_text(content, encoding="utf-8")
    return True


# ---------------------------------------------------------------- sleep society
def fetch_sleep(manifest, excluded, unreachable):
    s = requests.Session()
    s.headers.update({"User-Agent": UA})
    s.verify = False
    s.get("https://www.zgsmyjh.org/", timeout=25)
    time.sleep(DELAY)
    referer = "https://www.zgsmyjh.org/col.jsp?id=107"
    articles = [
        ("sleep-cssr-improve", "892", "如何改善我们的睡眠：从认识到应对"),
        ("sleep-cssr-mindfulness", "705", "告别失眠焦躁，正念之旅让我找回曾经的自己"),
        ("sleep-cssr-deprivation", "156", "睡眠不足的危害"),
        ("sleep-cssr-narcolepsy", "992", "从“看见”到“改变”：为发作性睡病患者寻找答案"),
        ("sleep-cssr-kids-whitepaper", "638", "《中国青少年儿童睡眠指数白皮书》发布（2019世界睡眠日启动会报道）"),
        ("sleep-cssr-neurology", "627", "2018神经病学与睡眠医学相遇（睡眠医学学科进展科普）"),
        ("sleep-cssr-day2019", "637", "2019年世界睡眠日大型睡眠科普启动会在北京召开"),
    ]
    for slug, aid, _ in articles:
        url = f"https://www.zgsmyjh.org/nd.jsp?id={aid}"
        r = get(s, url, headers={"Referer": referer})
        time.sleep(DELAY)
        if r is None or r.status_code != 200:
            unreachable.append(f"zgsmyjh.org 文章 id={aid} (status {r.status_code if r else 'none'})")
            continue
        r.encoding = "utf-8"
        soup = BeautifulSoup(r.text, "html.parser")
        rc = soup.select_one(".newsDetail .richContent") or soup.select_one(".richContent")
        if rc is None:
            unreachable.append(f"zgsmyjh.org 文章 id={aid} 无正文容器")
            continue
        title = soup.title.get_text(strip=True).split(" - ")[0] if soup.title else aid
        date = ""
        m = re.search(r"发表时间[：:]\s*([\d\-/: ]+)", r.text)
        if m:
            date = m.group(1).strip()[:10]
        imgs = len(rc.find_all("img"))
        body = block_text(rc)
        if not save(f"{slug}.txt", url, title,
                    f"中国睡眠研究会（官网大众科普/新闻资讯） {date}", body):
            excluded.append(f"{slug}.txt | {title} | {url} | 正文中文字 {zh_count(body)}<{MIN_ZH} 或图片版(img={imgs})")
            continue
        manifest.append((f"{slug}.txt", title, url, "中国睡眠研究会", date))
        print("OK sleep", slug, zh_count(body))


# ---------------------------------------------------------------- children hospital
SHC_IDS = {
    "child-fever-why": ("9272", "宝宝为什么会发热？"),
    "child-fever-expert": ("9282", "从容应对各种感染性发热，儿科专家来支招！"),
    "child-mycoplasma": ("9284", "儿童支原体肺炎进入高发季，患儿年龄越小，混合感染的风险越大"),
    "child-mycoplasma-rehab": ("9248", "支原体肺炎来袭，儿童如何进行肺康复？"),
    "child-tic": ("9285", "什么是抽动症？"),
    "child-tic-care": ("9480", "抽动障碍家庭必读：6大护理、饮食注意事项"),
    "child-allergic-rhinitis": ("9276", "鼻塞、流涕、打喷嚏……别把过敏性鼻炎当“感冒”！"),
    "child-allergic-rhinitis2": ("8030", "儿童过敏性鼻炎，你必须知道的真相"),
    "child-allergy-test": ("8020", "过敏检测知多少"),
    "child-myopia-signs": ("9217", "注意，这些是近视的“信号”"),
    "child-screen-time": ("10348", "寒假让孩子成天对着电子屏幕？可不要后悔"),
    "child-injury-winter": ("9796", "寒假儿童意外伤害的预防和应对措施"),
    "child-trauma-firstaid": ("8016", "儿童创伤的现场初步处理原则与方法"),
    "child-neurosurgery-safety": ("9794", "安全过寒假，远离意外伤害——上海市儿童医院神经外科为您支招"),
    "child-winter-traffic": ("9793", "上海即将再度迎来降温，如何守护孩子出行安全"),
    "child-heat-protection": ("8037", "高温天，如何帮助孩子防暑降温、避免疾病？"),
    "child-summer-care": ("8042", "宝贝盛夏养生宝典"),
    "child-gastroenteritis": ("9733", "孩子呕吐腹泻，可能是急性胃肠炎"),
    "child-chickenpox": ("9734", "水痘不“水”"),
    "child-jaundice-breastfeed": ("8041", "宝宝黄疸，一定要停母乳吗？"),
    "child-influenza-flu": ("9790", "甲乙流轮番上阵，家长们应该注意什么？"),
    "child-flu-vaccine": ("9736", "一个月前得了甲流，现在又感染了乙流？流感疫苗还来得及打吗？"),
    "child-autism": ("8024", "关注来自星星的孩子，让自闭症不再“孤独”"),
    "child-infection-protect": ("9281", "秋冬季孩子如何做好传染病个人防护？"),
    "child-snacking": ("9798", "你知道春节里孩子零食“红绿灯”吗？"),
    "child-teen-support": ("9795", "如何陪伴孩子度过青春期？"),
    "child-broken-arm": ("8032", "孩子手指突然伸不直了怎么办？"),
    "child-uti": ("8026", "儿童尿路感染，该怎么预防？"),
    "child-sunscreen": ("8018", "科学防晒，让宝宝无惧阳光"),
    "child-seatbelt": ("8015", "儿童使用安全带安全吗？——当心“座椅安全带综合征”！"),
}


def fetch_children(manifest, excluded, unreachable):
    s = requests.Session()
    s.headers.update({"User-Agent": UA})
    s.verify = False
    for slug, (cid, title) in SHC_IDS.items():
        url = f"https://www.shchildren.com.cn/contents/640/{cid}.html"
        r = get(s, url)
        time.sleep(DELAY)
        if r is None or r.status_code != 200:
            unreachable.append(f"shchildren.com.cn {cid} (status {r.status_code if r else 'none'})")
            continue
        r.encoding = r.apparent_encoding
        soup = BeautifulSoup(r.text, "html.parser")
        nr = soup.select_one(".infonr .nr")
        if nr is None:
            unreachable.append(f"shchildren.com.cn {cid} 无正文容器")
            continue
        imgs = len(nr.find_all("img"))
        body = block_text(nr)
        sj = soup.select_one(".infonr .sj")
        meta_line = sj.get_text(" ", strip=True) if sj else ""
        m = re.search(r"发布日期[：:]?\s*([\d\-]+)", meta_line)
        date = m.group(1) if m else ""
        m2 = re.search(r"发布人[：:]?\s*([^\s，。]+)", meta_line)
        editor = m2.group(1) if m2 else ""
        # keep 编辑/校审 credit lines that appear in body tail
        meta = f"上海市儿童医院（科普教育栏目） {date}"
        if editor and editor != "：":
            meta += f" 发布人：{editor}"
        if not save(f"{slug}.txt", url, title, meta, body):
            excluded.append(
                f"{slug}.txt | {title} | {url} | 正文中文字 {zh_count(body)}<{MIN_ZH} 或图片版(img={imgs})"
            )
            continue
        manifest.append((f"{slug}.txt", title, url, "上海市儿童医院", date))
        print("OK child", slug, zh_count(body))


# ---------------------------------------------------------------- red cross
RC_IDS = {
    "rc-firstaid-common": ("250580", "日常生活中突发危险的急救措施与常识", False),
    "rc-earthquake": ("250803", "地震来了，怎么办？", False),
    "rc-quakeslide-fire-heat": ("250566", "地震、滑坡、野外火灾、热浪发生时的应急措施", False),
    "rc-bleeding": ("251132", "外伤出血如何急救？", False),
    "rc-burn-firstaid": ("251130", "怎样进行烧伤急救？", False),
    "rc-airway-fb": ("251133", "气管异物怎样抢救？", True),
    "rc-cpr-compression": ("251128", "怎样做胸外心脏按压术？", True),
    "rc-home-aid-taboos": ("251120", "什么叫家庭急救“八戒”？", False),
    "rc-scene-assessment": ("251119", "现场急救时首先要对病人作哪些检查？", True),
    "rc-bandaging": ("251118", "如何进行急救包扎？", True),
    "rc-joint-dislocation": ("251125", "关节脱位的急救治疗", False),
    "rc-ear-foreign-body": ("251129", "怎样取出耳内异物？", False),
    "rc-explosion-selfhelp": ("250292", "突发爆炸，该如何自救", False),
    "rc-desperate-selfhelp": ("250533", "绝境之中如何自救", False),
    "rc-landslide-escape": ("250534", "山体滑坡如何避险逃生", False),
    "rc-shipwreck-escape": ("250535", "海上沉船自救逃生攻略", False),
    "rc-chemical-burn": ("251117", "什么叫化学烧伤，如何急救？", False),
    "rc-electrocution": ("250818", "提高意识防触电", False),
    "rc-child-drowning": ("259477", "孩子溺水无声且迅速 家长不可大意", True),
    "rc-sleep-selftest": ("259543", "宅家睡不好免疫下降？10道题自测你的睡眠健康吗", False),
    "rc-sleep-check": ("260378", "对号入座 “宅”家的你睡得还好吗？", False),
}


def fetch_redcross(manifest, excluded, unreachable, overlaps):
    s = requests.Session()
    s.headers.update({"User-Agent": UA})
    s.get("https://www.redcross.org.cn/", timeout=25)  # seed anti-leech cookies
    time.sleep(DELAY)
    for slug, (aid, title, overlap) in RC_IDS.items():
        url = f"https://www.redcross.org.cn/autoweb/hlwmh/third/hlwmh_hlwmhsmjk_{aid}.html?firstcode="
        r = get(s, url)
        time.sleep(DELAY)
        if r is None or r.status_code != 200:
            unreachable.append(f"redcross.org.cn 文章 {aid} (status {r.status_code if r else 'none'})")
            continue
        r.encoding = "utf-8"
        soup = BeautifulSoup(r.text, "html.parser")
        z = soup.select_one("#zoom")
        if z is None:
            unreachable.append(f"redcross.org.cn 文章 {aid} 无正文容器")
            continue
        imgs = len(z.find_all("img"))
        body = block_text(z)
        tm = soup.select_one(".u-time")
        date = re.sub(r"时间[：:]", "", tm.get_text(strip=True)).strip() if tm else ""
        h1 = soup.select_one("h1")
        real_title = h1.get_text(strip=True) if h1 else title
        srcm = re.findall(r"来源[：:]\s*([^\n]+)", body)
        src = (" 来源：" + srcm[0].strip()) if srcm else ""
        meta = f"中国红十字会总会（生命健康科普栏目） {date}{src}"
        if not save(f"{slug}.txt", url, real_title, meta, body):
            excluded.append(
                f"{slug}.txt | {real_title} | {url} | 正文中文字 {zh_count(body)}<{MIN_ZH} 或图片版(img={imgs})"
            )
            continue
        manifest.append((f"{slug}.txt", real_title, url, "中国红十字会总会", date))
        if overlap:
            overlaps.append(f"{slug}.txt | {real_title} | 主题与既有 firstaid《红十字急救手册》21篇部分重叠（按任务要求照抄存档，撰写条目时优先取手册未覆盖要点）")
        print("OK rc", slug, zh_count(body))


def main():
    BASE.mkdir(parents=True, exist_ok=True)
    manifest, excluded, unreachable, overlaps = [], [], [], []
    fetch_sleep(manifest, excluded, unreachable)
    fetch_children(manifest, excluded, unreachable)
    fetch_redcross(manifest, excluded, unreachable, overlaps)

    lines = [f"{f} | {t} | {u} | {org} | {d}" for f, t, u, org, d in manifest]
    tail = []
    if overlaps:
        tail.append("# 与 firstaid 重叠:")
        tail += [f"#   {x}" for x in overlaps]
    if excluded:
        tail.append("# 已排除:")
        tail += [f"#   {x}" for x in excluded]
    if unreachable:
        tail.append("# 不可达:")
        tail += [f"#   {x}" for x in unreachable]
    tail.append("# 不可达: www.cssr.org.cn 实为“中国空间科学学会”（Space Science Society of China），"
                "与中国睡眠研究会无关；中国睡眠研究会真实官网为 www.zgsmyjh.org（已在该站抓取）。")
    tail.append("# 跳过: 上海市儿童医院科普教育栏目 1~10 页中约 35 条列表项直链 mp.weixin.qq.com（微信正文本机不可达，全部跳过）；"
                "多篇站内文章正文为图片海报版（如 9302 热性惊厥、9253 怎么判断发热、9478 新生儿黄疸、9449 新生儿肺炎、9246 流感问答），按规则丢弃。")
    (BASE / "MANIFEST.txt").write_text("\n".join(lines + tail) + "\n", encoding="utf-8")
    print("saved:", len(manifest), "excluded:", len(excluded), "unreachable:", len(unreachable))


if __name__ == "__main__":
    main()
