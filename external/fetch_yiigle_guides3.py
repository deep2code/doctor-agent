#!/usr/bin/env python3
"""Wave-3 harvest: MORE Chinese guideline full texts into external/yiigle_guides/raw/.

Reuses fetch/extract/write logic from external/fetch_yiigle_guides.py (imported,
not modified). Appends hits + a new 未命中 section to raw/MANIFEST.txt. Never
overwrites existing files or manifest lines. Sequential, >=1.0s apart.

Usage:
    python3 external/fetch_yiigle_guides3.py
"""
import os
import re
import sys
import time
import urllib.parse

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fetch_yiigle_guides as fg  # noqa: E402

BASE = fg.BASE
SELE = fg.SELE_BASE
SLEEP = 1.05
MIN_CJK = 1500
SUBSTRINGS = ("诊断", "治疗", "推荐", "筛查", "管理", "共识", "指南")

# (slug, verbatim published title, publisher hint)
CANDIDATES = [
    # --- wave4: 重试（不同拼写/裸标题） ---
    ("copd2021b", "慢性阻塞性肺疾病诊治指南（2021年修订版）", "中华医学会呼吸病学分会"),
    ("hosp-pap2018", "中国成人医院获得性肺炎与呼吸机相关性肺炎诊断和治疗指南(2018年版)", "中华医学会呼吸病学分会"),
    ("bronchiectasis2012", "成人支气管扩张症诊治专家共识", "中华医学会呼吸病学分会"),
    ("sepsis2018b", "中国脓毒症脓毒性休克急诊治疗指南(2018)", "中华医学会急诊医学分会"),
    ("heatstroke2019", "热射病急诊诊断与治疗专家共识", "中华医学会急诊医学分会"),
    ("cpr2021", "中国心肺复苏专家共识", "中华医学会急诊医学分会"),
    ("sudden-deaf2015", "突发性聋诊断和治疗指南", "中华医学会耳鼻咽喉头颈外科学分会"),
    ("otalgia", "分泌性中耳炎诊断和治疗指南(2012)", "中华医学会耳鼻咽喉头颈外科学分会"),
    ("dry-eye2013", "干眼临床诊疗专家共识(2013年)", "中华医学会眼科学分会"),
    ("vciz2019", "血管性认知障碍诊治指南（2019版）", "中华医学会神经病学分会"),
    ("pd-bare", "中国帕金森病治疗指南", "中华医学会神经病学分会"),
    ("neuropathic-pain", "神经病理性疼痛诊疗中国专家共识", "中华医学会疼痛学分会"),
    ("sarcopenia2021b", "老年人肌少症诊疗中国专家共识（2021）", "中华医学会老年医学分会"),
    ("polypharmacy2019", "老年人多重用药安全管理专家共识", "中华医学会老年医学分会"),
    ("ibs2015", "中国肠易激综合征专家共识意见（2015年，上海）", "中华医学会消化病学分会"),
    ("anvugib2018", "急性非静脉曲张性上消化道出血诊治指南（2018年，杭州）", "中华医学会消化病学分会"),
    ("ap-emergency", "急性胰腺炎急诊诊断及治疗专家共识", "中华医学会急诊医学分会"),
    ("hiv2021", "中国艾滋病诊疗指南（2021年版）", "中华医学会感染病学分会"),
    ("hfmd-primary", "手足口病诊疗指南(2010年版)", "卫生部"),
    ("child-cap-lower", "儿童社区获得性肺炎管理指南(2013修订)(下)", "中华医学会儿科学分会呼吸学组"),
    ("wheeze-children", "儿童支气管哮喘诊断与防治指南（2016年版）", "重复??"),
    # --- wave4: 新主题 ---
    ("lipid-mgmt2023", "中国血脂管理指南（2023年）", "中华医学会心血管病学分会"),
    ("thyroid-pregnancy2019", "妊娠和产后甲状腺疾病诊治指南（第2版）", "中华医学会内分泌学分会"),
    ("abnormal-utb2022", "异常子宫出血诊断与治疗指南", "中华医学会妇产科学分会"),
    ("pelvic-pid2019", "盆腔炎性疾病诊治规范（2019修订版）", "中华医学会妇产科学分会"),
    ("mna-infant2013", "中国婴幼儿牛奶蛋白过敏诊治循证建议", "中华医学会儿科学分会"),
    ("rhinitis2015", "变应性鼻炎诊断和治疗指南（2015年，长春）", "中华医学会耳鼻咽喉头颈外科学分会"),
    ("vte-disease2018", "中国血栓性疾病防治指南", "中华医学会"),
    ("aplasia2017", "再生障碍性贫血诊断与治疗中国专家共识（2017年版）", "中华医学会血液学分会"),
    ("mm2017", "中国多发性骨髓瘤诊治指南（2017年修订）", "中华医学会血液学分会"),
    ("cancer-pain2018", "癌症疼痛诊疗规范（2018年版）", "国家卫生健康委"),
    ("geriatric-htn2019", "中国老年高血压管理指南2019", "中华医学会老年医学分会"),
    ("geriatric-t2d2022", "中国老年2型糖尿病防治临床指南（2022年版）", "中华医学会老年医学分会"),
    ("inpatient-glucose2017", "中国住院患者血糖管理专家共识", "中华医学会内分泌学分会"),
    ("copd-稳定期", "慢阻肺???暂", "x"),
    ("ckd-anemia2020", "肾性贫血诊断和治疗共识", "中华医学会肾脏病学分会"),
    ("lupus2019b", "中国系统性红斑狼疮诊疗指南(2019)", "中华医学会风湿病学分会"),
    ("osteoporosis2022", "原发性骨质疏松症诊疗指南（2022）", "中华医学会骨质疏松和骨矿盐疾病分会"),
    ("low-back2020", "腰椎间盘突出症诊疗指南", "中华医学会骨科学分会"),
    ("autism2022", "儿童孤独症诊疗康复指南", "卫生部"),
    ("hbv2019", "慢性乙型肝炎防治指南（2019年版）", "中华医学会肝病学分会"),
    ("hepatic-failure2017", "肝衰竭诊治指南（2018年版）", "中华医学会感染病学分会"),
    ("ascites2017", "肝硬化腹水及相关并发症的诊疗指南", "中华医学会肝病学分会"),
]


def clinical(text):
    return any(s in text for s in SUBSTRINGS)


def try_one(item, bases):
    slug, name, pub = item[0], item[1], item[2]
    path = os.path.join(fg.OUT_DIR, slug + ".txt")
    if os.path.exists(path):
        return ("skip", name)
    tried = []
    for base in bases:
        for variant in fg.name_variants(name):
            url = base + urllib.parse.quote(variant + ".html")
            status, body = fg.fetch(url)
            tried.append((variant[:20], status))
            time.sleep(SLEEP)
            if status != 200 or not body:
                continue
            raw = body.decode("utf-8", "replace")
            text, title = fg.extract_text(raw)
            c = fg.cjk_count(text)
            if c < MIN_CJK or not clinical(text):
                continue
            fg.write_hit(slug, variant, url, title or variant, text, pub)
            with open(fg.OUT_DIR + "/MANIFEST.txt", "a", encoding="utf-8") as f:
                year = (re.search(r"(20\d{2})", variant) or [""])[0]
                f.write("%s.txt | %s | %s | %s | %s\n" % (slug, variant, url, pub, year))
            return ("hit", "%s (%d cjk)" % (variant, c))
    return ("miss", name)


def main():
    bases = [BASE, SELE]
    hits, misses = 0, []
    for i, item in enumerate(CANDIDATES, 1):
        if "???" in item[1]:
            continue  # unresolved placeholder title, skip entirely
        kind, info = try_one(item, bases)
        if kind == "hit":
            hits += 1
            print("HIT  %3d %s" % (i, info), flush=True)
        elif kind == "miss":
            misses.append(item[1])
            print("MISS %3d %s" % (i, item[1]), flush=True)
    with open(fg.OUT_DIR + "/MANIFEST.txt", "a", encoding="utf-8") as f:
        f.write("# 未命中 wave3（cmab/seleguide 均无同名 guide_html 全文，含括号变体）:\n")
        for m in misses:
            f.write("#   %s\n" % m)
    print("done: %d hits, %d misses" % (hits, len(misses)))


if __name__ == "__main__":
    main()
