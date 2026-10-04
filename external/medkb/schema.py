"""统一格式 CorpusDoc：所有新 prose 源 convert 后的输出条目。

对应 Go 侧 internal/knowledge/corpus.go 的 CorpusDoc struct，字段增删必须两边同步。
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field, asdict

MAX_SUMMARY_CHARS = 800
MAX_BODY_BYTES = 24 * 1024  # 默认全文截断上限，与 plugin --max-body-bytes 一致

SOURCES = ("statpearls", "medgen", "lactmed", "nhc_mental", "firstaid", "travel_health")
KINDS = ("condition", "drug", "gene", "chromosome", "")


@dataclass
class CorpusDoc:
    source: str                      # statpearls | medgen | lactmed | ...
    id: str                          # 源内唯一，如 NBK430685
    lang: str                        # en | zh
    title: str
    summary: str = ""                # 检索加权主体
    kind: str = ""                   # condition | drug | gene
    title_zh: str = ""               # 有可靠中文映射才填
    url: str = ""
    sections: dict = field(default_factory=dict)   # treatment/prevention/...
    keywords: list = field(default_factory=list)
    body: str = ""                   # 全文（截断后）

    def to_json(self) -> dict:
        d = asdict(self)
        return {k: v for k, v in d.items() if v or k in ("source", "id", "lang", "title", "summary")}


def corpus_set(source: str, updated: str, entries: list[CorpusDoc]) -> list:
    """数据文件内容：统一种子格式后 corpus_<source>.json 顶层就是数组、每个元素一条
    CorpusDoc。旧 envelope 的顶层 source/updated 没有消费方（Go 解码器只认数组元素），
    随格式统一去掉；source 在这里当条目一致性检查用，updated 只写进日志。"""
    for e in entries:
        if e.source != source:
            raise ValueError(f"{source}: 条目 source={e.source!r} 与数据集不符")
    print(f"[{source}] {len(entries)} 条 (源版本 {updated})")
    return [e.to_json() for e in entries]


def validate_docs(path, source: str = "") -> list[str]:
    """返回错误列表；空列表 = 通过。source 由调用方（medkb validate <source>）给出。"""
    errors = []
    with open(path, encoding="utf-8") as f:
        entries = json.load(f)
    if not isinstance(entries, list) or not entries:
        return errors + [f"{path}: 顶层非空数组缺失"]
    seen_ids = set()
    for i, e in enumerate(entries):
        where = f"{path}#{i}"
        for req in ("id", "lang", "title"):
            if not e.get(req):
                errors.append(f"{where}: 缺必填字段 {req}")
        if source and e.get("source") != source:
            errors.append(f"{where}: source={e.get('source')!r} 与 {source!r} 不一致")
        elif e.get("source") not in SOURCES:
            errors.append(f"{where}: 未知 source {e.get('source')!r}（需加入 schema.SOURCES 与 Go 侧白名单）")
        if e.get("kind", "") not in KINDS:  # to_json 会丢掉空 kind，缺省视为合法
            errors.append(f"{where}: 非法 kind {e.get('kind')!r}")
        if e.get("lang") not in ("en", "zh"):
            errors.append(f"{where}: 非法 lang {e.get('lang')!r}")
        if len(e.get("summary", "")) > MAX_SUMMARY_CHARS + 100:
            errors.append(f"{where}: summary 超长 ({len(e['summary'])} 字符)")
        if len(e.get("body", "").encode()) > MAX_BODY_BYTES:
            errors.append(f"{where}: body 超过 {MAX_BODY_BYTES}B 上限")
        url = e.get("url", "")
        if url and not url.startswith(("http://", "https://")):
            errors.append(f"{where}: url 非法 {url!r}")
        kid = e.get("id", "")
        if kid in seen_ids:
            errors.append(f"{where}: 重复 id {kid!r}")
        seen_ids.add(kid)
        kw = e.get("keywords")
        if kw is not None and not all(isinstance(k, str) for k in kw):
            errors.append(f"{where}: keywords 含非字符串")
    return errors


def validate_icd11(path) -> list[str]:
    """icd11_terms.json 专用校验（非 CorpusDoc：exact_lookup 精确匹配表）。
    统一种子格式后顶层就是数组、每个元素一条术语。"""
    errors = []
    with open(path, encoding="utf-8") as f:
        terms = json.load(f)
    if not isinstance(terms, list) or not terms:
        return errors + [f"{path}: 顶层非空数组缺失"]
    seen = set()
    for i, t in enumerate(terms):
        where = f"{path}#terms[{i}]"
        if not t.get("icd11_code"):
            errors.append(f"{where}: 缺 icd11_code")
        if not (t.get("title_zh") or t.get("title_en")):
            errors.append(f"{where}: title_zh/title_en 至少一个")
        code = t.get("icd11_code", "")
        if code in seen:
            errors.append(f"{where}: 重复 icd11_code {code!r}")
        seen.add(code)
    return errors
