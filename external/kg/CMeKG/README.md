# CMeKG Data — Download Summary

## Repositories Checked

### 1. CMeKGCrawler (https://github.com/huyuanxin/CMeKGCrawler)
- **Status**: Crawler code is dead (CMeKg website no longer accessible)
- **Data available**: Only Neo4j database dumps via Baidu Pan
- **doc/ directory**: 4 markdown files (entity schema definitions)

### 2. MKGS (https://github.com/huyuanxin/MKGS)
- **Status**: Full Spring Boot application (admin + QA + diagnosis)
- **Data**: Neo4j database dumps via Baidu Pan (same as above)
- **doc/关系/**: Same 4 relationship schema files (with camelCase field names)

## Files Downloaded

| File | Size | Source |
|------|------|--------|
| `doc/疾病.md` | 2,856 bytes | CMeKGCrawler (26 disease entity fields) |
| `doc/症状.md` | 2,486 bytes | CMeKGCrawler (19 symptom entity fields) |
| `doc/药物.md` | 644 bytes | CMeKGCrawler (8 drug entity fields) |
| `doc/诊疗.md` | 504 bytes | CMeKGCrawler (3 treatment entity fields) |

## Knowledge Graph Schema

The CMeKG contains **~140,000 entities** and **~700,000 relationships** across 4 node types:

### Node Types & Key Fields
- **Disease (疾病)**: name, ICD-10, complication, SymptomAndSign, Drug therapy, Treatment programs, RelatedDisease, RelatedSymptom
- **Symptom (症状)**: check, Infectious, DrugTherapy, stage, SpreadWay
- **Drug (药物)**: Ingredients, OTC, Adverse reactions, Indications, Contraindications
- **Treatment (诊疗)**: RelatedDisease, RelatedSymp, CheckSubject

### Relationship Types (from Neo4j)
- Disease → Symptom (临床症状及体征)
- Disease → Drug (药物治疗)
- Disease → Treatment (辅助治疗/手术治疗)
- Disease → Disease (并发症/相关疾病)
- Symptom → Disease (相关疾病)
- Drug → Disease (适应症/不良反应)

## Data Access (Neo4j Database)

The actual structured data is only available as Neo4j database dumps via Baidu Pan:
- **Link 1**: https://pan.baidu.com/s/1I9GG4zD2rULAKA6Mb365LQ (提取码: MKGS) — Neo4j v5.2 & v4.4
- **Link 2**: https://pan.baidu.com/s/1UPtlNCEtH2P05LgG1veFEw (提取码: MKGS)

**Note**: No JSON/CSV data files are available in either GitHub repository. The crawler source code is Java (Spring + Neo4j OGM) and is no longer functional.
