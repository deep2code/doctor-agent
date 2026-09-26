#!/usr/bin/env python3
"""Batch-download full-text Chinese medical guidelines from the CMA knowledge base
(cmab.yiigle.com) into external/yiigle_guides/raw/*.txt as verbatim text material.

Main path (verified in earlier batches): direct links of the shape
    https://cmab.yiigle.com/uploads/guide_html/<URL-encoded full guideline name>.html
A page counts as a full guideline when HTTP 200 and >=1500 CJK characters.

Usage:
    python3 external/fetch_yiigle_guides.py            # try all candidates
    python3 external/fetch_yiigle_guides.py --retry    # only re-run previous misses (from MANIFEST)
No concurrency; >=0.9s between requests. Writes hits immediately.
"""
import html.parser
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = "https://cmab.yiigle.com/uploads/guide_html/"
OUT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "yiigle_guides", "raw")
UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
MIN_CJK = 1500
SLEEP = 0.9

# (slug, guideline full name as published in the CMA journal, publisher hint)
# Filenames mirror exact paper titles; parenthesis style varies per journal, so
# the fetcher also tries the full-width <-> half-width variant automatically.
CANDIDATES = [
    # --- 心血管/代谢 ---
    ("htn2024", "中国高血压防治指南（2024年修订版）", "中国高血压防治指南修订委员会"),
    ("htn2018", "中国高血压防治指南（2018年修订版）", "中国高血压防治指南修订委员会"),
    ("htn-primary2020", "国家基层高血压防治管理指南（2020版）", "国家心血管病中心、中华医学会），基层指南"),
    ("lipid-abnormal2016", "中国成人血脂异常防治指南（2016年修订版）", "中华医学会心血管病学分会"),
    ("hf2024", "中国心力衰竭诊断和治疗指南2024", "中华医学会心血管病学分会"),
    ("hf2018", "中国心力衰竭诊断和治疗指南2018", "中华医学会心血管病学分会"),
    ("ccs2018", "慢性冠脉综合征患者医疗管理中国指南（2018）", "中华医学会心血管病学分会"),
    ("af2023", "心房颤动诊断和治疗中国指南", "中华医学会心血管病学分会"),
    ("pci2025", "经皮冠状动脉介入治疗指南（2025）", "中华医学会心血管病学分会"),
    ("vte2018", "肺血栓栓塞症诊治与预防指南", "中华医学会呼吸病学分会"),
    # --- 内分泌 ---
    ("t2d2020", "中国2型糖尿病防治指南（2020年版）", "中华医学会糖尿病学分会"),
    ("t1d2021", "中国1型糖尿病诊治指南（2021版）", "中华医学会糖尿病学分会"),
    ("dkd2021", "中国糖尿病肾脏病防治指南（2021年版）", "中华医学会糖尿病学分会"),
    ("insulin2016", "中国胰岛素注射技术指南（2016版）", "中华医学会糖尿病学分会胰岛素抵抗学组"),
    ("tnod2012", "甲状腺结节和分化型甲状腺癌诊治指南", "中华医学会内分泌学分会"),
    ("hyperthyroid2022", "甲状腺功能亢进症基层诊疗指南（2019年）", "中华医学会"),
    ("gout2019", "中国高尿酸血症与痛风诊疗指南(2019)", "中华医学会内分泌学分会"),
    ("gout-antiinflam2025", "痛风抗炎症治疗指南（2025版）", "中华医学会"),
    ("osteoporosis2017", "原发性骨质疏松症诊疗指南（2017）", "中华医学会骨质疏松和骨矿盐疾病分会"),
    # --- 呼吸（避开已入库的哮喘2020/COPD2021/支扩/肺结节/肺功能） ---
    ("cough2021", "咳嗽的诊断与治疗指南（2021）", "中华医学会呼吸病学分会"),
    ("copd2013", "慢性阻塞性肺疾病诊治指南（2013年修订版）", "中华医学会呼吸病学分会"),
    ("ipf2018", "特发性肺纤维化诊断和治疗中国专家共识", "中华医学会呼吸病学分会"),
    ("cap2016", "中国成人社区获得性肺炎诊断和治疗指南(2016年版)", "中华医学会呼吸病学分会"),
    ("osa2018", "中国成人阻塞性睡眠呼吸暂停诊治指南（2018修订版）", "中华医学会呼吸病学分会"),
    ("asthma-acute", "支气管哮喘急性发作评估及处理中国专家共识", "中华医学会呼吸病学分会"),
    # --- 神经/精神/睡眠 ---
    ("stroke2018", "中国急性缺血性脑卒中诊治指南2018", "中华医学会神经病学分会"),
    ("istie2022", "中国缺血性脑卒中和短暂性脑缺血发作二级预防指南2022", "中华医学会神经病学分会"),
    ("ih2019", "中国脑出血诊治指南（2019）", "中华医学会神经病学分会"),
    ("pd2020", "中国帕金森病治疗指南（第四版）", "中华医学会神经病学分会"),
    ("migraine2022", "中国偏头痛诊断与治疗指南（2022版）", "中国头痛研究协作组"),
    ("myastenia2022", "重症肌无力诊断和治疗中国指南（2022版）", "中华医学会神经病学分会神经免疫学组"),
    ("insomnia2017", "中国成人失眠诊断与治疗指南（2017版）", "中华医学会神经病学分会睡眠障碍学组"),
    ("alz2020", "中国阿尔茨海默病痴呆诊疗指南（2020年版）", "中华医学会神经病学分会"),
    ("schiz2020", "中国精神分裂症防治指南（第二版）", "中华医学会精神病学分会"),
    ("depression2020", "中国抑郁障碍防治指南（第二版）", "中华医学会精神病学分会"),
    # --- 消化/肝病 ---
    ("gastritis2017", "中国慢性胃炎诊治共识意见（2017，上海）", "中华医学会消化病学分会"),
    ("hp6-2022", "第六次全国幽门螺杆菌感染处理共识报告", "中华医学会消化病学分会幽门螺杆菌学组"),
    ("gerd2020", "胃食管反流病治疗中国共识（2020）", "中华医学会消化病学分会"),
    ("alc-liver2018", "酒精性肝病防治指南（2018年更新版）", "中华医学会肝病学分会"),
    ("hepatic-enceph2018", "肝硬化肝性脑病诊疗指南（2018年）", "中华医学会肝病学分会"),
    ("constip2019", "中国慢性便秘专家共识意见(2019，广州)", "中华医学会消化病学分会"),
    ("aih2021", "自身免疫性肝炎诊断和治疗指南（2021）", "中华医学会肝病学分会"),
    ("ibd2018", "炎症性肠病诊断与治疗的共识意见（2018年，北京）", "中华医学会消化病学分会炎症性肠病学组"),
    # --- 肾脏/风湿 ---
    ("hua-multidisc2023", "高尿酸血症相关疾病诊疗多学科专家共识（2023年版）", "中华医学会肾脏病学分会"),
    ("ra2022", "中国类风湿关节炎诊疗指南（2022版）", "中华医学会风湿病学分会"),
    ("as2020", "中国强直性脊柱炎诊疗指南", "中华医学会风湿病学分会"),
    ("sle2021", "中国系统性红斑狼疮诊疗指南", "中华医学会风湿病学分会"),
    ("giao2020", "糖皮质激素性骨质疏松症诊疗指南（2022）", "中华医学会骨质疏松和骨矿盐疾病分会"),
    ("osteoarthritis2018", "骨关节炎诊疗指南", "中华医学会骨科分会"),
    # --- 妇产/生殖/孕产 ---
    ("prenatal2018", "孕前和孕期保健指南（2018）", "中华医学会妇产科学分会产科学组"),
    ("mht2023", "中国绝经管理与绝经激素治疗指南（2023版）", "中华医学会妇产科学分会绝经学组"),
    ("pcos2017", "多囊卵巢综合征中国诊疗指南", "中华医学会妇产科学分会内分泌学组"),
    ("fibroid2017", "子宫肌瘤的诊治中国专家共识", "中华医学会妇产科学分会"),
    ("adeno2015", "子宫腺肌病诊治中国专家共识", "中华医学会妇产科学分会"),
    ("bv2021", "细菌性阴道病诊断和治疗专家共识", "中华医学会妇产科学分会感染性疾病协作组"),
    ("endomet2012", "子宫内膜异位症诊治指南（第三版）", "中华医学会妇产科学分会"),
    ("iron-pregnancy2014", "妊娠期铁缺乏和缺铁性贫血诊治指南", "中华医学会围产医学分会"),
    ("coc2025", "复方口服避孕药临床应用专家共识（2025）", "中华医学会"),
    # --- 儿科 ---
    ("child-asthma2016", "儿童支气管哮喘诊断与防治指南(2016年版)", "中华医学会儿科学分会呼吸学组"),
    ("child-flu2020", "儿童流感诊断与治疗专家共识(2020年版)", "中华医学会儿科学分会呼吸学组"),
    ("child-cap", "儿童社区获得性肺炎管理指南（2013修订）", "中华医学会儿科学分会呼吸学组"),
    ("child-car2021", "中国儿童过敏性鼻炎诊疗临床实践指南", "中华医学会儿科学分会"),
    ("child-anemia2022", "儿童缺铁和缺铁性贫血防治建议", "中华医学会儿科学分会血液学组"),
    ("child-tic", "抽动障碍诊断与治疗中国专家共识（2017版）", "中华医学会儿科学分会神经学组"),
    # --- 皮肤（避开已入库的AD/银屑病2018/荨麻疹2018/痤疮/体股癣/甲真菌/斑秃/雄脱/带状/PHN） ---
    ("psoriasis2023", "中国银屑病诊疗指南（2023版）", "中华医学会皮肤性病学分会"),
    ("rosacea2021", "中国玫瑰痤疮诊疗指南（2021版）", "中华医学会皮肤性病学分会"),
    ("melasma2021", "中国黄褐斑诊疗专家共识（2021版）", "中华医学会皮肤性病学分会"),
    ("ped-ad2023", "中国儿童特应性皮炎诊疗指南（2023版）", "中华医学会儿科学分会皮肤学组"),
    ("tinea-manus2026", "中国手癣和足癣诊疗指南（2026版）", "中华医学会皮肤性病学分会真菌学组"),
    ("prurigo-nod2023", "中国结节性湿疹诊疗指南", "中华医学会皮肤性病学分会"),
    ("vitiligo2019", "中国白癜风诊疗专家共识", "中华医学会皮肤性病学分会"),
    # --- 眼科/耳鼻喉/口腔 ---
    ("tao2022", "中国甲状腺相关眼病诊断和治疗指南（2022年）", "中华医学会眼科学分会"),
    ("cataract2018", "中国白内障诊疗指南（2018年修订版）", "中华医学会眼科学分会"),
    ("ome2021", "分泌性中耳炎诊断和治疗指南（2021修订版）", "中华医学会耳鼻咽喉头颈外科学分会"),
    ("epistaxis", "鼻出血的诊断与治疗中国专家共识", "中华医学会耳鼻咽喉头颈外科学分会"),
    ("perio2023", "中国牙周炎诊疗指南（2023年版）", "中华医学会口腔医学分会"),
    # --- 骨科/康复/老年 ---
    ("cervical2018", "颈椎病诊治与康复指南", "中华医学会"),
    ("cobb2020", "中国老年髋部骨折患者围手术期管理专家共识", "中华医学会老年医学分会"),
    ("fall2021", "老年跌倒风险综合管理专家共识", "中华医学会老年医学分会"),
    # --- 感染/疫苗/急救 ---
    ("rabies-exposure2023", "狂犬病暴露预防处置专家共识", "中华医学会"),
    ("febrile-neutropenia", "肿瘤放化疗相关中性粒细胞减少症规范化管理指南", "中华医学会肿瘤学分会"),
    ("neonatal-jaundice2014", "新生儿高胆红素血症诊断和治疗专家共识", "中华医学会围产医学分会"),
    # --- 合理用药基层丛书（中华全科医师杂志） ---
    ("flu-primary-med", "流行性感冒基层合理用药指南", "中华医学会全科医学分会"),
    ("insomnia-primary-med", "失眠基层合理用药指南", "中华医学会"),
    ("hp-primary-med", "幽门螺杆菌感染基层合理用药指南", "中华医学会"),
    ("dm-primary-med", "2型糖尿病基层合理用药指南", "中华医学会"),
]

BLOCK_TAGS = {
    "p", "div", "br", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6",
    "table", "tbody", "section", "article", "blockquote", "pre", "dt", "dd",
}
SKIP_TAGS = {"script", "style", "noscript", "svg", "head"}


class TextExtractor(html.parser.HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.parts = []
        self.skip_depth = 0
        self.title = ""
        self.in_title = False

    def handle_starttag(self, tag, attrs):
        if tag in SKIP_TAGS:
            self.skip_depth += 1
        if tag == "title":
            self.in_title = True
        if tag in BLOCK_TAGS:
            self.parts.append("\n")

    def handle_endtag(self, tag):
        if tag in SKIP_TAGS and self.skip_depth > 0:
            self.skip_depth -= 1
        if tag == "title":
            self.in_title = False
        if tag in BLOCK_TAGS:
            self.parts.append("\n")

    def handle_data(self, data):
        if self.in_title and not self.title:
            self.title = data.strip()
        if self.skip_depth == 0:
            self.parts.append(data)


def extract_text(raw_html):
    p = TextExtractor()
    p.feed(raw_html)
    text = "".join(p.parts)
    text = re.sub(r"[ \t\u3000]+", lambda m: " " if " " in m.group(0) or "\u3000" in m.group(0) else " ", text)
    lines = [ln.strip() for ln in text.splitlines()]
    lines = [ln for ln in lines if ln]
    return "\n".join(lines), p.title


def cjk_count(s):
    return len(re.findall(r"[\u4e00-\u9fff]", s))


def fetch(url):
    req = urllib.request.Request(url, headers={
        "User-Agent": UA,
        "Accept": "text/html,application/xhtml+xml",
        "Accept-Language": "zh-CN,zh;q=0.9",
    })
    try:
        with urllib.request.urlopen(req, timeout=25) as resp:
            body = resp.read()
            return resp.status, body
    except urllib.error.HTTPError as e:
        return e.code, b""
    except Exception as e:  # network errors -> unreachable
        print(f"    ERR {e}", flush=True)
        return 0, b""


def publisher_line(name, text, pub_hint):
    """Best-effort: capture journal/volume line and dates shown on the page."""
    m = re.search(r"中华[\u4e00-\u9fff]{1,12}杂志[^\n]{0,80}?\d{4}[,，]\s*\d{1,3}\s*\(\s*\d{1,3}\s*\)[^\n]{0,30}", text)
    bits = []
    if m:
        bits.append(m.group(0).strip()[:120])
    else:
        m2 = re.search(r"(中华[\u4e00-\u9fff]{1,15}分会[\u4e00-\u9fff]{0,15}学组?|中华[\u4e00-\u9fff]{1,15}杂志编辑委员会)[^\n]{0,60}", text)
        if m2:
            bits.append(m2.group(0).strip()[:120])
    dm = re.search(r"(发布|更新|印刷|日期)[:：]?\s*(20\d{2}[-/年]\d{1,2}[-/月]\d{0,2}日?)", text)
    if dm:
        bits.append(dm.group(2))
    return "%s，%s" % (pub_hint, "；".join(bits) if bits else "未标注")


def write_hit(slug, name, url, title, text, pub_hint):
    os.makedirs(OUT_DIR, exist_ok=True)
    path = os.path.join(OUT_DIR, slug + ".txt")
    with open(path, "w", encoding="utf-8") as f:
        f.write("# URL: %s\n" % url)
        f.write("# 标题: %s\n" % title)
        f.write("# 发布方/日期: %s\n\n" % publisher_line(name, text, pub_hint))
        f.write(text)
        if not text.endswith("\n"):
            f.write("\n")
    return path


SELE_BASE = "https://seleguide.yiigle.com/uploads/guide_html/"

# Wave 2: alternate exact-journal spellings for wave-1 misses (4th field = base URL override)
WAVE2 = [
    ("htn-primary2020", "国家基层高血压防治管理指南2020版", "国家心血管病中心"),
    ("af2023", "心房颤动诊断和治疗中国指南（2023）", "中华医学会心血管病学分会"),
    ("t2d2017", "中国2型糖尿病防治指南（2017年版）", "中华医学会糖尿病学分会"),
    ("cgm2018", "中国持续葡萄糖监测临床应用指南（2018年版）", "中华医学会糖尿病学分会"),
    ("cough2015", "咳嗽的诊断与治疗指南（2015版）", "中华医学会呼吸病学分会"),
    ("istie2022", "中国缺血性脑卒中和短暂性脑缺血发作二级预防指南（2022）", "中华医学会神经病学分会"),
    ("pd3rd", "中国帕金森病治疗指南（第三版）", "中华医学会神经病学分会"),
    ("gastritis2017", "中国慢性胃炎共识意见（2017年，上海）", "中华医学会消化病学分会"),
    ("hp-erase2019", "幽门螺杆菌根除与胃癌防控的专家共识意见（2019年，上海）", "中华医学会消化病学分会幽门螺杆菌学组"),
    ("hp5-2017", "第五次全国幽门螺杆菌感染处理共识报告", "中华医学会消化病学分会幽门螺杆菌学组"),
    ("gerd-primary", "胃食管反流病基层诊疗指南（2019年）", "中华医学会"),
    ("gout-primary", "痛风基层诊疗指南（2019年）", "中华医学会"),
    ("lipid-primary", "血脂异常基层诊疗指南（2018年）", "中华医学会"),
    ("obesity-primary", "肥胖症基层诊疗指南（2019年）", "中华医学会"),
    ("rhinitis-primary", "变应性鼻炎基层诊疗指南（2018年）", "中华医学会"),
    ("stroke-rehab2017", "中国脑卒中早期康复治疗指南", "中华医学会神经病学分会"),
    ("alc2018", "酒精性肝病防治指南(2018更新版)", "中华医学会肝病学分会"),
    ("hua-md2023", "中国高尿酸血症相关疾病诊疗多学科专家共识（2023年版）", "中华医学会肾脏病学分会等"),
    ("ra-guid", "中国类风湿关节炎诊疗指南", "中华医学会风湿病学分会"),
    ("sle2019", "中国系统性红斑狼疮诊疗指南（2019）", "中华医学会风湿病学分会"),
    ("child-cap2013", "儿童社区获得性肺炎管理指南(2013修订)(上)", "中华医学会儿科学分会呼吸学组"),
    ("pci2016", "中国经皮冠状动脉介入治疗指南（2016）", "中华医学会心血管病学分会"),
    ("cvd-pp2023", "中国心血管病一级预防指南", "中华医学会心血管病学分会"),
    ("mht2018", "中国绝经管理与绝经激素治疗指南（2018年）", "中华医学会妇产科学分会绝经学组"),
    ("psoriasis2023", "中国银屑病诊疗指南（2023版）", "中华医学会皮肤性病学分会", SELE_BASE),
    ("rosacea2021", "中国玫瑰痤疮诊疗指南（2021版）", "中华医学会皮肤性病学分会", SELE_BASE),
    ("melasma2021", "中国黄褐斑诊疗专家共识（2021版）", "中华医学会皮肤性病学分会", SELE_BASE),
    ("ped-ad2023", "中国儿童特应性皮炎诊疗共识（2023版）", "中华医学会儿科学分会皮肤学组", SELE_BASE),
    ("vitiligo-cn", "中国白癜风诊疗指南（2019）", "中华医学会皮肤性病学分会", SELE_BASE),
    ("urticaria2022", "中国荨麻疹诊疗指南（2022版）", "中华医学会皮肤性病学分会", SELE_BASE),
]


def name_variants(name):
    """Yield the name plus its full-width <-> half-width parenthesis variant."""
    yield name
    if "（" in name:
        yield name.replace("（", "(").replace("）", ")")
    elif "(" in name:
        yield name.replace("(", "（").replace(")", "）")


def try_candidates(items):
    manifest = os.path.join(OUT_DIR, "MANIFEST.txt")
    hits, misses = [], []
    for i, item in enumerate(items, 1):
        slug, name, pub = item[0], item[1], item[2]
        base = item[3] if len(item) > 3 else BASE
        path = os.path.join(OUT_DIR, slug + ".txt")
        if os.path.exists(path):
            continue
        hit = False
        tried = []
        for variant in name_variants(name):
            url = base + urllib.parse.quote(variant + ".html")
            status, body = fetch(url)
            tried.append((variant, status))
            time.sleep(SLEEP)
            if status != 200 or not body:
                continue
            raw = body.decode("utf-8", "replace")
            text, title = extract_text(raw)
            c = cjk_count(text)
            if c < MIN_CJK:
                print("SHORT %4d %s (%d cjk)" % (i, variant, c), flush=True)
                continue
            write_hit(slug, variant, url, title or variant, text, pub)
            hits.append(slug)
            with open(manifest, "a", encoding="utf-8") as f:
                year = (re.search(r"(20\d{2})", variant) or [""])[0]
                f.write("%s.txt | %s | %s | %s | %s\n" % (slug, variant, url, pub, year))
            print("HIT  %4d %s (%d cjk)" % (i, variant, c), flush=True)
            hit = True
            break
        if not hit:
            print("MISS %4d %s %s" % (i, name, tried), flush=True)
            misses.append(name)
    return hits, misses


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    h1, m1 = try_candidates(CANDIDATES)
    print("wave1: %d hits" % len(h1), flush=True)
    h2, m2 = try_candidates(WAVE2)
    print("wave2: %d hits" % len(h2), flush=True)
    allm = m1 + m2
    print("Total hits: %d, misses: %d" % (len(h1) + len(h2), len(allm)))
    manifest = os.path.join(OUT_DIR, "MANIFEST.txt")
    with open(manifest, "a", encoding="utf-8") as f:
        f.write("# 未命中（cmab.yiigle.com 无同名 guide_html 文件，含括号变体重试）:\n")
        for name in allm:
            f.write("#   %s\n" % name)
        f.write("# 不可达: 无（站点全程 200/404，未出现网络不可达）\n")


if __name__ == "__main__":
    main()
