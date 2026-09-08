/* 医答 · 人体解剖图（教科书级分层示意 + 三级钻取）
 * 层级：全身 → 系统视图（八大系统） → 器官聚焦（viewBox 自动缩放）
 * 数据驱动 SVG：骨骼层 / 器官层 / 血管层 / 标注层独立可控。
 * 点击器官 → 详情卡；滚轮缩放、拖拽平移、双击复位；支持全屏弹窗。
 * 配套动画见 anatomy-anim.js（AnatomyAnim）。
 * 全部矢量自绘，无外部资源，离线可用。
 */
(function () {
  "use strict";

  /* ---------- 系统定义（order = 导航顺序） ---------- */
  var SYSTEMS = {
    motion: { name: "运动系统（骨骼）", color: "#8a97a5", desc: "颅骨、脊柱、胸廓、骨盆与四肢骨构成支架，保护脏器并支撑运动。" },
    nerve:  { name: "神经系统", color: "#8b5cf6", desc: "脑与脊髓整合全身信号，调控感觉、运动与内脏活动。" },
    endo:   { name: "内分泌系统", color: "#d97706", desc: "垂体、甲状腺、肾上腺等腺体分泌激素，调节代谢、生长与水电解质平衡。" },
    resp:   { name: "呼吸系统", color: "#0ea5e9", desc: "气道与肺完成氧气与二氧化碳的气体交换。" },
    circ:   { name: "循环系统", color: "#dc2626", desc: "心脏与血管把氧气、养分送往全身并回收代谢废物。" },
    digest: { name: "消化系统", color: "#ea580c", desc: "消化管道与肝胆胰协同完成消化、吸收与解毒。" },
    immune: { name: "免疫系统", color: "#16a34a", desc: "脾脏与淋巴系统识别清除病原，过滤血液。" },
    uri:    { name: "泌尿系统", color: "#0d9488", desc: "肾脏过滤血液生成尿液，调节水电解质与血压。" }
  };
  var SYSTEM_ORDER = ["nerve", "resp", "circ", "digest", "uri", "endo", "immune", "motion"];

  /* 系统视图的取景区域（viewBox，画布 460x660） */
  var SYSTEM_REGIONS = {
    nerve:  "110,0,240,470",
    endo:   "150,10,220,400",
    resp:   "100,40,260,300",
    circ:   "100,40,300,420",
    digest: "100,240,280,320",
    immune: "110,70,290,420",
    uri:    "130,330,220,220",
    motion: "60,0,340,660"
  };

  /* ---------- 器官/结构定义（正面观） ----------
   * layer:  skel=骨骼 vess=血管 org=器官（默认 org）
   * unit:   与提问部位(id)对应，命中时高亮
   * lb:     标注线终点 [x,y]（标注层使用）
   * sysOnly: true = 仅系统视图显示（全身图过小，细节反而干扰）
   */
  var PARTS = [
    /* ===== 骨骼层 ===== */
    { id: "skull", name: "颅骨", en: "Cranium", sys: "motion", unit: "head", layer: "skel", lb: [230, 30],
      info: "包围并保护脑组织，构成头面部支架。",
      d: '<path d="M196,58 Q196,22 230,22 Q264,22 264,58 Q264,74 256,84 Q246,94 230,94 Q214,94 204,84 Q196,74 196,58 Z"/><path d="M204,84 Q230,96 256,84" fill="none"/>' },
    { id: "mandible", name: "下颌骨", en: "Mandible", sys: "motion", unit: "head", layer: "skel",
      info: "面部最大的骨头，负责咀嚼运动。",
      d: '<path d="M206,88 Q206,104 218,106 Q230,108 242,106 Q254,104 254,88" fill="none"/>' },
    { id: "spine", name: "脊柱", en: "Vertebral column", sys: "motion", unit: "back", layer: "skel",
      info: "24 块椎骨+骶尾骨，支撑躯干、保护脊髓。",
      d: '<g>' + (function () { var s = ""; for (var i = 0; i < 14; i++) { var y = 100 + i * 24; var w = 15 - Math.abs(i - 7) * 0.4; s += '<rect x="' + (230 - w) + '" y="' + y + '" width="' + (w * 2) + '" height="17" rx="5"/>'; } return s; })() + "</g>" },
    { id: "ribs", name: "胸廓（肋骨）", en: "Thoracic cage", sys: "motion", unit: "chest", layer: "skel",
      info: "12 对肋骨围成胸廓，保护心肺并参与呼吸运动。",
      d: '<g fill="none">' +
        '<path d="M222,158 Q170,164 152,192 Q142,214 150,240"/>' +
        '<path d="M222,180 Q176,186 162,210 Q156,228 162,250"/>' +
        '<path d="M222,202 Q182,208 170,230 Q168,244 176,262"/>' +
        '<path d="M222,224 Q190,230 182,250 Q184,262 192,274"/>' +
        '<path d="M238,158 Q290,164 308,192 Q318,214 310,240"/>' +
        '<path d="M238,180 Q284,186 298,210 Q304,228 298,250"/>' +
        '<path d="M238,202 Q278,208 290,230 Q292,244 284,262"/>' +
        '<path d="M238,224 Q270,230 278,250 Q276,262 268,274"/>' +
        "</g>" },
    { id: "sternum", name: "胸骨", en: "Sternum", sys: "motion", unit: "chest", layer: "skel",
      info: "胸前正中骨板，肋骨经肋软骨与之相连。",
      d: '<path d="M224,150 L236,150 L236,246 L230,258 L224,246 Z"/>' },
    { id: "clavicle", name: "锁骨", en: "Clavicle", sys: "motion", unit: "arms", layer: "skel",
      info: "连接胸骨与肩胛骨，支撑肩部活动。",
      d: '<path d="M222,142 Q190,136 162,150" fill="none"/><path d="M238,142 Q270,136 298,150" fill="none"/>' },
    { id: "pelvis", name: "骨盆", en: "Pelvis", sys: "motion", unit: "hips", layer: "skel",
      info: "由髋骨+骶骨围成，承托腹腔脏器、连接躯干与下肢。",
      d: '<path d="M172,452 Q164,496 190,514 Q214,528 230,506 Q246,528 270,514 Q296,496 288,452 Q262,466 230,466 Q198,466 172,452 Z"/>' },
    { id: "femur", name: "股骨", en: "Femur", sys: "motion", unit: "legs", layer: "skel",
      info: "人体最长的骨头，即大腿骨。",
      d: '<path d="M198,516 Q190,570 192,630" fill="none"/><path d="M262,516 Q270,570 268,630" fill="none"/>' },
    { id: "humerus", name: "肱骨/尺桡骨", en: "Humerus / radius / ulna", sys: "motion", unit: "arms", layer: "skel",
      info: "上臂与前臂的骨干，配合关节完成上肢活动。",
      d: '<path d="M158,158 Q122,240 112,320 Q106,368 104,400" fill="none"/><path d="M302,158 Q338,240 348,320 Q354,368 356,400" fill="none"/>' },
    { id: "knee", name: "膝关节", en: "Knee joint", sys: "motion", unit: "legs", layer: "skel", sysOnly: true,
      info: "人体最大最复杂的关节：股骨下端+胫骨上端+髌骨，半月板缓冲震荡。",
      d: '<path d="M186,586 Q198,574 210,586 M186,586 Q178,596 186,606" fill="none"/><path d="M274,586 Q262,574 250,586 M274,586 Q282,596 274,606" fill="none"/><ellipse cx="198" cy="598" rx="14" ry="7" fill="none"/><ellipse cx="262" cy="598" rx="14" ry="7" fill="none"/>' },

    /* ===== 血管层 ===== */
    { id: "aorta", name: "主动脉", en: "Aorta", sys: "circ", unit: "heart", layer: "vess",
      info: "从心脏发出的最大动脉，把富氧血液送往全身。",
      d: '<path d="M244,206 Q252,176 230,172 Q214,170 210,186 L210,250" fill="none"/>' },
    { id: "vc", name: "下腔静脉", en: "Inferior vena cava", sys: "circ", unit: "heart", layer: "vess",
      info: "收集下半身静脉血回流心脏的大血管。",
      d: '<path d="M218,220 Q216,300 222,420" fill="none"/>' },
    { id: "pulm", name: "肺动脉/肺静脉", en: "Pulmonary vessels", sys: "resp", unit: "lungs", layer: "vess",
      info: "把缺氧血送入肺、把富氧血收回心脏的血管干。",
      d: '<path d="M236,196 Q262,190 282,198 M236,200 Q206,196 186,204" fill="none"/>' },
    { id: "arm-artery", name: "肱动脉", en: "Brachial artery", sys: "circ", unit: "arms", layer: "vess", sysOnly: true,
      info: "上臂主干动脉，肘窝处可摸到脉搏。",
      d: '<path d="M152,164 Q124,244 114,330 Q108,372 106,396" fill="none"/><path d="M308,164 Q336,244 346,330 Q352,372 354,396" fill="none"/>' },
    { id: "leg-artery", name: "股动脉", en: "Femoral artery", sys: "circ", unit: "legs", layer: "vess", sysOnly: true,
      info: "下肢主干动脉，腹股沟处可触脉搏。",
      d: '<path d="M222,508 Q216,580 214,644 M238,508 Q244,580 246,644" fill="none"/>' },

    /* ===== 器官层 ===== */
    { id: "brain", name: "大脑", en: "Cerebrum", sys: "nerve", unit: "head", lb: [168, 44],
      info: "高级神经中枢：思维、记忆、感觉与运动调控中心。",
      d: '<path d="M202,52 Q202,28 230,28 Q258,28 258,52 Q258,68 248,76 Q230,84 212,76 Q202,68 202,52 Z"/><path d="M216,36 Q212,48 218,58 M232,32 Q230,46 236,60 M246,38 Q248,50 242,60" fill="none"/>' },
    { id: "brainstem", name: "脑干", en: "Brainstem", sys: "nerve", unit: "head", sysOnly: true, lb: [180, 92],
      info: "延髓+脑桥+中脑：呼吸、心跳、血压等生命中枢所在。",
      d: '<path d="M224,74 Q222,84 224,94 M236,74 Q238,84 236,94" fill="none" style="stroke-width:5"/>' },
    { id: "nerves-lumbar", name: "脊神经（腰骶丛）", en: "Spinal nerves", sys: "nerve", unit: "back", sysOnly: true,
      info: "从椎间孔发出的神经根，支配躯干与四肢的感觉运动。",
      d: '<g fill="none" style="stroke-width:3">' +
        '<path d="M214,352 Q186,376 168,404"/><path d="M246,352 Q274,376 292,404"/>' +
        '<path d="M214,392 Q190,416 176,444"/><path d="M246,392 Q270,416 284,444"/>' +
        '<path d="M212,432 Q192,456 182,486"/><path d="M248,432 Q268,456 278,486"/>' +
        "</g>" },
    { id: "thyroid", name: "甲状腺", en: "Thyroid", sys: "endo", unit: "thyroid", lb: [318, 116],
      info: "蝴蝶形内分泌腺，分泌甲状腺素调节代谢与发育。",
      d: '<path d="M222,112 Q212,104 206,112 Q202,122 212,126 Q222,128 226,120 Z"/><path d="M238,112 Q248,104 254,112 Q258,122 248,126 Q238,128 234,120 Z"/><path d="M226,118 Q230,116 234,118" fill="none"/>' },
    { id: "pituitary", name: "垂体", en: "Pituitary gland", sys: "endo", unit: "head", sysOnly: true, lb: [180, 84],
      info: "豌豆大小悬垂于颅底的「主腺」，指挥其他内分泌腺工作。",
      d: '<ellipse cx="230" cy="84" rx="6" ry="7"/>' },
    { id: "adrenal", name: "肾上腺", en: "Adrenal glands", sys: "endo", unit: "kidneys", sysOnly: true, lb: [316, 360],
      info: "骑跨在双肾上方的三角小腺体，分泌皮质醇、肾上腺素等应激激素。",
      d: '<path d="M186,352 Q196,342 208,350 M252,352 Q264,342 274,350" fill="none" style="stroke-width:7"/>' },
    { id: "airway", name: "气管与支气管", en: "Trachea & bronchi", sys: "resp", unit: "airway", lb: [318, 138],
      info: "空气进出肺的通道，支气管逐级分支入肺。",
      d: '<path d="M225,96 L225,168 Q225,178 218,184 M225,168 Q225,178 232,184" fill="none"/><rect x="225" y="98" width="10" height="66" rx="5" fill="currentColor" opacity=".25"/>' },
    { id: "larynx", name: "喉", en: "Larynx", sys: "resp", unit: "airway", sysOnly: true, lb: [186, 104],
      info: "咽喉部的发声器官，吞咽时会厌盖住气道防呛咳。",
      d: '<rect x="221" y="96" width="18" height="16" rx="6"/>' },
    { id: "nasal", name: "鼻腔与鼻窦", en: "Nasal cavity", sys: "resp", unit: "head", sysOnly: true, lb: [186, 60],
      info: "吸入空气的第一站：加温、加湿并过滤灰尘。",
      d: '<path d="M224,52 Q230,46 236,52 L234,74 Q230,78 226,74 Z"/>' },
    { id: "alveoli", name: "肺泡", en: "Alveoli", sys: "resp", unit: "lungs", sysOnly: true, lb: [128, 268],
      info: "支气管末端的葡萄串状气囊，气体交换就发生在薄薄的泡壁上（成人约 3 亿个）。",
      d: '<g>' +
        '<circle cx="192" cy="268" r="5"/><circle cx="202" cy="274" r="5"/><circle cx="196" cy="281" r="5"/><circle cx="207" cy="266" r="4"/>' +
        '<circle cx="268" cy="268" r="5"/><circle cx="258" cy="274" r="5"/><circle cx="264" cy="281" r="5"/><circle cx="253" cy="266" r="4"/>' +
        "</g>" },
    { id: "lungR", name: "右肺", en: "Right lung", sys: "resp", unit: "lungs", lb: [128, 176],
      info: "三叶。气体交换的主要场所，随呼吸张缩。",
      d: '<path d="M218,152 Q176,152 152,196 Q136,226 142,268 Q146,292 170,292 Q204,290 214,262 Q222,236 222,200 Q222,168 218,152 Z"/><path d="M214,220 Q190,236 168,262" fill="none"/>' },
    { id: "lungL", name: "左肺", en: "Left lung", sys: "resp", unit: "lungs", lb: [336, 190],
      info: "两叶。心切迹让位给心脏，故比右肺略小。",
      d: '<path d="M242,152 Q284,152 308,196 Q324,226 318,268 Q314,292 290,292 Q262,290 250,266 Q242,244 240,208 Q239,172 242,152 Z"/><path d="M248,228 Q274,240 296,264" fill="none"/>' },
    { id: "heart", name: "心脏", en: "Heart", sys: "circ", unit: "heart", lb: [300, 236],
      info: "四腔肌性泵：左右心房+左右心室，昼夜不停地推动血液循环。",
      d: '<path d="M232,204 Q224,196 214,200 Q198,206 200,226 Q202,250 224,268 Q232,274 240,268 Q258,252 262,230 Q264,208 248,202 Q238,198 232,204 Z"/><path d="M212,224 Q230,232 250,226 M226,208 L226,266" fill="none"/>' },
    { id: "diaphragm", name: "膈肌", en: "Diaphragm", sys: "resp", unit: "chest",
      info: "分隔胸腹腔的穹顶状呼吸肌，收缩下沉完成吸气。",
      d: '<path d="M148,290 Q230,252 312,290" fill="none"/>' },
    { id: "esophagus", name: "食道", en: "Esophagus", sys: "digest", unit: "stomach", sysOnly: true, lb: [186, 200],
      info: "长约 25cm 的肌性管道，把食物团推送入胃。",
      d: '<path d="M238,100 Q242,180 250,292" fill="none" style="stroke-width:9"/>' },
    { id: "liver", name: "肝脏", en: "Liver", sys: "digest", unit: "liver", lb: [126, 302],
      info: "最大的内脏器官：代谢、解毒、合成蛋白、分泌胆汁。",
      d: '<path d="M152,296 Q222,278 292,300 Q300,306 292,322 Q258,346 206,342 Q166,336 152,316 Q146,304 152,296 Z"/><path d="M222,290 Q226,308 220,330" fill="none"/>' },
    { id: "gall", name: "胆囊", en: "Gallbladder", sys: "digest", unit: "gallbladder", lb: [148, 342],
      info: "储存并浓缩肝脏分泌的胆汁，进食时排入肠道助消化。",
      d: '<path d="M232,326 Q244,330 244,342 Q242,352 234,350 Q228,346 230,336 Z"/>' },
    { id: "stomach", name: "胃", en: "Stomach", sys: "digest", unit: "stomach", lb: [330, 316],
      info: "J 形肌性囊袋：储存食物、分泌胃酸初步消化。",
      d: '<path d="M258,298 Q296,290 310,312 Q320,334 300,352 Q276,368 258,352 Q248,340 254,324 Q258,308 258,298 Z"/><path d="M260,306 Q252,330 266,346" fill="none"/>' },
    { id: "spleen", name: "脾脏", en: "Spleen", sys: "immune", unit: "spleen", lb: [348, 344],
      info: "最大的淋巴器官：过滤血液、清除衰老红细胞、储存免疫细胞。",
      d: '<path d="M314,314 Q336,312 340,330 Q342,348 326,354 Q310,356 306,340 Q304,322 314,314 Z"/>' },
    { id: "pancreas", name: "胰腺", en: "Pancreas", sys: "digest", unit: "pancreas", lb: [152, 372],
      info: "分泌消化酶与胰岛素、胰高血糖素，调控血糖。",
      d: '<path d="M238,360 Q270,352 300,362 Q306,366 300,372 Q268,366 242,372 Q232,368 238,360 Z"/>' },
    { id: "kidneyR", name: "右肾", en: "Right kidney", sys: "uri", unit: "kidneys", lb: [130, 388],
      info: "成对存在的滤血器官：每天滤过约 180L 血液生成原尿。",
      d: '<path d="M196,362 Q178,362 174,382 Q172,402 190,408 Q202,410 206,396 Q208,372 196,362 Z"/>' },
    { id: "kidneyL", name: "左肾", en: "Left kidney", sys: "uri", unit: "kidneys", lb: [334, 388],
      info: "形似蚕豆，右肾因肝脏略低。调节水电解质与血压。",
      d: '<path d="M264,362 Q282,362 286,382 Q288,402 270,408 Q258,410 254,396 Q252,372 264,362 Z"/>' },
    { id: "ureter", name: "输尿管", en: "Ureters", sys: "uri", unit: "kidneys", layer: "vess",
      info: "把尿液从肾盂输送到膀胱的细长管道。",
      d: '<path d="M196,408 Q202,446 222,462 M264,408 Q258,446 238,462" fill="none"/>' },
    { id: "colon", name: "大肠（结肠）", en: "Large intestine", sys: "digest", unit: "colon", lb: [128, 428],
      info: "框形走行：升结肠→横结肠→降结肠→乙状结肠，吸收水分形成粪便。",
      d: '<path d="M162,352 Q150,356 150,380 L150,430 Q150,452 172,460 Q200,470 230,468 Q262,470 288,460 Q310,452 310,430 L310,380 Q310,356 298,352" fill="none" style="stroke-width:12"/>' },
    { id: "appendix", name: "阑尾", en: "Appendix", sys: "digest", unit: "appendix", lb: [116, 466],
      info: "盲肠末端的细长突起，发炎即阑尾炎，典型表现为右下腹痛。",
      d: '<path d="M152,432 Q148,456 160,466" fill="none" style="stroke-width:7"/>' },
    { id: "smallint", name: "小肠", en: "Small intestine", sys: "digest", unit: "intestine", lb: [332, 432],
      info: "长约 5-7 米的盘曲管道：营养吸收的主战场。",
      d: '<g fill="none" style="stroke-width:9">' +
        '<path d="M186,382 Q230,372 274,382 Q286,388 276,398 Q234,390 194,400 Q182,406 192,414 Q234,406 270,416 Q280,422 270,430 Q230,422 192,432 Q182,438 192,446 Q230,438 268,448"/>' +
        "</g>" },
    { id: "bladder", name: "膀胱", en: "Urinary bladder", sys: "uri", unit: "bladder", lb: [318, 482],
      info: "肌性储尿囊，容量约 400-500ml，充盈产生尿意。",
      d: '<path d="M210,462 Q210,486 230,488 Q250,486 250,462 Q240,456 230,458 Q220,456 210,462 Z"/>' },
    { id: "prostate", name: "前列腺（男性）", en: "Prostate", sys: "uri", unit: "bladder",
      info: "包绕尿道起始部，分泌前列腺液参与精液构成。",
      d: '<path d="M220,488 Q230,494 240,488 Q240,500 230,502 Q220,500 220,488 Z"/>' },
    { id: "lymph", name: "淋巴结群", en: "Lymph nodes", sys: "immune", unit: "neck", sysOnly: true, lb: [186, 132],
      info: "免疫「哨站」：颈、腋、腹股沟浅表成群，肿大常提示附近有感染。",
      d: '<g>' +
        '<circle cx="204" cy="108" r="5"/><circle cx="212" cy="122" r="4"/><circle cx="256" cy="108" r="5"/><circle cx="248" cy="122" r="4"/>' +
        '<circle cx="148" cy="184" r="6"/><circle cx="312" cy="184" r="6"/>' +
        '<circle cx="184" cy="470" r="5"/><circle cx="276" cy="470" r="5"/>' +
        "</g>" }
  ];

  var LAYER_NAMES = { skel: "骨骼", org: "器官", vess: "血管" };

  /* ---------- 基础工具 ---------- */
  function hitSet(hits) {
    var on = {};
    (hits || "").split(",").forEach(function (h) {
      h = h.trim();
      if (h) on[h] = 1;
    });
    return on;
  }
  function partHit(p, on) {
    return !!(on[p.unit] || on[p.id]);
  }
  function findPart(id) {
    for (var i = 0; i < PARTS.length; i++) if (PARTS[i].id === id) return PARTS[i];
    return null;
  }
  function partsOfSystem(sys) {
    return PARTS.filter(function (p) { return p.sys === sys; });
  }

  /* ---------- SVG 组装 ----------
   * opts: { labels:bool, system:sysId|null, hits:"a,b", noDim:bool, showSysOnly:bool }
   * system 存在时：非该系统部件淡出（dim），该系统 sysOnly 细节显示。
   */
  function buildSVG(hits, opts) {
    opts = opts || {};
    var on = hitSet(hits);
    var labels = opts.labels !== false;
    var system = opts.system || null;
    var dim = !!system && !opts.noDim;
    var s = '<svg class="anat-svg" viewBox="0 0 460 660" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="人体解剖示意图">';
    s += '<g class="anat-body">' +
      '<ellipse cx="230" cy="58" rx="36" ry="40"/>' +
      '<path d="M214,92 L214,118 Q212,134 192,140 M246,92 L246,118 Q248,134 268,140"/>' +
      '<path d="M118,172 Q230,138 342,172 Q356,220 332,270 Q324,308 338,340 L342,456 Q338,486 310,492 L280,496 Q262,500 258,516 M150,492 Q148,472 152,456 L158,340 Q136,308 128,270 Q104,220 118,172 Z"/>' +
      '<path d="M128,186 Q94,272 88,364 Q84,430 92,470 L104,466 Q98,420 104,368 Q112,280 144,196"/>' +
      '<path d="M332,186 Q366,272 372,364 Q376,430 368,470 L356,466 Q362,420 356,368 Q348,280 316,196"/>' +
      '<path d="M196,520 Q186,600 188,654 M264,520 Q274,600 272,654"/>' +
      "</g>";
    ["skel", "vess", "org"].forEach(function (layer) {
      s += '<g class="anat-layer anat-' + layer + '">';
      PARTS.forEach(function (p) {
        var pl = p.layer || "org";
        if (pl !== layer) return;
        var showDetail = system && p.sys === system;
        if (p.sysOnly && !showDetail) return;
        var hit = partHit(p, on);
        var dimCls = dim && p.sys !== system ? " is-dim" : "";
        s += '<g class="anat-part sys-' + p.sys + dimCls + (hit ? " is-hit" : "") + '" data-part="' + p.id + '" tabindex="0" role="button" aria-label="' + p.name + '">' +
          "<title>" + p.name + " " + p.en + "</title>" + p.d + "</g>";
      });
      s += "</g>";
    });
    if (labels) {
      s += '<g class="anat-labels">';
      PARTS.forEach(function (p) {
        if (p.sysOnly && !(system && p.sys === system)) return;
        if (!p.lb) return;
        var hit = partHit(p, on);
        var ax = p.lb[0] < 230 ? p.lb[0] + 4 : p.lb[0] - 4;
        s += '<g class="anat-lb' + (hit ? " is-hit" : "") + '" data-for="' + p.id + '">' +
          '<line x1="' + p.lb[0] + '" y1="' + p.lb[1] + '" x2="' + (p.lb[0] < 230 ? p.lb[0] - 26 : p.lb[0] + 26) + '" y2="' + p.lb[1] + '"/>' +
          '<text x="' + (ax + (p.lb[0] < 230 ? -28 : 28)) + '" y="' + (p.lb[1] + 4) + '" text-anchor="' + (p.lb[0] < 230 ? "end" : "start") + '">' + p.name + "</text></g>";
      });
      s += "</g>";
    }
    s += "</svg>";
    return s;
  }

  /* ---------- 详情卡 ---------- */
  function infoHTML(p) {
    var sys = SYSTEMS[p.sys] || {};
    return '<div class="anat-info"><div class="anat-info-head"><b>' + p.name + "</b><i>" + p.en + "</i></div>" +
      '<div class="anat-info-sys"><span class="anat-dot" style="background:' + (sys.color || "#999") + '"></span>' + (sys.name || "") + "</div>" +
      '<div class="anat-info-desc">' + p.info + "</div></div>";
  }
  function bindClicks(root, showInfo) {
    root.addEventListener("click", function (e) {
      var g = e.target.closest ? e.target.closest(".anat-part") : null;
      if (g) {
        var p = findPart(g.getAttribute("data-part"));
        if (p && showInfo) showInfo(p);
      }
    });
  }

  /* ---------- 缩放/平移 ---------- */
  var BASE_VB = { x: 0, y: 0, w: 460, h: 660 };
  function parseVB(str) {
    var a = (str || "0,0,460,660").split(",").map(parseFloat);
    return { x: a[0], y: a[1], w: a[2], h: a[3] };
  }
  function makeZoomable(svg, onPick) {
    var vb = { x: BASE_VB.x, y: BASE_VB.y, w: BASE_VB.w, h: BASE_VB.h };
    var minW = 60;
    function apply() { svg.setAttribute("viewBox", vb.x + " " + vb.y + " " + vb.w + " " + vb.h); }
    function zoomAt(px, py, factor) {
      var nw = Math.min(BASE_VB.w * 1.6, Math.max(minW, vb.w * factor));
      var k = nw / vb.w;
      vb.x = px - (px - vb.x) * k;
      vb.y = py - (py - vb.y) * k;
      vb.w = nw; vb.h = BASE_VB.h * (nw / BASE_VB.w);
      apply();
    }
    svg.addEventListener("wheel", function (e) {
      e.preventDefault();
      var r = svg.getBoundingClientRect();
      var px = vb.x + (e.clientX - r.left) / r.width * vb.w;
      var py = vb.y + (e.clientY - r.top) / r.height * vb.h;
      zoomAt(px, py, e.deltaY > 0 ? 1.16 : 0.86);
    }, { passive: false });
    var drag = null;
    svg.addEventListener("pointerdown", function (e) {
      drag = { x: e.clientX, y: e.clientY, vx: vb.x, vy: vb.y, moved: false };
      svg.setPointerCapture(e.pointerId);
    });
    svg.addEventListener("pointermove", function (e) {
      if (!drag) return;
      var r = svg.getBoundingClientRect();
      var dx = (e.clientX - drag.x) / r.width * vb.w;
      var dy = (e.clientY - drag.y) / r.height * vb.h;
      if (Math.abs(dx) + Math.abs(dy) > 2) drag.moved = true;
      vb.x = drag.vx - dx; vb.y = drag.vy - dy;
      apply();
    });
    svg.addEventListener("pointerup", function () { drag = null; });
    svg.addEventListener("dblclick", function (e) {
      e.preventDefault();
      var r = svg.getBoundingClientRect();
      zoomAt(vb.x + (e.clientX - r.left) / r.width * vb.w, vb.y + (e.clientY - r.top) / r.height * vb.h, 0.6);
    });
    function setVB(next) { vb = { x: next.x, y: next.y, w: next.w, h: next.h }; apply(); }
    function getVB() { return { x: vb.x, y: vb.y, w: vb.w, h: vb.h }; }
    svg.__setVB = setVB;
    svg.__getVB = getVB;
    svg.__reset = function () { setVB(BASE_VB); };
    /* 平滑聚焦到某部件 */
    svg.__focusPart = function (partId) {
      var g = svg.querySelector('[data-part="' + partId + '"]');
      if (!g) return false;
      var bb;
      try { bb = g.getBBox(); } catch (e) { return false; }
      var pad = Math.max(bb.width, bb.height) * 1.4 + 24;
      var cx = bb.x + bb.width / 2, cy = bb.y + bb.height / 2;
      var w = Math.min(BASE_VB.w, Math.max(110, pad * 2.6));
      var h = w * BASE_VB.h / BASE_VB.w;
      /* 分 8 步动画插值 */
      var from = getVB(), to = { x: cx - w / 2, y: cy - h / 2, w: w, h: h };
      var t0 = null;
      function step(ts) {
        if (!t0) t0 = ts;
        var k = Math.min(1, (ts - t0) / 420);
        k = 1 - Math.pow(1 - k, 3);
        setVB({
          x: from.x + (to.x - from.x) * k,
          y: from.y + (to.y - from.y) * k,
          w: from.w + (to.w - from.w) * k,
          h: from.h + (to.h - from.h) * k
        });
        if (k < 1) requestAnimationFrame(step);
      }
      requestAnimationFrame(step);
      return true;
    };
    svg.addEventListener("click", function (e) {
      if (drag && drag.moved) return;
      var g = e.target.closest ? e.target.closest(".anat-part") : null;
      if (g && onPick) onPick(findPart(g.getAttribute("data-part")));
    });
    return svg;
  }

  /* ---------- 内嵌小图 ---------- */
  function inline(hits, opts) {
    opts = opts || {};
    var wrap = document.createElement("div");
    wrap.className = "anat-inline";
    wrap.innerHTML = buildSVG(hits, { labels: false });
    var svg = wrap.querySelector("svg");
    var cap = document.createElement("div");
    cap.className = "anat-cap";
    cap.innerHTML = '解剖示意图（<b class="anat-zoom-hint">🔍 点击放大</b> · 分级查看系统与器官 · 可缩放拖动）';
    wrap.appendChild(cap);
    var info = document.createElement("div");
    info.className = "anat-info-slot";
    wrap.appendChild(info);
    bindClicks(wrap, function (p) { info.innerHTML = infoHTML(p); });
    cap.querySelector(".anat-zoom-hint").addEventListener("click", function (e) {
      e.stopPropagation();
      openModal(hits);
    });
    svg.addEventListener("dblclick", function () { openModal(hits); });
    svg.style.cursor = "zoom-in";
    if (opts.onReady) opts.onReady(wrap);
    return wrap;
  }

  /* ---------- 全屏弹窗 + 三级导航 ---------- */
  var modalEl = null;
  var nav = { view: "whole", sys: null }; /* view: whole | sys */

  function ensureModal() {
    if (modalEl) return modalEl;
    modalEl = document.createElement("div");
    modalEl.className = "anat-modal";
    modalEl.innerHTML =
      '<div class="anat-modal-panel">' +
      '<div class="anat-modal-head"><nav class="anat-crumbs" aria-label="解剖层级"></nav>' +
      '<span class="anat-toggles">' +
      '<label><input type="checkbox" data-layer="skel" checked>骨骼</label>' +
      '<label><input type="checkbox" data-layer="org" checked>器官</label>' +
      '<label><input type="checkbox" data-layer="vess" checked>血管</label>' +
      '<label><input type="checkbox" data-lbl checked>标注</label>' +
      "</span>" +
      '<button type="button" class="anat-reset" title="双击图面也可复位">复位</button>' +
      '<button type="button" class="anat-close">✕</button></div>' +
      '<div class="anat-anim-chips" hidden></div>' +
      '<div class="anat-modal-body"></div>' +
      '<div class="anat-modal-foot"><div class="anat-info-slot"><div class="anat-info-hint">滚轮缩放 · 拖动平移 · 双击放大 · 点击器官逐级深入</div></div></div>' +
      "</div>";
    document.body.appendChild(modalEl);
    modalEl.querySelector(".anat-close").addEventListener("click", closeModal);
    modalEl.querySelector(".anat-reset").addEventListener("click", function () {
      var svg = modalEl.querySelector(".anat-modal-body .anat-svg");
      if (svg && svg.__reset) svg.__reset();
    });
    modalEl.addEventListener("click", function (e) { if (e.target === modalEl) closeModal(); });
    document.addEventListener("keydown", function (e) { if (e.key === "Escape") closeModal(); });
    /* 图层开关 */
    modalEl.querySelectorAll(".anat-toggles input").forEach(function (cb) {
      cb.addEventListener("change", function () {
        var body = modalEl.querySelector(".anat-modal-body");
        if (cb.dataset.layer) {
          var g = body.querySelector(".anat-" + cb.dataset.layer);
          if (g) g.style.display = cb.checked ? "" : "none";
        } else {
          var lb = body.querySelector(".anat-labels");
          if (lb) lb.style.display = cb.checked ? "" : "none";
        }
      });
    });
    /* 面包屑点击 */
    modalEl.querySelector(".anat-crumbs").addEventListener("click", function (e) {
      var b = e.target.closest ? e.target.closest("[data-nav]") : null;
      if (!b) return;
      var v = b.getAttribute("data-nav");
      if (v === "whole") renderModal();
      else renderModal(v);
    });
    return modalEl;
  }

  function crumbsHTML() {
    var h = '<button type="button" data-nav="whole"' + (nav.view === "whole" ? ' class="cur"' : "") + ">全身</button>";
    if (nav.view !== "whole") {
      var sys = SYSTEMS[nav.view] || {};
      h += ' <span class="sep">›</span> <button type="button" data-nav="' + nav.view + '" class="cur">' + sys.name + "</button>";
    }
    return h;
  }

  function animChipsHTML() {
    if (!window.AnatomyAnim) return "";
    var list = nav.view === "whole"
      ? window.AnatomyAnim.all()
      : window.AnatomyAnim.forSystem(nav.view);
    if (!list.length) return "";
    return list.map(function (a) {
      return '<button type="button" class="anat-anim-chip" data-anim="' + a.id + '">' +
        (a.kind === "patho" ? "🩺 " : "▶️ ") + a.name + "</button>";
    }).join("");
  }

  function renderModal(sysId) {
    var m = ensureModal();
    if (window.AnatomyAnim) window.AnatomyAnim.stop();
    nav.view = sysId || "whole";
    nav.sys = nav.view === "whole" ? null : nav.view;
    var body = m.querySelector(".anat-modal-body");
    var foot = m.querySelector(".anat-modal-foot .anat-info-slot");
    var region = nav.view === "whole" ? null : SYSTEM_REGIONS[nav.view];
    body.innerHTML = buildSVG(pendingHits, { labels: true, system: nav.view === "whole" ? null : nav.view });
    var svg = body.querySelector("svg");
    if (region && svg.__setVB) svg.__setVB(parseVB(region));
    makeZoomable(svg, function (p) {
      foot.innerHTML = infoHTML(p);
      /* 器官聚焦：全身视图 → 跳系统视图；系统视图 → 原地放大 */
      if (nav.view === "whole") {
        renderModal(p.sys);
        var svg2 = m.querySelector(".anat-modal-body .anat-svg");
        var slot = m.querySelector(".anat-modal-foot .anat-info-slot");
        slot.innerHTML = infoHTML(p);
        if (svg2 && svg2.__focusPart) svg2.__focusPart(p.id);
      } else if (svg.__focusPart) {
        svg.__focusPart(p.id);
      }
    });
    foot.innerHTML = '<div class="anat-info-hint">' +
      (nav.view === "whole"
        ? "点击器官进入对应系统视图并放大 · 滚轮缩放 · 拖动平移"
        : (SYSTEMS[nav.view] ? SYSTEMS[nav.view].desc : "") + " · 点击器官继续放大") +
      "</div>";
    m.querySelector(".anat-crumbs").innerHTML = crumbsHTML();
    var chips = m.querySelector(".anat-anim-chips");
    var chipsHTML = animChipsHTML();
    chips.innerHTML = chipsHTML;
    chips.hidden = !chipsHTML;
    if (window.AnatomyAnim) window.AnatomyAnim.bindChips(chips, svg, foot, nav.view);
    /* 重置图层开关为全开 */
    m.querySelectorAll(".anat-toggles input").forEach(function (cb) { cb.checked = true; });
  }

  function openModal(hits, opts) {
    var m = ensureModal();
    pendingHits = hits || "";
    renderModal(opts && opts.system);
    m.classList.add("open");
    document.body.style.overflow = "hidden";
  }
  var pendingHits = "";
  function closeModal() {
    if (modalEl) {
      if (window.AnatomyAnim) window.AnatomyAnim.stop();
      modalEl.classList.remove("open");
      document.body.style.overflow = "";
    }
  }

  window.AnatomyView = {
    inline: inline,
    openModal: function (hits, opts) { openModal(hits, opts); },
    SYSTEMS: SYSTEMS,
    SYSTEM_ORDER: SYSTEM_ORDER,
    SYSTEM_REGIONS: SYSTEM_REGIONS,
    PARTS: PARTS,
    buildSVG: buildSVG,
    findPart: findPart,
    parseVB: parseVB,
    BASE_VB: BASE_VB
  };
})();
