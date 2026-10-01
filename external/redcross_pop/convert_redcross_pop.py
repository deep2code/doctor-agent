#!/usr/bin/env python3
"""Batch-19 wave-1 converter: 中国红十字会总会·生命健康科普 raw text -> medical entries.

Reads external/redcross_pop/raw/rc-*.txt (verbatim article text harvested by
fetch_redcross_pop.py) and writes internal/knowledge/data/redcross_first_aid.json.

Same field contract as convert_nhc_rumor.py: the medical dataset unmarshals into
knowledge.KnowledgeEntry, which has no summary_zh/details_zh, so answer content goes
into `prevention` (scored by retriever_keyword rule 4 and emitted by knowledge_search)
and red-flag/transport advice into `when_to_seek_care` (emitted, not scored).

Granularity: one entry per TOPIC, not per article — 自然灾害应急 (洪水/海啸/台风) and
暑期防淹溺 (自救/施救/预防) are split because their queries never co-occur, while the
two 交通事故 articles are merged into one entry with two citations because they answer
the same question and would otherwise compete for the same top5 slot.

Content is sliced by paragraph index range with a first/last hook assertion, so a
re-harvest that shifts the text fails loudly instead of publishing a silent truncation.
Paragraphs are copied verbatim (numbers are never rewritten); excluded paragraphs are
listed in EXCLUDED and mirrored in MANIFEST.txt.

Usage:
    python3 external/redcross_pop/convert_redcross_pop.py
"""
import glob
import json
import os
import re
import sys

RAW_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "raw")
OUT = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
                   "internal", "knowledge", "data", "redcross_first_aid.json")

CITATION_BASE = "中国红十字会总会·生命健康科普（redcross.org.cn）- "

# Paragraphs deliberately not published, with the reason (also recorded in MANIFEST).
EXCLUDED = {
    "rc-rescue-breathing": "整篇丢弃：俯卧压背法/仰卧压胸法为现行心肺复苏已弃用手法，"
                           "人工呼吸频次口径(14--16次/分)与去极化标准不符；同主题由 cs-cpr、"
                           "jkb_health、mdweekly 覆盖",
    "rc-traffic-accident": "仅保留第3段（伤员搬动与伤情判断），其余两段（报警呼救）与 2019 年"
                           "《交通事故的现场救护》同题且更旧",
    "rc-stroke-emergency-care": "第8段（用 l% 的重碳酸钠水润湿口唇）：原刊 OCR 把 1% 印成 l%，"
                                "数字不可信且属过时护理细节",
    "rc-wound-care": "第7段（小伤口涂红汞/紫药水）：汞溴红与龙胆紫已不推荐用于伤口，"
                     "与批次6D/既有碘伏口径冲突",
    "rc-child-fall-prevention": "第0段：2020 年复工复产疫情背景导语",
    "rc-food-storage": "第0段：疫情防控期间囤货背景导语",
    "rc-revenge-exercise": "第0/1/3/7/8/9段：疫情居家叙事与患者自述体，无可照做的指导",
}

# (entry id, condition_zh, category, [(file prefix, start, end, first hook, last hook)],
#  [(same shape) -> when_to_seek_care], [keywords])
CURATED = [
    ("rc-acute-abdomen-care", "急腹症的早期处理", "first_aid",
     [("rc-acute-abdomen", 0, 7, "当腹痛来得突然", "腹壁肌肉发生持续性收缩"),
      ("rc-acute-abdomen", 12, 18, "在送医院之前", "以便进一步处理")],
     [("rc-acute-abdomen", 7, 12, "由于引起急腹痛的原因很多", "反复呕吐以及不能大便")],
     ["急腹症", "急性腹痛", "突然肚子剧痛", "腹痛剧烈出冷汗", "板状腹",
      "肚子疼能不能吃止痛药", "急腹症为什么要禁食", "急腹症早期怎么处理",
      "腹痛呕吐能不能硬扛", "上腹中部疼痛是胃穿孔", "下腹疼痛是阑尾炎",
      "压痛反跳痛肌紧张"]),

    ("rc-chemical-self-protection", "化学品泄漏的个人防护", "first_aid",
     [("rc-chemical-accident", 1, 4, "化学事故特点", "农药杀虫剂等等"),
      ("rc-chemical-accident", 10, 19, "个人防护措施", "周围居民和单位不要开窗通风")],
     [("rc-chemical-accident", 19, 20, "受到危险化学品伤害时", "不要拖延")],
     ["化学品泄漏", "危险化学品事故", "液化气泄漏", "液氯泄漏", "氯气泄漏怎么逃",
      "关闭门窗防止毒气进入", "毛巾口罩呼吸道防护", "异味土壤", "遗弃化学品不要捡",
      "危化品运输车泄漏", "撤离到上风口", "不围观拨打报警电话"]),

    ("rc-chemical-evacuation", "化学毒区疏散与洗消", "first_aid",
     [("rc-chemical-accident", 4, 10, "组织民众防护主要措施", "抢救中毒人员等工作"),
      ("rc-chemical-accident", 20, 23, "公共防护措施", "全身式防毒衣")],
     [],
     ["化学事故疏散", "受染区", "染毒空气", "转移到坚固密封的建筑物", "上风方向疏散",
      "防毒面具", "化学毒区洗消", "化学毒区", "人群密集撤离不要挤踏"]),

    ("rc-drowning-self-rescue", "溺水时的自救", "first_aid",
     [("rc-child-drowning-summer", 0, 6, "据不完全统计", "用手反复按捏抽搐部位的肌肉"),
      ("rc-child-drowning-summer", 20, 21, "一旦溺水，千万不要惊慌", "设法自救")],
     [],
     ["溺水自救", "落水自救方法", "游泳抽筋怎么办", "手指抽筋向背侧扳直",
      "脚趾抽筋", "头后仰口鼻露出水面", "拍水呼救", "甩脱鞋子和重物", "淹溺窒息死亡"]),

    ("rc-drowning-rescue", "淹溺者救上岸后的处理", "first_aid",
     [("rc-child-drowning-summer", 6, 10, "如遇溺水者", "直接送医院检查治疗即可")],
     [],
     ["溺水急救步骤", "上岸后先清除口腔淤泥杂草", "清除口腔鼻咽部异物", "没有呼吸立即心肺复苏",
      "侧卧位保暖脱湿衣", "未成年人不要下水救人", "投掷救生圈木棍长绳", "大声呼救拨打120"]),

    ("rc-drowning-prevention", "预防儿童淹溺", "first_aid",
     [("rc-child-drowning-summer", 10, 20, "预防淹溺小贴士", "不要在未冻结实的冰面上行走或滑冰")],
     [],
     ["预防淹溺", "儿童游泳安全", "小学生独自去游泳", "正规游泳池", "江河湖泊水库池塘游泳",
      "禁止游泳的标牌", "雷雨天气不要游泳", "剧烈运动后不要立刻游泳", "游泳前了解水情做热身",
      "游泳时间过长疲劳", "头晕心慌气短上岸休息", "河边追逐打闹失足落水", "冰面滑冰"]),

    ("rc-child-fall-proof", "儿童坠落伤的居家防范", "first_aid",
     [("rc-child-fall-prevention", 1, 6, "北京儿童医院急诊科主任", "不要去做这些动作")],
     [("rc-child-fall-prevention", 6, 7, "如果孩子一旦发生了坠落伤", "加重他的伤害")],
     ["儿童坠落伤", "坠楼伤", "婴幼儿单独放沙发上", "婴儿床护栏床挡", "抽屉柜固定在墙上",
      "孩子单独锁在家里", "窗台阳台堆放可攀爬物品", "头伸到窗户外面", "窗户安全防护装置",
      "抛举孩子", "孩子从床上摔下来怎么办", "坠落伤严重程度和高度成正比吗"]),

    ("rc-flood-response", "洪水来临时怎么办", "first_aid",
     [("rc-flood-tsunami-typhoon", 1, 13, "一、洪水", "关掉电闸和自来水截门")],
     [],
     ["洪水应急措施", "山洪", "洪泛区", "15厘米深的水冲倒人", "60厘米深的水浮起小汽车",
      "远离洪水污水", "关掉电闸", "关掉电闸自来水截门", "电池收音机手摇发电", "洪水警报",
      "准备疏散带上应急箱"]),

    ("rc-flood-return-home", "洪水过后回家检查", "first_aid",
     [("rc-flood-tsunami-typhoon", 13, 25, "（三）回家之前的应对措施",
       "包括罐装食物和密封包装的食物或水")],
     [],
     ["洪水退去后回家检查什么", "洪水接触过的食物要扔", "宣布安全才能回家", "煤气味嘶嘶声",
      "掉落的电线水坑", "洪水退去立即清扫", "罐装食物密封包装", "现金手电筒瓶装水",
      "丢掉接触过洪水泥浆的药品"]),

    ("rc-tsunami-escape", "海啸逃生", "first_aid",
     [("rc-flood-tsunami-typhoon", 25, 44, "二、海啸", "可能造成墙倒屋塌")],
     [],
     ["海啸逃生", "海啸预警", "海底大地震", "持续20秒以上的地震", "海边低地",
      "高于海平面30米", "3公里外的内陆", "躲开坠落电线", "海啸警报拉响",
      "一波海浪过后下一波", "不要观看海浪", "不要进入被水包围的建筑物"]),

    ("rc-typhoon-preparedness", "台风防御", "first_aid",
     [("rc-flood-tsunami-typhoon", 44, 61, "三、台风", "如果不确定，扔掉")],
     [],
     ["台风防御", "台风将至", "台风警报", "台风将在48小时内来袭", "台风警报36小时内来袭",
      "用板子封住窗户和门", "家庭逃生预案演练", "给汽车加满油", "台风不要用蜡烛",
      "手电筒照明", "台风后自来水污染", "冰箱食物变质"]),

    ("rc-food-no-fridge", "不宜放冰箱的食物", "food_safety",
     [("rc-food-storage", 1, 11, "四大类不宜放冰箱", "反而更容易变黑、腐烂")],
     [],
     ["不宜放冰箱的食物", "土豆洋葱保存", "芋头番薯莲藕牛蒡", "根茎类蔬菜要不要冷藏",
      "鱼类冰箱放多久", "鱼体酸败", "馒头面包花卷放冰箱变干变硬", "热带水果放冰箱变黑",
      "香蕉芒果木瓜保存", "干爽纸箱阴凉背光"]),

    ("rc-food-storage-by-type", "按食物种类选择储存条件", "food_safety",
     [("rc-food-storage", 11, 19, "根据种类，选择合理的储藏条件", "放在冰箱里可以延缓")],
     [],
     ["粮食抽真空小包装", "玉米大米黄曲霉", "真空条件", "熟肉放保鲜盒几天",
      "腊肉香肠保存", "生肉冷藏还是冷冻", "冷冻肉提前一夜放冷藏室解冻",
      "蔬菜维生素C分解", "储藏温度越高分解越快"]),

    ("rc-stroke-position-care", "中风病人的现场照护", "first_aid",
     [("rc-stroke-emergency-care", 0, 8, "脑血管疾病，也称为脑血管意外", "不要移动上半身")],
     [("rc-stroke-emergency-care", 9, 10, "脑血管意外病人是否该住院", "以免拖延时间")],
     ["中风现场救护", "脑出血病人怎么搬动", "脑梗送医院前", "松开衣服上半身垫高",
      "呕吐物误入气管", "头部侧向一边", "大小便失禁就地处置", "不要移动上半身",
      "安静和暖", "肢体瘫痪失语昏迷", "睡眠中起来活动时肢体不灵活"]),

    ("rc-traffic-scene-rescue", "交通事故现场救护", "first_aid",
     [("rc-traffic-trauma-scene", 0, 12, "交通事故是最常见的", "社会心理帮助"),
      ("rc-traffic-accident", 2, 3, "对于伤员则不必急于把他们从车上", "予以早期处理")],
     [],
     ["交通事故现场救护", "车祸第一个发现者怎么办", "拨打122", "急救电话说清地点人数",
      "事故车辆起火爆炸再次倾覆", "切勿立即移动伤员", "关闭引擎危险报警闪光灯拉紧手刹",
      "先救命后治伤", "检伤分类", "保护事故现场", "车祸伤员颅脑损伤",
      "开放性骨折能搬动吗", "松开颈胸腰部贴身衣服", "清除口鼻呕吐物防窒息",
      "事故现场心理帮助"]),

    ("rc-tourniquet-use", "止血带的使用方法", "first_aid",
     [("rc-tourniquet-bandaging", 0, 3, "止血带是1886年埃斯马赫发明的", "严防勒伤组织")],
     [],
     ["止血带的使用", "四肢大动脉出血", "止血带绑在伤口上方", "患肢抬高数分钟",
      "垫上毛巾防组织擦伤", "上止血带标记时间", "宽的布条毛巾代替止血带", "每隔30分钟放松"]),

    ("rc-bandaging-method", "绷带与三角巾包扎法", "first_aid",
     [("rc-tourniquet-bandaging", 3, 10, "在外伤急救中，常常用到包扎", "由左向右")],
     [],
     ["绷带包扎方法", "三角巾包扎大伤口", "卷轴绷带", "软绷带", "石膏绷带", "橡皮膏粘膏",
      "包扎由左向右", "手臂弯着绑腿直着绑", "先盖消毒纱布", "包扎过紧局部肿胀",
      "包扎止血固定患肢"]),

    ("rc-wound-initial-care", "伤口的初步处理", "first_aid",
     [("rc-wound-care", 0, 7, "各种外伤，常常引起皮肤和软组织的损伤", "不要把碘酒、酒精涂入伤口内"),
      ("rc-wound-care", 8, 13, "在处理较大的创伤伤口时", "后者使细胞破裂")],
     [],
     ["伤口初步处理", "外伤伤口怎么消毒", "处理伤口的原则", "包扎要做到快准轻牢",
      "由里向外擦拭", "断肢保存方法", "断指怎么送医", "消毒纱布包好放进塑料袋",
      "断肢不要浸泡消毒液", "高渗低渗溶液泡断肢", "伤口化脓感染气性坏疽破伤风"]),

    ("rc-resume-exercise", "停练后恢复锻炼的原则", "exercise",
     [("rc-revenge-exercise", 6, 7, "B 年轻人要避免", "报复性锻炼"),
      ("rc-revenge-exercise", 10, 14, "其实不管哪个年龄阶段的人", "比较理想的运动时间")],
     [],
     ["报复性锻炼", "长时间不运动后恢复锻炼", "心肺功能和基础代谢能力下降",
      "运动量循序渐进", "最大心率的80%", "制定科学的锻炼计划", "空腹和饱餐后不宜运动",
      "餐后2小时运动", "运动时间不宜过早过晚", "突然大量运动适得其反"]),

    ("rc-elderly-resume-exercise", "老年人恢复锻炼量力而行", "exercise",
     [("rc-revenge-exercise", 2, 3, "A 老年人重拾锻炼注意量力而行", "量力而行"),
      ("rc-revenge-exercise", 4, 6, "长期不运动会导致骨量丢失", "宅居已久的老年人该如何运动锻炼")],
     [],
     ["老年人宅家很久后恢复锻炼", "老年人锻炼量力而行", "长期不运动骨量丢失", "长期不运动会加重骨质疏松",
      "骨质疏松乏力腰背", "有慢性基础性疾病的老年人运动", "宅居已久如何运动锻炼"]),
]


def parse(path):
    lines = open(path, encoding="utf-8").read().splitlines()
    head = {}
    for l in lines[:6]:
        if l.startswith("# "):
            k, _, v = l[2:].partition(":")
            head[k.strip()] = v.strip()
        elif l.strip():
            break
    body = [l.strip() for l in lines[4:] if l.strip()]
    # 来源/记者 credits sometimes sit at the end of the article text.
    while body and re.match(r"^(出品|审核|来源|策划|制图|编辑|校对|记者|通讯员)[：:]", body[-1]):
        head.setdefault("署名", []).append(body.pop())
    return head, body


def cjk(s):
    return len(re.findall(r"[\u4e00-\u9fff]", s))


def load_raw():
    out = {}
    for p in sorted(glob.glob(os.path.join(RAW_DIR, "*.txt"))):
        stem = os.path.basename(p).split("_")[0]
        head, body = parse(p)
        out[stem] = (head, body, p)
    return out


def cite(head):
    title = head.get("标题", "")
    prov = head.get("发布机构与日期", "")
    year_m = re.search(r"(20\d{2})", prov)
    if not year_m:
        sys.exit("no year in header of %s" % title)
    return {
        "type": "national_guideline",
        "title": CITATION_BASE + title + ("（%s）" % prov if prov else ""),
        "journal": "",
        "year": int(year_m.group(1)),
        "doi": "",
        "pmid": "",
        "level": "official_guideline",
        "url": head.get("URL", ""),
    }


def take(raw, stem, start, end, first_hook, last_hook):
    if stem not in raw:
        sys.exit("no raw file %s" % stem)
    head, body, _ = raw[stem]
    if end > len(body):
        sys.exit("%s has %d paras, segment asks for %d" % (stem, len(body), end))
    seg = body[start:end]
    if not seg or first_hook not in seg[0] or last_hook not in seg[-1]:
        sys.exit("segment mismatch %s[%d:%d]: want %r..%r got %r..%r"
                 % (stem, start, end, first_hook, last_hook,
                    seg[0][:30] if seg else "", seg[-1][:30] if seg else ""))
    return head, seg


def build():
    raw = load_raw()
    entries, covered = [], set()
    for eid, cond, cat, segs, wtc_segs, extra in CURATED:
        body, wtc, cites = [], [], []
        for stem, s, e, fh, lh in segs + wtc_segs:
            if stem not in covered:
                covered.add(stem)
        for stem, s, e, fh, lh in segs:
            head, seg = take(raw, stem, s, e, fh, lh)
            for p in seg:
                if not p:
                    sys.exit("empty paragraph in %s[%d:%d]" % (stem, s, e))
                body.append(p)
            c = cite(head)
            if c not in cites:
                cites.append(c)
        for stem, s, e, fh, lh in wtc_segs:
            head, seg = take(raw, stem, s, e, fh, lh)
            wtc.extend(seg)
            c = cite(head)
            if c not in cites:
                cites.append(c)
        if cjk("".join(body)) < 60:
            sys.exit("%s: entry body too short (%d CJK)" % (eid, cjk("".join(body))))
        kws, seen = [], set()
        for k in [cond] + extra:
            k = k.strip()
            if k and k not in seen:
                seen.add(k)
                kws.append(k)
        entries.append({
            "id": eid,
            "condition_zh": cond,
            "condition_en": "",
            "category": cat,
            "keywords": kws,
            "prevention": body,
            "when_to_seek_care": wtc,
            "citations": cites,
        })

    stems = set(raw)
    dropped = sorted(stems - covered)
    print("entries: %d | raw files: %d | covered: %d | not published: %s"
          % (len(entries), len(stems), len(covered), ",".join(dropped)))
    ids = [e["id"] for e in entries]
    dupes = {i for i in ids if ids.count(i) > 1}
    if dupes:
        sys.exit("duplicate ids: %s" % dupes)
    for d in dropped:
        if d not in EXCLUDED:
            sys.exit("raw file %s neither curated nor explained" % d)

    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(entries, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print("wrote %s" % OUT)
    total = sum(cjk(p) for e in entries for p in e["prevention"])
    print("body CJK: %d | avg per entry: %d | citations: %d"
          % (total, total // len(entries), sum(len(e["citations"]) for e in entries)))


if __name__ == "__main__":
    build()
