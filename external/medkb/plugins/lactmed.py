"""LactMed（哺乳期用药安全，美国 NLM，公有领域）。

raw: LitArch FTP 整包 tar.gz（~210MB）。下载地址两步发现：
  1. 书页 https://www.ncbi.nlm.nih.gov/books/NBK501922/ 内 grep litarch 目录链接
  2. 列目录取 *.tar.gz
convert: JATS XML → CorpusDoc{kind=drug}。sections 按小节名归一；
title_zh/keywords 用内置 60 常用药中文映射（仅覆盖命中项）。
"""

from __future__ import annotations

import re
import tarfile
import xml.etree.ElementTree as ET

from .. import http
from ..schema import CorpusDoc, corpus_set
from . import base

BOOK_PAGE = "https://www.ncbi.nlm.nih.gov/books/NBK501922/"

# 常用药英文→中文（通用名）。映射缺省时 title_zh 留空，检索走 Go 侧 corpusSynonyms。
ZH_DRUGS = {
    "ibuprofen": "布洛芬", "acetaminophen": "对乙酰氨基酚", "paracetamol": "对乙酰氨基酚",
    "aspirin": "阿司匹林", "amoxicillin": "阿莫西林", "ampicillin": "氨苄西林",
    "penicillin": "青霉素", "penicillin g": "青霉素G", "azithromycin": "阿奇霉素",
    "erythromycin": "红霉素", "clarithromycin": "克拉霉素", "cephalexin": "头孢氨苄",
    "ceftriaxone": "头孢曲松", "cefaclor": "头孢克洛", "cefuroxime": "头孢呋辛",
    "clindamycin": "克林霉素", "metronidazole": "甲硝唑", "ciprofloxacin": "环丙沙星",
    "levofloxacin": "左氧氟沙星", "ofloxacin": "氧氟沙星", "moxifloxacin": "莫西沙星",
    "sulfamethoxazole": "磺胺甲噁唑", "nitrofurantoin": "呋喃妥因", "fluconazole": "氟康唑",
    "ketoconazole": "酮康唑", "acyclovir": "阿昔洛韦", "oseltamivir": "奥司他韦",
    "prednisone": "泼尼松", "prednisolone": "泼尼松龙", "dexamethasone": "地塞米松",
    "budesonide": "布地奈德", "hydrocortisone": "氢化可的松",
    "ibuprofen lysine": "布洛芬赖氨酸", "diclofenac": "双氯芬酸", "indomethacin": "吲哚美辛",
    "naproxen": "萘普生", "meloxicam": "美洛昔康", "celecoxib": "塞来昔布",
    "morphine": "吗啡", "fentanyl": "芬太尼", "codeine": "可待因", "tramadol": "曲马多",
    "tramadol and acetaminophen": "曲马多对乙酰氨基酚",
    "diazepam": "地西泮", "lorazepam": "劳拉西泮", "sertraline": "舍曲林",
    "fluoxetine": "氟西汀", "paroxetine": "帕罗西汀", "escitalopram": "艾司西酞普兰",
    "amitriptyline": "阿米替林", "metformin": "二甲双胍", "insulin": "胰岛素",
    "levothyroxine": "左甲状腺素", "methimazole": "甲巯咪唑", "propylthiouracil": "丙硫氧嘧啶",
    "warfarin": "华法林", "heparin": "肝素", "enoxaparin": "依诺肝素",
    "amlodipine": "氨氯地平", "nifedipine": "硝苯地平", "metoprolol": "美托洛尔",
    "propranolol": "普萘洛尔", "labetalol": "拉贝洛尔", "atenolol": "阿替洛尔",
    "captopril": "卡托普利", "enalapril": "依那普利", "losartan": "氯沙坦",
    "hydrochlorothiazide": "氢氯噻嗪", "furosemide": "呋塞米", "spironolactone": "螺内酯",
    "atorvastatin": "阿托伐他汀", "omeprazole": "奥美拉唑", "pantoprazole": "泮托拉唑",
    "ranitidine": "雷尼替丁", "famotidine": "法莫替丁", "salbutamol": "沙丁胺醇",
    "albuterol": "沙丁胺醇", "montelukast": "孟鲁司特", "loratadine": "氯雷他定",
    "cetirizine": "西替利嗪", "chlorpheniramine": "氯苯那敏", "promethazine": "异丙嗪",
    "carbamazepine": "卡马西平", "valproic acid": "丙戊酸", "lamotrigine": "拉莫三嗪",
    "phenytoin": "苯妥英", "levetiracetam": "左乙拉西坦", "topiramate": "托吡酯",
    "lithium": "锂", "clozapine": "氯氮平", "risperidone": "利培酮",
    "olanzapine": "奥氮平", "quetiapine": "喹硫平", "aripiprazole": "阿立哌唑",
    "haloperidol": "氟哌啶醇", "methyldopa": "甲基多巴", "bromocriptine": "溴隐亭",
    "cabergoline": "卡麦角林", "ergotamine": "麦角胺", "misoprostol": "米索前列醇",
    "mifepristone": "米非司酮", "lamivudine": "拉米夫定", "zidovudine": "齐多夫定",
    "tenofovir": "替诺福韦", "lopinavir": "洛匹那韦", "rifampin": "利福平",
    "isoniazid": "异烟肼", "pyrazinamide": "吡嗪酰胺", "ethambutol": "乙胺丁醇",
    "albuterol and ipratropium": "沙丁胺醇异丙托溴铵", "ipratropium": "异丙托溴铵",
    "theophylline": "茶碱", "ambroxol": "氨溴索", "guaifenesin": "愈创甘油醚",
    "dextromethorphan": "右美沙芬", "pseudoephedrine": "伪麻黄碱",
}

# JATS 小节标题 → 归一化 section key
SECTION_KEYS = [
    (re.compile(r"^summary of use during lactation", re.I), "lactation_summary"),
    (re.compile(r"^drug levels", re.I), "drug_levels"),
    (re.compile(r"^infant levels", re.I), "infant_levels"),
    (re.compile(r"^effects in breastfed infants", re.I), "infant_effects"),
    (re.compile(r"^effects on lactation", re.I), "lactation_effects"),
    (re.compile(r"^alternate drugs", re.I), "alternate_drugs"),
]


def fetch(ctx):
    tar = ctx.raw_dir / "lactmed_NBK501922.tar.gz"
    if not tar.exists() or tar.stat().st_size == 0:
        # 两步发现：书页 → litarch 目录 → tar 链接
        page = http.get(BOOK_PAGE)
        m = re.search(r'href="(https://ftp\.ncbi\.nlm\.nih\.gov/pub/litarch/[0-9a-f]{2}/[0-9a-f]{2}/)"', page)
        if not m:
            raise RuntimeError("书页未找到 litarch 目录链接，FTP 布局可能已变")
        listing = http.get(m.group(1))
        m2 = re.search(r'href="([^"]+\.tar\.gz)"', listing)
        if not m2:
            raise RuntimeError("litarch 目录中未找到 tar.gz")
        url = m.group(1) + m2.group(1)
        ctx.log(f"发现下载地址: {url}")
        http.download(url, tar)
    xdir = ctx.raw_dir / "x"
    if not (xdir / "NBK501922").exists():
        ctx.log("解压 tar.gz …")
        with tarfile.open(tar, "r:gz") as tf:
            tf.extractall(xdir, filter="data")
        ctx.log("解压完成")
    else:
        ctx.log("已解压，跳过")


def _local(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def _text(el) -> str:
    return " ".join("".join(el.itertext()).split())


def _zh_for(title: str) -> str:
    t = title.lower().strip()
    if t in ZH_DRUGS:
        return ZH_DRUGS[t]
    for en, zh in ZH_DRUGS.items():
        if t.startswith(en + " ") or f" {en} " in f" {t} ":
            return zh
    return ""


def _cap_bytes(text: str, max_bytes: int) -> str:
    """按 UTF-8 字节数截断（与 schema.MAX_BODY_BYTES 校验一致）。"""
    raw = text.encode("utf-8")
    if len(raw) <= max_bytes:
        return text
    return raw[:max_bytes - 16].decode("utf-8", "ignore").rstrip() + "\n\n[truncated]"


def _doc_from_article(path, max_body: int) -> CorpusDoc | None:
    try:
        root = ET.parse(path).getroot()
    except ET.ParseError:
        return None
    # BITS DTD: book-part-wrapper > book-part > title-group > title = 药名
    # （book-meta 里是 book-title，标签名不同，不会误取）
    title = ""
    wrapper_id = root.get("id") or ""
    for el in root.iter():
        if _local(el.tag) == "title":
            title = _text(el)
            break
    if not title:
        return None
    sections: dict[str, str] = {}
    body_parts: list[str] = []
    for sec in root.iter():
        if _local(sec.tag) != "sec":
            continue
        sec_title = ""
        paras: list[str] = []
        for child in sec:
            if _local(child.tag) == "title":
                sec_title = _text(child)
            elif _local(child.tag) == "p":
                paras.append(_text(child))
        if not paras:
            continue
        text = "\n\n".join(p for p in paras if p)
        body_parts.append(text)
        for pat, key in SECTION_KEYS:
            if pat.match(sec_title):
                sections.setdefault(key, text[:4000])
                break
    if not sections:
        return None  # 事实页/非药物记录
    body = _cap_bytes("\n\n".join(body_parts), max_body)
    summary = (sections.get("lactation_summary") or body.split("\n\n", 1)[0])[:600]
    zh = _zh_for(title)
    keywords = [title.lower()]
    if zh:
        keywords.append(zh)
    if wrapper_id:
        keywords.append(wrapper_id.replace("-", " "))
    return CorpusDoc(
        source="lactmed", id=f"lactmed-{path.stem}", lang="en", title=title,
        summary=summary, kind="drug", title_zh=zh,
        sections=sections or None, keywords=keywords, body=body,
    )


def convert(ctx):
    max_body = ctx.args.get("max_body_bytes") or 24576
    max_chapters = ctx.args.get("max_chapters") or 0
    docs = []
    for xml_path in sorted((ctx.raw_dir / "x").rglob("*.nxml")):
        d = _doc_from_article(xml_path, max_body)
        if d:
            docs.append(d)
            if max_chapters and len(docs) >= max_chapters:
                break
    ctx.log(f"{len(docs)} 药物条目（title_zh {sum(1 for d in docs if d.title_zh)} 条命中中文映射）")
    out = base.out_path("lactmed")
    base.write_json(out, corpus_set("lactmed", base.today(), docs))
    ctx.log(f"写出 {out} ({out.stat().st_size >> 10}KiB)")
