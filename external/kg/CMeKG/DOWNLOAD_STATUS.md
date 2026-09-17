# CMeKG Download Summary

## Status: Manual Application Required

The CMeKG (Chinese Medical Knowledge Graph) dataset is **not freely downloadable** via automated means. Here are the findings:

## Available Sources (in order of accessibility)

### 1. Tianchi Dataset (Recommended)
- **URL**: https://tianchi.aliyun.com/dataset/81506
- **Access**: Requires manual application
- **Process**:
  1. Send email to `tianchi_open_dataset@alibabacloud.com`
  2. Include: name, affiliation, purpose (academic research only)
  3. Wait for approval (typically 3 business days)
  4. Receive download link via email

### 2. Baidu Pan Links (From CMeKGCrawler/MKGS projects)
- **CMeKG Neo4j Database**: https://pan.baidu.com/s/1I9GG4zD2rULAKA6Mb365LQ
  - 提取码: MKGS
  - Adapts to Neo4j v5.2 & v4.4
- **Alternate Neo4j Link**: https://pan.baidu.com/s/1UPtlNCEtH2P05LgG1veFEw
  - 提取码: MKGS

### 3. Google Drive (Via MedKGEval)
- **URL**: https://drive.google.com/file/d/1nRrhOWlvzdOgpatpQxJP0jiZrnRxhN8v/view?usp=sharing
- **Source**: MedKGEval repository (github.com/ZihengZZH/MedKGEval)
- **Note**: Contains CMeKG data for evaluation purposes

### 4. CMeKG Website (Tools Only)
- **URL**: http://nscc.zzu.edu.cn/know/
- **Provides**: NER, RE, and segmentation tools (not the KG data itself)
- **Note**: The website has limited API access; KG data is not directly downloadable

### 5. OpenCMKG (Alternative)
- **URL**: https://github.com/RuiqingDing/OpenCMKG
- **Data**: Aggregates from multiple sources including QAKG, OwnThink, CHIP2021
- **Format**: `triples.txt` (entity1, relation, entity2)
- **Note**: Not exactly CMeKG, but a similar Chinese medical KG

## CMeKG 2.0 Statistics
- 11,076 diseases
- 18,471 drugs
- 14,794 symptoms
- 3,546 diagnostic technologies
- 1,566,494 concept-relation triples

## Recommendation

For your use case (doctor-agent), the best options are:

1. **Apply via Tianchi** - Most official source, requires email
2. **Try Google Drive link** - May work without application
3. **Use Baidu Pan** - If you have Baidu account access

## Next Steps

1. Try downloading from Google Drive (link above)
2. Apply to Tianchi if Google Drive fails
3. Consider OpenCMKG as an alternative if CMeKG is unavailable
