#!/usr/bin/env python3
"""
bake_onnx.py - ONNX Runtime INT8 离线烘焙知识库向量

复刻 Go 端 vector-bake (internal/knowledge/bake.go) 全部逻辑:
  - gz 解压 (zstd/gzip 自动检测, magic bytes)
  - archiveBaseName (.json.zst / .json.gz -> .json)
  - seedFile 数据集分类 (~30 个 case, 与 Go seed.go seedFile() 完全一致)
  - buildSearchText (56 个 key 提取, valueToString 拼接, toLowerCase)
  - extractKey (14 字段优先级, fallback idx-N)
  - seedList / seedEntries / seedSingleton (dedupeKey 去重)
  - uuidFromSourceHash (sha256 -> UUIDv4, 版本/变体位)
  - bakePayload (source/type/entry_id/data)
  - 文本长度排序 (消除 ONNX padding 浪费, 3-5x 加速)
  - vectorSkipDatasets (4 个跳过: medkg/nmpa/cpubmed/icd10)
  - 文本截断 1024 字符 (rune-safe)

加速原理:
  1. ONNX Runtime INT8 量化 (2.27GB -> ~480MB, arm64 NEON SDOT 指令)
  2. dense-only (关闭 sparse/colbert, 只取 CLS token + L2 归一化)
  3. 进程内直调 (无 HTTP/Ollama 开销)
  4. 批量推理 (batch_size=128, 文本长度排序消除 padding)
  5. intra_op 多线程 (ONNX RT 内部并行)

用法:
  python3 bake_onnx.py \\
    --src=internal/knowledge/gz \\
    --host=localhost --port=6334 \\
    --collection=medical_knowledge \\
    --model=/path/to/bge-m3.onnx \\
    --tokenizer=BAAI/bge-m3 \\
    --workers=8 --batch-size=128 \\
    --int8 --recreate

依赖:
  pip install onnxruntime transformers qdrant-client zstandard numpy

INT8 量化模型获取:
  # 方式1: 下载现成 ONNX 模型后用 --int8 自动量化
  pip install onnxruntime transformers
  # 从 yunikosoftware/bge-m3-onnx GitHub Releases 下载 model.onnx
  # 或从 HuggingFace BAAI/bge-m3 用 optimum 导出:
  python -c "from optimum.onnxruntime import ORTModelForFeatureExtraction; m=ORTModelForFeatureExtraction.from_pretrained('BAAI/bge-m3', export=True); m.save_pretrained('./bge-m3-onnx')"

  # 方式2: 直接用 HuggingFace 模型缓存 (脚本自动找 model.onnx)
  # tokenizer 从 BAAI/bge-m3 下载; ONNX 模型手动指定路径

注意:
  - 首次从 Go/Ollama 切换到 ONNX 时, 务必加 --recreate 重建 collection
  - UUID 由 sha256(dataset|key + sha256(data)) 派生, Python 的 JSON 序列化
    与 Go 的 json.Marshal 不完全一致 (key 顺序/HTML 转义), 因此跨引擎
    UUID 不可混用; Python 内部多次烘焙幂等 (sort_keys=True 保证一致性)
"""

import argparse
import gzip
import hashlib
import json
import os
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

# ============================================================================
# 常量 (与 Go internal/knowledge/kb.go 完全一致)
# ============================================================================

DS_MEDICAL = "medical"
DS_DRUG = "drug"
DS_EMERGENCY = "emergency"
DS_FOOD_RISK = "foodrisk"
DS_LAB_TEST = "labtest"
DS_LITERATURE = "literature"
DS_MSD = "msd"
DS_CLINVAR = "clinvar"
DS_MEDLINEPLUS = "medlineplus"
DS_MEDINS = "medins"
DS_EML = "eml"
DS_FDA = "fda"
DS_NHC = "nhc"
DS_FHS = "fhs"
DS_AAP = "aap"
DS_HEALTH_MYTHS = "healthmyths"
DS_ESSENTIAL = "essential"
DS_ICD10 = "icd10"
DS_NMPA = "nmpa"
DS_MEDICAL_KG = "medkg"
DS_MEDICAL_DIALOGUES = "medicaldialogues"
DS_DISEASE_ENC = "diseaseenc"
DS_CPUBMED = "cpubmed"
DS_HUATUO = "huatuo"
DS_MEDICAL_QA = "medicalqa"
DS_TTD = "ttd"
DS_SIDER = "sider"
DS_BODY_PART = "bodypart"
DS_GROWTH = "growth"
DS_MILESTONES = "milestones"
DS_NEWBORN = "newborn"
DS_VERSION = "version"

# vectorSkipDatasets: 有专用检索工具的结构化数据集, 不需要向量化
# (与 Go bake.go vectorSkipDatasets 完全一致)
VECTOR_SKIP_DATASETS = {
    DS_MEDICAL_KG,   # 354,766 rows (medical_kg_triples.json)
    DS_NMPA,         # 167,615 rows (nmpa_drugs.json)
    DS_CPUBMED,      # 105,416 rows (cpubmed_kg.json)
    DS_ICD10,        #  35,862 rows (icd10_diseases.json)
}

# extractKey 优先级字段列表 (与 Go seed.go extractKey() 完全一致)
EXTRACT_KEY_PRIORITY = [
    "id", "ID", "clinvar_id", "icd10_code", "code", "Code",
    "name", "Name", "name_zh", "NameZH",
    "title", "Title", "variation", "Variation",
]

# buildSearchText 提取字段列表 (与 Go seed.go buildSearchText() 完全一致)
BUILD_SEARCH_KEYS = [
    "id", "ID", "code", "Code", "title", "Title", "name", "Name", "name_zh", "NameZH",
    "question", "Question", "answer", "Answer", "keywords", "Keywords",
    "symptoms", "Symptoms", "content", "Content", "gene", "Gene",
    "disease", "Disease", "relation", "Relation", "category", "Category",
    "department", "Department", "description", "Description",
    "definition", "Definition", "head", "Head", "entity1", "Entity1",
    "entity2", "Entity2", "variation", "Variation", "synonyms", "Synonyms",
    "part_key", "PartKey", "part_zh", "PartZH", "aliases", "Aliases",
    "conditions", "Conditions", "red_flags", "RedFlags",
    "self_care", "SelfCare", "departments", "Departments",
]

# seedList -> DS_MEDICAL 的文件名列表 (与 Go seed.go seedFile() 完全一致)
SEED_LIST_MEDICAL_FILES = [
    "thalassemia.json", "g6pd_deficiency.json",
    "nasopharyngeal_carcinoma.json", "hepatitis_b.json",
    "lactose_intolerance.json", "aldh2_deficiency.json",
    "dengue.json", "fungal_infections.json",
    "who_factsheets.json", "who_vaccines.json",
    "china_vaccines.json", "feeding_guidelines.json",
    "cdc_entries.json", "diabetes.json", "hypertension.json",
    "cardiovascular.json", "copd.json", "tuberculosis.json",
    "hp_infection.json",
    "common_diseases.json", "common_diseases_batch2.json",
    "common_diseases_batch3.json", "common_diseases_batch4.json",
    "elderly_care.json", "gyn_health.json", "ortho_child_health.json",
]

# seedEntries: 文件名 -> (dataset, JSON field name) 映射
# (与 Go seed.go seedFile() 中各 case 的 struct JSON tag 完全一致)
SEED_ENTRIES_MAP = {
    "msd_manual.json":          (DS_MSD,           "entries"),
    "clinvar.json":             (DS_CLINVAR,       "variants"),
    "medlineplus.json":         (DS_MEDLINEPLUS,    "entries"),
    "medins_drugs.json":        (DS_MEDINS,         "drugs"),
    "who_eml.json":             (DS_EML,            "entries"),
    "fda_drug_labels.json":     (DS_FDA,            "drugs"),
    "nhc_guides.json":          (DS_NHC,            "entries"),
    "fhs_guides.json":         (DS_FHS,            "entries"),
    "aap_articles.json":       (DS_AAP,            "entries"),
    "icd10_diseases.json":     (DS_ICD10,          "diseases"),
    "nmpa_drugs.json":         (DS_NMPA,           "drugs"),
    "medical_kg_triples.json": (DS_MEDICAL_KG,     "triples"),
    "medical_dialogues.json":  (DS_MEDICAL_DIALOGUES, "dialogues"),
    "disease_encyclopedias.json": (DS_DISEASE_ENC,  "diseases"),
    "cpubmed_kg.json":         (DS_CPUBMED,        "triples"),
    "huatuo_qa.json":          (DS_HUATUO,         "qa_pairs"),
    "medical_qa_pairs.json":   (DS_MEDICAL_QA,     "qa_pairs"),
}

# seedEntries (top-level array): 文件名 -> dataset
SEED_ENTRIES_TOP_LEVEL = {
    "health_myths.json":       DS_HEALTH_MYTHS,
    "body_part_triage.json":   DS_BODY_PART,
    "essential_medicines.json": DS_ESSENTIAL,
}

# seedSingleton: 文件名 -> (dataset, key)
SEED_SINGLETON_MAP = {
    "emergency_triage.json":   (DS_EMERGENCY, "rules"),
    "version.json":            (DS_VERSION,   "data"),
    "growth_standards.json":   (DS_GROWTH,    "data"),
    "newborn_care.json":       (DS_NEWBORN,   "data"),
    "ttd_data.json":           (DS_TTD,       "data"),
    "sider_drugs.json":        (DS_SIDER,     "data"),
}


# ============================================================================
# 数据结构
# ============================================================================

@dataclass
class KBRow:
    """与 Go internal/knowledge/seed.go KBRow 对应"""
    key: str          # 行 key (extractKey + dedupeKey)
    search_text: str  # buildSearchText 结果 (lowercased)
    data: str         # 原始 JSON 字符串 (json.dumps sort_keys=True)


# ============================================================================
# 归档文件处理 (与 Go internal/knowledge/archive.go 完全一致)
# ============================================================================

# zstd magic bytes: 28 b5 2f fd
_ZSTD_MAGIC = b"\x28\xb5\x2f\xfd"
# gzip magic bytes: 1f 8b
_GZIP_MAGIC = b"\x1f\x8b"


def decompress_archive(data: bytes) -> bytes:
    """解压知识库归档, 自动检测 zstd 或 gzip (与 Go decompressArchive 一致)"""
    if len(data) >= 4 and data[:4] == _ZSTD_MAGIC:
        import zstandard as zstd
        dec = zstd.ZstdDecompressor()
        return dec.decompress(data)
    if len(data) >= 2 and data[:2] == _GZIP_MAGIC:
        return gzip.decompress(data)
    # 未压缩 — 直接返回
    return data


def archive_base_name(path: str) -> str:
    """
    去掉归档扩展名, 返回源 JSON 文件名
    (与 Go archive.go archiveBaseName 一致)
    """
    base = os.path.basename(path)
    for ext in [".json.zst", ".json.gz", ".gz", ".zst"]:
        if base.endswith(ext):
            return base[:-len(ext)] + ".json"
    return base


# ============================================================================
# JSON 辅助函数 (与 Go internal/knowledge/seed.go 完全一致)
# ============================================================================

def value_to_string(v: Any) -> str:
    """
    将 JSON 值转为字符串 (与 Go seed.go valueToString 一致)
    - string -> 原样返回
    - list   -> 递归各元素用空格连接
    - number -> str(number)
    - bool   -> str(bool)
    - 其他   -> ""
    """
    if isinstance(v, str):
        return v
    if isinstance(v, list):
        return " ".join(value_to_string(e) for e in v)
    if isinstance(v, bool):
        return str(v)
    if isinstance(v, (int, float)):
        return str(v)
    return ""


def extract_key(obj: Any, idx: int) -> str:
    """
    从 JSON 对象提取稳定行 key (与 Go seed.go extractKey 一致)
    优先级: id > ID > clinvar_id > icd10_code > code > Code >
            name > Name > name_zh > NameZH > title > Title >
            variation > Variation
    fallback: idx-{idx}
    """
    if not isinstance(obj, dict):
        return f"idx-{idx}"
    for k in EXTRACT_KEY_PRIORITY:
        if k in obj:
            v = obj[k]
            if isinstance(v, str) and v:
                return v
    return f"idx-{idx}"


def build_search_text(obj: Any) -> str:
    """
    从 JSON 对象构建搜索文本 (与 Go seed.go buildSearchText 一致)
    按 BUILD_SEARCH_KEYS 顺序提取字段值, 空格连接, toLowerCase
    如果对象不是 dict 或没匹配到任何 key, fallback 为 JSON 全文 toLowerCase
    """
    if not isinstance(obj, dict):
        return json.dumps(obj, ensure_ascii=False).lower()

    parts = []
    for k in BUILD_SEARCH_KEYS:
        if k in obj:
            parts.append(value_to_string(obj[k]))

    if not parts:
        return json.dumps(obj, ensure_ascii=False).lower()

    return " ".join(parts).lower()


def dedupe_key(seen: dict, key: str) -> str:
    """
    去重 key (与 Go seed.go dedupeKey 一致)
    首次出现返回原 key; 第 N 次 (N>=2) 返回 "{key}-{N}"
    """
    n = seen.get(key, 0)
    seen[key] = n + 1
    if n == 0:
        return key
    return f"{key}-{n + 1}"


# ============================================================================
# seedFile — 数据集分类 (与 Go seed.go seedFile() 完全一致)
# ============================================================================

def seed_list(raw: bytes) -> list:
    """seedList: 从 JSON 数组构建行 (与 Go seed.go seedList 一致)"""
    items = json.loads(raw)
    if not isinstance(items, list):
        raise ValueError(f"expected JSON array, got {type(items).__name__}")
    rows = []
    seen = {}
    for i, item in enumerate(items):
        key = dedupe_key(seen, extract_key(item, i))
        data = json.dumps(item, ensure_ascii=False, sort_keys=True)
        rows.append(KBRow(key=key, search_text=build_search_text(item), data=data))
    return rows


def seed_entries(items: list) -> list:
    """
    seedEntries: 从列表元素构建行 (与 Go seed.go seedEntries 一致)
    Go 先 marshal items -> []json.RawMessage, 再逐个处理;
    Python 直接 json.dumps 每个元素 (sort_keys=True 保证幂等)
    """
    rows = []
    seen = {}
    for i, item in enumerate(items):
        key = dedupe_key(seen, extract_key(item, i))
        data = json.dumps(item, ensure_ascii=False, sort_keys=True)
        rows.append(KBRow(key=key, search_text=build_search_text(item), data=data))
    return rows


def seed_singleton(key: str, raw: bytes) -> KBRow:
    """seedSingleton: 整个文档作为一行 (与 Go seed.go seedSingleton 一致)"""
    obj = json.loads(raw)
    data = json.dumps(obj, ensure_ascii=False, sort_keys=True)
    return KBRow(key=key, search_text=build_search_text(obj), data=data)


def seed_file(base: str, raw: bytes) -> tuple:
    """
    seedFile: 数据集分类 (与 Go seed.go seedFile() 完全一致)
    返回 (dataset_name, rows) 或 ("", []) 如果文件名不匹配
    """
    # ── seedList -> DSMedical ──
    if base in SEED_LIST_MEDICAL_FILES:
        return DS_MEDICAL, seed_list(raw)

    # ── seedList -> DSDrug ──
    if base == "drug_contraindications.json":
        return DS_DRUG, seed_list(raw)

    # ── seedList -> DSFoodRisk ──
    if base == "food_risk.json":
        return DS_FOOD_RISK, seed_list(raw)

    # ── seedList -> DSLabTest ──
    if base == "lab_tests.json":
        return DS_LAB_TEST, seed_list(raw)

    # ── seedSingleton ──
    if base in SEED_SINGLETON_MAP:
        ds, key = SEED_SINGLETON_MAP[base]
        return ds, [seed_singleton(key, raw)]

    # ── seedEntries (从 wrapper 对象提取列表) ──
    if base in SEED_ENTRIES_MAP:
        ds, field = SEED_ENTRIES_MAP[base]
        obj = json.loads(raw)
        items = obj.get(field, [])
        return ds, seed_entries(items)

    # ── seedEntries (top-level array) ──
    if base in SEED_ENTRIES_TOP_LEVEL:
        ds = SEED_ENTRIES_TOP_LEVEL[base]
        items = json.loads(raw)
        if not isinstance(items, list):
            raise ValueError(f"expected JSON array for {base}, got {type(items).__name__}")
        return ds, seed_entries(items)

    # ── 特殊: literature.json (topics + articles) ──
    if base == "literature.json":
        obj = json.loads(raw)
        rows = []
        # topics 行: 整个数组作为一个 KBRow
        topics = obj.get("topics", [])
        topic_data = json.dumps(topics, ensure_ascii=False, sort_keys=True)
        rows.append(KBRow(
            key="topics",
            search_text=build_search_text(topics),  # list -> fallback lowercased JSON
            data=topic_data,
        ))
        # articles 行: 每篇文章一个 KBRow
        for i, article in enumerate(obj.get("articles", [])):
            key = article.get("id", "") if isinstance(article, dict) else ""
            if not key:
                key = f"art-{i}"
            data = json.dumps(article, ensure_ascii=False, sort_keys=True)
            rows.append(KBRow(
                key=key,
                search_text=build_search_text(article),
                data=data,
            ))
        return DS_LITERATURE, rows

    # ── 特殊: development_milestones.json (meta + ages) ──
    if base == "development_milestones.json":
        obj = json.loads(raw)
        rows = []
        # meta 行
        meta = {
            "source": obj.get("source", ""),
            "definition": obj.get("definition", ""),
        }
        meta_data = json.dumps(meta, ensure_ascii=False, sort_keys=True)
        rows.append(KBRow(
            key="meta",
            search_text=build_search_text(meta),
            data=meta_data,
        ))
        # ages 行
        for age in obj.get("ages", []):
            key = age.get("age_key", "") if isinstance(age, dict) else ""
            if not key:
                key = f"age-{len(rows)}"
            data = json.dumps(age, ensure_ascii=False, sort_keys=True)
            rows.append(KBRow(
                key=key,
                search_text=build_search_text(age),
                data=data,
            ))
        return DS_MILESTONES, rows

    # ── 未知文件: 静默跳过 (与 Go 一致) ──
    return "", []


# ============================================================================
# UUID + Payload (与 Go bake.go / syncer.go 完全一致)
# ============================================================================

def uuid_from_source_hash(source: str, content_hash: bytes) -> str:
    """
    从源名+内容哈希派生确定性 UUIDv4 (与 Go syncer.go uuidFromSourceHash 一致)
    sha256(source + "|" + content_hash) -> 取前 16 字节 -> 设 version(4)/variant 位
    """
    h = hashlib.sha256(source.encode("utf-8") + b"|" + content_hash).digest()
    b = bytearray(h[:16])
    b[6] = (b[6] & 0x0F) | 0x40  # version 4
    b[8] = (b[8] & 0x3F) | 0x80  # variant RFC 4122
    return f"{b[0:4].hex()}-{b[4:6].hex()}-{b[6:8].hex()}-{b[8:10].hex()}-{b[10:16].hex()}"


def bake_payload(dataset: str, key: str, data: str) -> dict:
    """
    构建 Qdrant point payload (与 Go bake.go bakePayload 一致)
    type 字段: DSDrug("drug") -> "drug", 其余 -> "knowledge"
    """
    typ = "drug" if dataset == DS_DRUG else "knowledge"
    return {
        "source": dataset,
        "type": typ,
        "entry_id": key,
        "data": data,
    }


# ============================================================================
# ONNX Runtime Embedder
# ============================================================================

def _model_total_size(model_path: str) -> int:
    """模型总字节 = 主文件 + external data (model.onnx_data), 不存在则只算主文件"""
    total = os.path.getsize(model_path)
    data_path = model_path + "_data"
    if os.path.exists(data_path):
        total += os.path.getsize(data_path)
    return total


class ONNXEmbedder:
    """
    bge-m3 ONNX Runtime embedder (dense-only, INT8)

    加载 ONNX 模型 -> tokenize -> CLS pooling -> L2 normalize
    可选 INT8 动态量化 (quantize_dynamic QInt8)
    """

    def __init__(self, model_path: str, tokenizer_name: str,
                 workers: int = 8, int8: bool = False, max_text_chars: int = 1024,
                 device: str = "cpu"):
        self.max_text_chars = max_text_chars
        self._model_path = self._ensure_model(model_path, int8)

        # 加载 tokenizer
        print(f"  tokenizer: {tokenizer_name}")
        from transformers import AutoTokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(tokenizer_name)

        # 创建 ONNX RT session
        import onnxruntime as ort
        session_opts = ort.SessionOptions()
        session_opts.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        session_opts.intra_op_num_threads = workers

        # cpu: CoreML/CPU 路线 — CoreML EP 对该模型动态 shape 分区执行不稳定（短输入
        #      能过, 真实 batch 中途报错），Mac 上禁用，长烘焙确定性优先
        # cuda: GPU 服务器烘焙 — 需 pip install onnxruntime-gpu（CUDA/cuDNN 就绪）
        import onnxruntime as _ort
        if device == "cuda":
            if "CUDAExecutionProvider" not in _ort.get_available_providers():
                raise RuntimeError(
                    "CUDAExecutionProvider 不可用 (available=%s)。"
                    "先 pip install onnxruntime-gpu 并确认 nvidia-smi 与 cuDNN 正常"
                    % _ort.get_available_providers()
                )
            providers = ["CUDAExecutionProvider", "CPUExecutionProvider"]
        else:
            providers = ["CPUExecutionProvider"]

        print(f"  ONNX model: {self._model_path}")
        print(f"  providers:  {providers}")
        print(f"  threads:    {workers}")

        self.session = ort.InferenceSession(
            self._model_path,
            sess_options=session_opts,
            providers=providers,
        )

        # 检查模型 I/O
        self.input_names = [inp.name for inp in self.session.get_inputs()]
        self.output_info = self.session.get_outputs()
        print(f"  inputs:     {self.input_names}")
        print(f"  outputs:    {[o.name for o in self.output_info]}")

    @staticmethod
    def _ensure_model(model_path: str, int8: bool) -> str:
        """如果 --int8 且模型未量化, 执行动态量化并缓存"""
        if not int8:
            return model_path

        basename = os.path.basename(model_path).lower()
        if "int8" in basename or "quant" in basename:
            return model_path  # 已量化

        int8_path = model_path.replace(".onnx", ".int8.onnx")
        if os.path.exists(int8_path):
            print(f"  INT8 缓存: {int8_path}")
            return int8_path

        print("  正在执行 INT8 动态量化 (首次运行, 需要几分钟)...")
        from onnxruntime.quantization import quantize_dynamic, QuantType
        quantize_dynamic(
            model_path,
            int8_path,
            weight_type=QuantType.QInt8,
        )
        # model.onnx 本体只有图结构, fp32 权重在 model.onnx_data (external data),
        # 尺寸要加上一起算, 否则会打出 "0.5MB -> 542MB" 的误导数字
        size_orig = _model_total_size(model_path) / 1024 / 1024
        size_int8 = os.path.getsize(int8_path) / 1024 / 1024
        print(f"  量化完成: {size_orig:.1f}MB -> {size_int8:.1f}MB")
        return int8_path

    def embed_batch(self, texts: list) -> list:
        """
        批量 embedding (dense-only)

        流程: 文本截断 -> tokenize -> ONNX 推理 -> CLS pooling -> L2 normalize
        返回: list[list[float]], 每个内层 list 是 1024 维向量
        """
        import numpy as np

        # 文本截断 (rune-safe, 与 Go bake.go bakeBatch 一致)
        processed = []
        for t in texts:
            if not t:
                t = ""
            if self.max_text_chars > 0 and len(t) > self.max_text_chars:
                t = t[:self.max_text_chars]
            processed.append(t)

        # Tokenize
        encoded = self.tokenizer(
            processed,
            padding=True,
            truncation=True,
            max_length=8192,  # bge-m3 支持最大 8192 tokens
            return_tensors="np",
        )

        # 构建 ONNX 输入
        inputs = {}
        for name in self.input_names:
            if name in encoded:
                inputs[name] = encoded[name]
            elif name == "token_type_ids":
                import numpy as np
                inputs[name] = np.zeros_like(encoded["input_ids"])

        # ONNX 推理
        outputs = self.session.run(None, inputs)

        # bge-m3 dense: CLS token (last_hidden_state[:, 0]) + L2 normalize
        # 输出按名取（optimum 导出可能带 pooler_output 等额外输出，顺序不定）
        output_names = [o.name for o in self.output_info]
        for key in ("last_hidden_state", "token_embeddings"):
            if key in output_names:
                last_hidden_state = outputs[output_names.index(key)]
                break
        else:
            last_hidden_state = outputs[0]  # (batch, seq_len, 1024)
        embeddings = last_hidden_state[:, 0]  # (batch, 1024) — CLS pooling

        # L2 normalize
        norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
        embeddings = embeddings / np.maximum(norms, 1e-9)

        # float32 -> list (Qdrant client 接受 list[float])
        return embeddings.astype(np.float32).tolist()


# ============================================================================
# Qdrant Baker
# ============================================================================

class QdrantBaker:
    """Qdrant collection 管理与批量 upsert"""

    def __init__(self, host: str, port: int, collection: str, recreate: bool = False):
        from qdrant_client import QdrantClient
        self.collection = collection
        self.client = QdrantClient(host=host, port=port, prefer_grpc=True, timeout=60)

        if recreate:
            try:
                self.client.delete_collection(collection_name=collection)
                print(f"  已删除旧 collection: {collection}")
            except Exception:
                pass

        self._ensure_collection()

    def _ensure_collection(self):
        """创建 collection (与 Go vector_store.go ensureCollection 一致)"""
        from qdrant_client.http.models import VectorParams, Distance

        collections = self.client.get_collections()
        existing = {c.name for c in collections.collections}
        if self.collection in existing:
            print(f"  collection 已存在: {self.collection}")
            return

        params = VectorParams(
            size=1024,
            distance=Distance.COSINE,
            on_disk=True,
        )
        # 尝试设 float16 (Qdrant client >= 1.7.0)
        try:
            from qdrant_client.http.models import Datatype
            params.datatype = Datatype.FLOAT16
        except (ImportError, AttributeError):
            pass  # 老版本不支持, 用默认 float32

        self.client.create_collection(
            collection_name=self.collection,
            vectors_config=params,
        )
        print(f"  已创建 collection: {self.collection} (1024d, cosine, float16, on_disk)")

    def upsert_points(self, points: list):
        """批量 upsert points"""
        from qdrant_client.http.models import PointStruct
        qdrant_points = [
            PointStruct(id=p["id"], vector=p["vector"], payload=p["payload"])
            for p in points
        ]
        self.client.upsert(
            collection_name=self.collection,
            points=qdrant_points,
            wait=True,
        )


# ============================================================================
# 烘焙逻辑 (与 Go bake.go bakeDataset / bakeBatch 一致)
# ============================================================================

def bake_dataset(embedder: ONNXEmbedder, baker: QdrantBaker,
                 dataset: str, rows: list,
                 batch_size: int, max_text_chars: int) -> tuple:
    """
    烘焙一个数据集 (与 Go bake.go bakeDataset 一致)
    1. 按文本长度排序 (消除 padding 浪费)
    2. 分批
    3. 每批: 截断文本 -> embed -> 构建 points -> upsert
    返回 (point_count, errors[])
    """
    if not rows:
        return 0, []

    # 按搜索文本长度排序 (ascending, 与 Go 一致)
    rows.sort(key=lambda r: len(r.search_text))

    # 分批
    batches = []
    for i in range(0, len(rows), batch_size):
        batches.append(rows[i:i + batch_size])

    n_batches = len(batches)
    total = 0
    errors = []

    for idx, batch in enumerate(batches):
        # 准备文本 (SearchText 为空时 fallback 到 Data, 与 Go 一致)
        texts = []
        for r in batch:
            t = r.search_text if r.search_text else r.data
            texts.append(t)

        # Embed
        try:
            vectors = embedder.embed_batch(texts)
        except Exception as e:
            msg = f"{dataset} batch {idx}: {e}"
            errors.append(msg)
            print(f"  ERROR: {msg}")
            continue

        # 构建 points
        points = []
        for j, r in enumerate(batch):
            entry_hash = hashlib.sha256(r.data.encode("utf-8")).digest()
            point_id = uuid_from_source_hash(dataset + "|" + r.key, entry_hash)
            points.append({
                "id": point_id,
                "vector": vectors[j],
                "payload": bake_payload(dataset, r.key, r.data),
            })

        # Upsert
        try:
            baker.upsert_points(points)
        except Exception as e:
            msg = f"{dataset} upsert batch {idx}: {e}"
            errors.append(msg)
            print(f"  ERROR: {msg}")
            continue

        total += len(points)

        # 进度 (与 Go 一致: >20 批次时每 10 批或最后一批输出)
        if n_batches > 20 and ((idx + 1) % 10 == 0 or idx + 1 == n_batches):
            pct = (idx + 1) * 100 / n_batches
            print(f"    progress {dataset:<16s} {idx+1}/{n_batches} batches ({pct:.0f}%)")

    return total, errors


def bake(gz_dir: str, embedder: ONNXEmbedder, baker: QdrantBaker,
         batch_size: int, max_text_chars: int):
    """
    主烘焙循环 (与 Go bake.go Bake 一致)
    glob *.json.*z* -> sort -> 逐文件解压 -> seedFile -> bakeDataset
    """
    # glob 匹配 (与 Go archiveGlob = "*.json.*z*" 一致)
    import glob
    pattern = os.path.join(gz_dir, "*.json.*z*")
    files = sorted(glob.glob(pattern))
    if not files:
        print(f"ERROR: no knowledge archives found in {gz_dir}")
        sys.exit(1)

    start = time.time()
    print(f"\nStarting gz -> Qdrant bake")
    print(f"  gz_dir:     {gz_dir}")
    print(f"  collection: {baker.collection}")
    print(f"  files:      {len(files)}")
    print(f"  batch_size: {batch_size}")
    print()

    datasets = 0
    total_points = 0
    all_errors = []
    skipped = []

    for f in files:
        base = archive_base_name(f)
        with open(f, "rb") as fh:
            raw = decompress_archive(fh.read())

        ds, rows = seed_file(base, raw)
        if not ds:
            print(f"  skip {base} (unsupported)")
            continue
        if ds in VECTOR_SKIP_DATASETS:
            print(f"  skip {base} (lookup-tool covered)")
            skipped.append(ds)
            continue

        n_batches = (len(rows) + batch_size - 1) // batch_size
        print(f"  baking {ds:<16s} {len(rows):6d} rows ({n_batches} batches)...")
        n, errs = bake_dataset(embedder, baker, ds, rows, batch_size, max_text_chars)
        total_points += n
        all_errors.extend(errs)
        datasets += 1
        print(f"  baked  {ds:<16s} {n:6d} points")

        # 手动 GC (与 Go runtime.GC() 一致, 释放当前数据集的内存)
        import gc
        gc.collect()

    duration = time.time() - start
    print()
    print(f"Bake finished")
    print(f"  datasets: {datasets}")
    print(f"  points:   {total_points}")
    print(f"  errors:   {len(all_errors)}")
    print(f"  skipped:  {len(skipped)} ({', '.join(skipped) if skipped else 'none'})")
    print(f"  duration: {duration:.1f}s")

    if all_errors:
        print("\nErrors:")
        for e in all_errors[:10]:
            print(f"  - {e}")
        if len(all_errors) > 10:
            print(f"  ... and {len(all_errors) - 10} more")

    return 0 if not all_errors else 1


# ============================================================================
# CLI 入口
# ============================================================================

def main():
    parser = argparse.ArgumentParser(
        description="ONNX Runtime INT8 离线烘焙知识库向量 (bge-m3 dense-only)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--src", default="internal/knowledge/gz",
                        help="gz 知识库目录 (default: internal/knowledge/gz)")
    parser.add_argument("--host", default="localhost",
                        help="Qdrant host (default: localhost)")
    parser.add_argument("--port", type=int, default=6334,
                        help="Qdrant gRPC port (default: 6334)")
    parser.add_argument("--collection", default="medical_knowledge",
                        help="Qdrant collection name (default: medical_knowledge)")
    parser.add_argument("--model", required=True,
                        help="ONNX 模型文件路径 (bge-m3)")
    parser.add_argument("--tokenizer", default="BAAI/bge-m3",
                        help="tokenizer 名称或路径 (default: BAAI/bge-m3)")
    parser.add_argument("--int8", action="store_true",
                        help="启用 INT8 动态量化 (首次自动量化并缓存)")
    parser.add_argument("--workers", type=int, default=8,
                        help="ONNX RT intra_op 线程数 (default: 8)")
    parser.add_argument("--batch-size", type=int, default=128,
                        help="批量大小 (default: 128)")
    parser.add_argument("--max-text-chars", type=int, default=1024,
                        help="文本截断字符数 (default: 1024, rune-safe)")
    parser.add_argument("--device", default="cpu", choices=["cpu", "cuda"],
                        help="推理设备 (default: cpu; cuda 需 onnxruntime-gpu)")
    parser.add_argument("--recreate", action="store_true",
                        help="删除旧 collection 从零开始 (首次从 Go/Ollama 切换时必用)")

    args = parser.parse_args()

    # 前置检查
    if not os.path.isdir(args.src):
        print(f"ERROR: src 目录不存在: {args.src}")
        sys.exit(1)
    if not os.path.isfile(args.model):
        print(f"ERROR: ONNX 模型不存在: {args.model}")
        print("  下载方式:")
        print("    # 从 optimum 导出:")
        print("    python -c \"from optimum.onnxruntime import ORTModelForFeatureExtraction; "
              "m=ORTModelForFeatureExtraction.from_pretrained('BAAI/bge-m3', export=True); "
              "m.save_pretrained('./bge-m3-onnx')\"")
        print("    # 然后指定 --model=./bge-m3-onnx/model.onnx")
        sys.exit(1)

    if args.device == "cuda" and args.int8:
        print("错误: --int8 (QInt8 动态量化) 面向 CPU NEON/AVX, CUDA 上无加速甚至报错。GPU 请用 fp32 原模型")
        sys.exit(1)

    print("=" * 50)
    print("  ONNX Runtime 离线烘焙知识库向量")
    print(f"  model:      {args.model}")
    print(f"  tokenizer:   {args.tokenizer}")
    print(f"  int8:        {args.int8}")
    print(f"  workers:     {args.workers}")
    print(f"  batch_size:  {args.batch_size}")
    print(f"  max_chars:   {args.max_text_chars}")
    print(f"  device:      {args.device}")
    print(f"  gz_dir:      {args.src}")
    print(f"  qdrant:      {args.host}:{args.port}")
    print(f"  collection:  {args.collection}")
    print(f"  recreate:    {args.recreate}")
    print("=" * 50)
    print()

    # 初始化 embedder
    print("[1/2] 初始化 ONNX Runtime embedder...")
    embedder = ONNXEmbedder(
        model_path=args.model,
        tokenizer_name=args.tokenizer,
        workers=args.workers,
        int8=args.int8,
        max_text_chars=args.max_text_chars,
        device=args.device,
    )

    # 初始化 Qdrant
    print()
    print("[2/2] 连接 Qdrant...")
    baker = QdrantBaker(
        host=args.host,
        port=args.port,
        collection=args.collection,
        recreate=args.recreate,
    )

    # 烘焙
    print()
    rc = bake(args.src, embedder, baker, args.batch_size, args.max_text_chars)
    sys.exit(rc)


if __name__ == "__main__":
    main()
