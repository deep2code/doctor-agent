/* 医答 · 解剖图动画播放器（依赖 anatomy.js 的 AnatomyView）
 * 生理动画（循环播放）：心跳血循环 / 呼吸运动 / 消化蠕动 / 神经信号传导
 * 病理动画（分步讲解）：动脉粥样硬化 / 肺炎 / 急性阑尾炎 / 肾结石梗阻
 * 生理动画用 CSS keyframes + SMIL 循环；病理动画由 rAF 时间轴分步推进，
 * 每步切换 caption 并给场景加 stage 类，CSS transition 做平滑过渡。
 */
(function () {
  "use strict";
  if (!window.AnatomyView) return;

  /* ---------- 样式注入（一次性） ---------- */
  var CSS = [
    ".anat-svg .anat-anim-overlay path.flow { fill:none; stroke-linecap:round; stroke-dasharray:7 9; }",
    ".anat-svg .anat-anim-overlay path.flow-red { stroke:#dc2626; }",
    ".anat-svg .anat-anim-overlay path.flow-blue { stroke:#3b82f6; }",
    ".anat-svg .anat-anim-overlay path.flow-air { stroke:#7dd3fc; stroke-dasharray:4 10; }",
    ".anat-svg .anat-anim-overlay .flow-anim { animation: anatFlow 1.1s linear infinite; }",
    ".anat-svg .anat-anim-overlay .flow-slow { animation-duration: 3.2s; }",
    "@keyframes anatFlow { to { stroke-dashoffset: -32; } }",
    ".anat-svg .anat-anim-overlay .flow.rev { animation-direction: reverse; }",

    /* 生理：心跳泵血 */
    ".anat-play-heartbeat .anat-part[data-part='heart'] { transform-box: fill-box; transform-origin:50% 50%; animation: anatPump 1s ease-in-out infinite; }",
    "@keyframes anatPump { 0%{transform:scale(1)} 18%{transform:scale(1.14)} 36%{transform:scale(1)} 55%{transform:scale(1.07)} 100%{transform:scale(1)} }",

    /* 生理：呼吸 */
    ".anat-play-breathing .anat-part[data-part='lungR'], .anat-play-breathing .anat-part[data-part='lungL'] { transform-box: fill-box; transform-origin:50% 30%; animation: anatLung 4.6s ease-in-out infinite; }",
    "@keyframes anatLung { 0%,100%{transform:scale(1)} 45%{transform:scale(1.09,1.13)} }",
    ".anat-play-breathing .anat-part[data-part='diaphragm'] { animation: anatDiaph 4.6s ease-in-out infinite; }",
    "@keyframes anatDiaph { 0%,100%{transform:translateY(0)} 45%{transform:translateY(14px)} }",
    ".anat-play-breathing .air-in { animation: anatAirIn 4.6s ease-in-out infinite; }",
    ".anat-play-breathing .air-out { animation: anatAirOut 4.6s ease-in-out infinite; }",
    "@keyframes anatAirIn { 0%{opacity:0} 12%{opacity:1} 40%{opacity:1} 50%{opacity:0} 100%{opacity:0} }",
    "@keyframes anatAirOut { 0%{opacity:0} 50%{opacity:0} 60%{opacity:1} 90%{opacity:1} 100%{opacity:0} }",

    /* 生理：神经传导 */
    ".anat-play-nervesis .sig { stroke:#8b5cf6; stroke-width:4; fill:none; stroke-linecap:round; stroke-dasharray:4 14; animation: anatNerve 1s linear infinite; }",
    "@keyframes anatNerve { to { stroke-dashoffset: -36; } }",

    /* 暂停总开关（生理动画） */
    ".anat-svg.anat-paused * { animation-play-state: paused !important; }",
    ".anat-svg.anat-paused { cursor: default; }",

    /* ---------- 病理：动脉粥样硬化 ---------- */
    ".anat-play-atheroscl .plaque { transform-box: fill-box; transform-origin:20% 50%; transform:scale(.15); opacity:0; transition: transform 1.4s ease, opacity 1s; }",
    ".anat-play-atheroscl.st2 .plaque { transform:scale(.55); opacity:1; }",
    ".anat-play-atheroscl.st3 .plaque { transform:scale(1); }",
    ".anat-play-atheroscl.st3 .vessel-flow { animation-duration: 3.4s; }",
    ".anat-play-atheroscl.st4 .clot { opacity:1; transform:scale(1); }",
    ".anat-play-atheroscl .clot { opacity:0; transform:scale(.3); transform-box:fill-box; transform-origin:60% 50%; transition: all 1.2s ease; }",
    ".anat-play-atheroscl.st4 .heart-dim { opacity:.45; transition: opacity 1.4s; }",

    /* ---------- 病理：肺炎 ---------- */
    ".anat-play-pneumonia .germ { opacity:0; transition: opacity 1s; }",
    ".anat-play-pneumonia.st2 .germ { opacity:1; }",
    ".anat-play-pneumonia .exudate { opacity:0; transition: opacity 1.4s; }",
    ".anat-play-pneumonia.st3 .exudate { opacity:.55; }",
    ".anat-play-pneumonia.st4 .exudate { opacity:.85; }",
    ".anat-play-pneumonia.st4 .anat-part[data-part='lungR'] { opacity:.6; }",
    ".anat-play-pneumonia.st3 .anat-part[data-part='lungR'], .anat-play-pneumonia.st4 .anat-part[data-part='lungR'] { animation: none; transform:scale(1) !important; }",

    /* ---------- 病理：阑尾炎 ---------- */
    ".anat-play-append .anat-part[data-part='appendix'] path { transition: all 1.2s ease; }",
    ".anat-play-append.st2 .anat-part[data-part='appendix'] path { stroke:#ef4444; stroke-width:12; }",
    ".anat-play-append.st3 .anat-part[data-part='appendix'] path { stroke:#dc2626; stroke-width:15; }",
    ".anat-play-append .pus { opacity:0; transition: opacity 1s; }",
    ".anat-play-append.st3 .pus { opacity:1; }",
    ".anat-play-append.st4 .pus { animation: anatBlink .7s ease-in-out infinite; }",
    "@keyframes anatBlink { 50% { opacity:.25; } }",

    /* ---------- 病理：肾结石 ---------- */
    ".anat-play-stone .stone { transition: transform 1.6s ease; transform-box: fill-box; }",
    ".anat-play-stone.st2 .stone { transform: translate(14px, 60px); }",
    ".anat-play-stone.st3 .stone { transform: translate(18px, 96px); }",
    ".anat-play-stone .hydro { opacity:0; transition: opacity 1.4s; transform-box:fill-box; transform-origin:50% 50%; transform:scale(.8); }",
    ".anat-play-stone.st3 .hydro { opacity:.8; transform:scale(1.15); }",
    ".anat-play-stone.st4 .pain { opacity:1; }",
    ".anat-play-stone .pain { opacity:0; transition: opacity 1s; }",

    /* ---------- 生理：免疫应答 ---------- */
    ".anat-play-immune .germ-i { animation: anatGermWiggle 1.2s ease-in-out infinite; }",
    "@keyframes anatGermWiggle { 50% { transform: translate(4px,-3px); } }",
    ".anat-play-immune .wbc-a { animation: anatWbcA 5s ease-in-out infinite; }",
    ".anat-play-immune .wbc-b { animation: anatWbcB 5s ease-in-out infinite; }",
    "@keyframes anatWbcA { 0%{transform:translate(0,0);opacity:1} 55%{transform:translate(34px,26px);opacity:1} 70%{opacity:0} 100%{transform:translate(34px,26px);opacity:0} }",
    "@keyframes anatWbcB { 0%{transform:translate(0,0);opacity:1} 55%{transform:translate(-30px,30px);opacity:1} 70%{opacity:0} 100%{transform:translate(-30px,30px);opacity:0} }",
    ".anat-play-immune .germ-die { animation: anatGermDie 5s linear infinite; }",
    "@keyframes anatGermDie { 0%,50%{transform:scale(1);opacity:1} 65%{transform:scale(.6);opacity:.5} 80%,100%{transform:scale(0);opacity:0} }",
    ".anat-play-immune .node { transform-box: fill-box; transform-origin:50% 50%; animation: anatNode 5s ease-in-out infinite; }",
    "@keyframes anatNode { 0%,55%{transform:scale(1)} 80%{transform:scale(1.5)} 100%{transform:scale(1)} }",

    /* ---------- 生理：血糖调节 ---------- */
    ".anat-play-insulin .glu { animation: anatGlu 4.5s linear infinite; }",
    ".anat-play-insulin .glu.d2 { animation-delay:.4s } .anat-play-insulin .glu.d3 { animation-delay:.8s } .anat-play-insulin .glu.d4 { animation-delay:1.2s } .anat-play-insulin .glu.d5 { animation-delay:1.6s } .anat-play-insulin .glu.d6 { animation-delay:2s }",
    "@keyframes anatGlu { 0%{opacity:0} 15%{opacity:1} 60%{transform:translate(30px,42px);opacity:1} 85%,100%{transform:translate(44px,64px);opacity:0} }",
    ".anat-play-insulin .ins-sig { transform-box: fill-box; transform-origin:50% 50%; animation: anatIns 4.5s ease-in-out infinite; }",
    "@keyframes anatIns { 0%,10%{transform:scale(1);opacity:.7} 25%{transform:scale(1.5);opacity:1} 60%,100%{transform:scale(1);opacity:.7} }",
    ".anat-play-insulin .cell-door { animation: anatCellDoor 4.5s ease-in-out infinite; }",
    "@keyframes anatCellDoor { 0%,40%{opacity:.3} 60%{opacity:1} 90%,100%{opacity:.3} }",

    /* ---------- 生理：尿液生成 ---------- */
    ".anat-play-urine .urine-drop { animation: anatUrineDrop 6s linear infinite; }",
    ".anat-play-urine .urine-drop.d2 { animation-delay:1.2s } .anat-play-urine .urine-drop.d3 { animation-delay:2.4s }",
    "@keyframes anatUrineDrop { 0%{opacity:0} 8%{opacity:1} 60%{opacity:1} 75%,100%{opacity:0} }",
    ".anat-play-urine .anat-part[data-part='bladder'] { transform-box:fill-box; transform-origin:50% 50%; animation: anatBladderFill 12s ease-in-out infinite; }",
    "@keyframes anatBladderFill { 0%{transform:scale(.92)} 45%{transform:scale(1.08)} 70%{transform:scale(1.08)} 85%{transform:scale(.92)} 100%{transform:scale(.92)} }",

    /* ---------- 生理：肺泡气体交换 ---------- */
    ".anat-play-gas .o2 { animation: anatO2 3.2s linear infinite; }",
    ".anat-play-gas .o2.d2 { animation-delay:1s } .anat-play-gas .o2.d3 { animation-delay:2s }",
    "@keyframes anatO2 { 0%{transform:translate(0,-6px);opacity:0} 25%{opacity:1} 75%{transform:translate(6px,26px);opacity:1} 90%,100%{transform:translate(8px,36px);opacity:0} }",
    ".anat-play-gas .co2 { animation: anatCO2 3.2s linear infinite; }",
    ".anat-play-gas .co2.d2 { animation-delay:1.4s }",
    "@keyframes anatCO2 { 0%{transform:translate(0,10px);opacity:0} 25%{opacity:1} 75%{transform:translate(-8px,-22px);opacity:1} 90%,100%{transform:translate(-10px,-34px);opacity:0} }",

    /* ---------- 生理：反射弧 ---------- */
    ".anat-play-reflex .tap { animation: anatTap 3s ease-in-out infinite; }",
    "@keyframes anatTap { 0%,8%{opacity:0} 12%{opacity:1} 22%{opacity:0} 100%{opacity:0} }",
    ".anat-play-reflex .seg-up { animation: anatSegUp 3s linear infinite; }",
    "@keyframes anatSegUp { 0%,14%{stroke-dashoffset:60} 45%{stroke-dashoffset:0} 100%{stroke-dashoffset:0} }",
    ".anat-play-reflex .seg-down { animation: anatSegDown 3s linear infinite; }",
    "@keyframes anatSegDown { 0%,45%{stroke-dashoffset:60} 75%{stroke-dashoffset:0} 100%{stroke-dashoffset:0} }",
    ".anat-play-reflex .kick { animation: anatKick 3s ease-in-out infinite; }",
    "@keyframes anatKick { 0%,70%{opacity:0} 80%{opacity:1} 90%,100%{opacity:0} }",

    /* ---------- 病理：胃食管反流 ---------- */
    ".anat-play-gerd .acid { transition: transform 1.6s ease; }",
    ".anat-play-gerd.st2 .acid { transform: translateY(-34px); }",
    ".anat-play-gerd.st3 .acid { transform: translateY(-120px); }",
    ".anat-play-gerd.st4 .acid { transform: translateY(-150px); }",
    ".anat-play-gerd .injure { opacity:0; transition: opacity 1.2s; }",
    ".anat-play-gerd.st4 .injure { opacity:1; }",

    /* ---------- 病理：哮喘 ---------- */
    ".anat-play-asthma .lumen { transition: transform 1.4s ease; transform-box:fill-box; transform-origin:50% 50%; }",
    ".anat-play-asthma.st2 .lumen { transform:scale(.72); }",
    ".anat-play-asthma.st3 .lumen { transform:scale(.5); }",
    ".anat-play-asthma.st4 .lumen { transform:scale(.3); }",
    ".anat-play-asthma .mucus { opacity:0; transition: opacity 1.2s; }",
    ".anat-play-asthma.st3 .mucus, .anat-play-asthma.st4 .mucus { opacity:.85; }",
    ".anat-play-asthma .air-weak { animation-duration: 3.6s; opacity:.4; }",

    /* ---------- 病理：高血压 ---------- */
    ".anat-play-hbp .lumen { transition: transform 1.4s ease; transform-box:fill-box; transform-origin:50% 50%; }",
    ".anat-play-hbp.st2 .lumen { transform:scale(.68); }",
    ".anat-play-hbp.st3 .lumen { transform:scale(.55); }",
    ".anat-play-hbp.st4 .lumen { transform:scale(.5); }",
    ".anat-play-hbp.st3 .anat-part[data-part='heart'] { transform-box:fill-box; transform-origin:50% 50%; animation: anatHbpPump .7s ease-in-out infinite; }",
    "@keyframes anatHbpPump { 50% { transform:scale(1.16); } }",
    ".anat-play-hbp .press { opacity:0; transition: opacity 1s; }",
    ".anat-play-hbp.st2 .press, .anat-play-hbp.st3 .press, .anat-play-hbp.st4 .press { opacity:1; }",

    /* ---------- 病理：2型糖尿病 ---------- */
    ".anat-play-t2d .glu-pile { opacity:0; transition: opacity 1.2s; }",
    ".anat-play-t2d.st3 .glu-pile, .anat-play-t2d.st4 .glu-pile { opacity:1; }",
    ".anat-play-t2d .key { transition: transform 1.2s ease; }",
    ".anat-play-t2d.st2 .key { transform: translate(10px,-8px) rotate(24deg); }",
    ".anat-play-t2d.st4 .ins-pan { transform-box:fill-box; transform-origin:50% 50%; animation: anatIns 1.4s ease-in-out infinite; }",

    /* ---------- 病理：胆结石发作 ---------- */
    ".anat-play-gall .anat-part[data-part='gall'] { transform-box:fill-box; transform-origin:50% 50%; }",
    ".anat-play-gall.st2 .anat-part[data-part='gall'] { animation: anatGallSq 1.2s ease-in-out infinite; }",
    ".anat-play-gall.st3 .anat-part[data-part='gall'], .anat-play-gall.st4 .anat-part[data-part='gall'] { animation:none; transform:scale(.94); }",
    "@keyframes anatGallSq { 50% { transform:scale(.9); } }",
    ".anat-play-gall .g-stone { transition: transform 1.4s ease; }",
    ".anat-play-gall.st3 .g-stone, .anat-play-gall.st4 .g-stone { transform: translate(-16px,-14px); }",
    ".anat-play-gall .g-pain { opacity:0; transition: opacity 1s; }",
    ".anat-play-gall.st4 .g-pain { opacity:1; }",

    /* ---------- 病理：胃炎/溃疡 ---------- */
    ".anat-play-gastr .hpylori { animation: anatWiggle2 1.4s ease-in-out infinite; }",
    "@keyframes anatWiggle2 { 50% { transform: translate(3px,-4px) rotate(10deg); } }",
    ".anat-play-gastr .erosion { transform-box:fill-box; transform-origin:50% 50%; transform:scale(.3); opacity:.4; transition: all 1.4s ease; }",
    ".anat-play-gastr.st3 .erosion { transform:scale(1); opacity:1; }",
    ".anat-play-gastr.st4 .erosion { transform:scale(1.25); fill:#7f1d1d; }",

    /* ---------- 病理：脑卒中 ---------- */
    ".anat-play-stroke .clot-go { transition: transform 1.8s ease-in; }",
    ".anat-play-stroke.st2 .clot-go { transform: translate(34px,-96px); }",
    ".anat-play-stroke.st3 .clot-go { transform: translate(52px,-138px); }",
    ".anat-play-stroke .brain-dim { opacity:0; transition: opacity 1.6s; }",
    ".anat-play-stroke.st3 .brain-dim { opacity:.55; }",
    ".anat-play-stroke.st4 .brain-dim { opacity:.8; }",
    ".anat-play-stroke .fast { opacity:0; transition: opacity 1s; }",
    ".anat-play-stroke.st4 .fast { opacity:1; }"
  ].join("\n");

  function injectCSS() {
    if (document.getElementById("anat-anim-css")) return;
    var st = document.createElement("style");
    st.id = "anat-anim-css";
    st.textContent = CSS;
    document.head.appendChild(st);
  }

  /* ---------- 动画注册表 ----------
   * kind: physio=循环生理动画（setup 注入持续动画元素）
   *       patho=分步病理动画（stages 时间轴，t=进入该步的秒）
   */
  var ANIMS = [
    {
      id: "heartbeat", sys: "circ", kind: "physio", name: "心跳与血液循环",
      desc: "心脏每收缩一次，把血液泵入主动脉；静脉血经肺动脉入肺加氧，鲜红的动脉血再流回全身。",
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<path class="flow flow-red flow-anim" d="M244,212 Q262,264 252,330 Q242,414 230,466"/>' +
          '<path class="flow flow-red flow-anim" d="M240,214 Q254,246 250,290" opacity=".7"/>' +
          '<path class="flow flow-blue flow-anim rev" d="M220,214 Q206,258 212,330 Q218,408 228,462"/>' +
          '<path class="flow flow-blue flow-anim" d="M234,200 Q262,190 288,204"/>' +
          '<path class="flow flow-red flow-anim rev" d="M234,206 Q204,196 178,208"/>';
      }
    },
    {
      id: "breathing", sys: "resp", kind: "physio", name: "呼吸运动",
      desc: "膈肌收缩下沉，胸腔变大、肺随之扩张吸入空气；膈肌舒张上抬，肺回缩把废气排出。",
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<path class="flow flow-air air-in flow-anim" d="M230,52 L230,180 M230,150 L216,190 M230,150 L244,190"/>' +
          '<path class="flow flow-air air-out flow-anim" d="M230,190 L230,52"/>';
      }
    },
    {
      id: "peristalsis", sys: "digest", kind: "physio", name: "消化蠕动",
      desc: "食道蠕动把食物团送入胃，胃研磨搅拌后，食糜在小肠被进一步消化吸收，残渣进入大肠。",
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<circle r="8" fill="#a16207" opacity=".85">' +
          '<animateMotion dur="7s" repeatCount="indefinite" path="M238,100 Q244,180 254,296 Q270,306 292,318 Q300,336 268,344 Q238,352 226,368 Q198,392 224,408 Q262,420 240,440 Q210,452 186,440"/>' +
          "</circle>" +
          '<circle r="3.4" fill="#fff" opacity=".9">' +
          '<animateMotion dur="7s" begin="0.25s" repeatCount="indefinite" path="M238,100 Q244,180 254,296 Q270,306 292,318 Q300,336 268,344 Q238,352 226,368 Q198,392 224,408 Q262,420 240,440 Q210,452 186,440"/>' +
          "</circle>";
      }
    },
    {
      id: "nervesis", sys: "nerve", kind: "physio", name: "神经信号传导",
      desc: "大脑发出指令沿脊髓下行，经脊神经传到四肢；感觉信号也沿同一条「高速路」上传回大脑。",
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<path class="sig" d="M230,40 L230,470"/>' +
          '<path class="sig" d="M230,140 L176,190 M230,140 L284,190" style="animation-delay:.2s"/>' +
          '<path class="sig" d="M230,360 L172,410 M230,360 L288,410 M230,400 L180,470 M230,400 L280,470" style="animation-delay:.45s"/>';
      }
    },
    {
      id: "atheroscl", sys: "circ", kind: "patho", name: "动脉粥样硬化与心梗",
      desc: "高血压/高血脂/吸烟等因素让脂质沉积在动脉壁，斑块越长越大；斑块一旦破裂，血小板聚成血栓堵死血管，下游心肌缺血坏死——这就是心梗。",
      stages: [
        { t: 0, caption: "正常冠状动脉：管腔通畅，血流顺畅。" },
        { t: 4, caption: "脂质沉积：胆固醇等在血管壁形成黄色脂纹（斑块萌芽）。" },
        { t: 8, caption: "斑块增大：管腔明显狭窄，血流受阻，活动后易胸闷胸痛（心绞痛）。" },
        { t: 12, caption: "斑块破裂→血栓形成：血管被完全堵死，下游心肌缺血坏死——急性心肌梗死！" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<path class="flow flow-red vessel-flow flow-anim" d="M236,214 Q268,236 296,246" style="stroke-width:7;opacity:.85"/>' +
          '<g class="plaque"><ellipse cx="266" cy="236" rx="15" ry="9" fill="#fbbf24" stroke="#d97706" transform="rotate(-24 266 236)"/><ellipse cx="272" cy="232" rx="6" ry="3.4" fill="#fef3c7"/></g>' +
          '<g class="clot"><ellipse cx="288" cy="243" rx="10" ry="7" fill="#7f1d1d" transform="rotate(-20 288 243)"/></g>' +
          '<g class="heart-dim">' +
          '<path d="M296,250 Q312,258 310,272" fill="none" stroke="#94a3b8" stroke-dasharray="3 4" stroke-width="2"/>' +
          "</g>";
      }
    },
    {
      id: "pneumonia", sys: "resp", kind: "patho", name: "肺炎的发展",
      desc: "病原体侵入肺泡后，免疫细胞与渗出液涌入肺泡，肺泡失去换气能力——受累肺组织「实变」，出现发热、咳嗽、气促。",
      stages: [
        { t: 0, caption: "正常肺泡：薄壁气囊，氧气与二氧化碳自由交换。" },
        { t: 4, caption: "病原入侵：细菌/病毒进入肺泡，免疫细胞拉响警报。" },
        { t: 8, caption: "炎性渗出：白细胞和液体涌入肺泡，肺泡被「灌满」。" },
        { t: 12, caption: "肺实变：受累肺段失去换气功能——发热、咳嗽、气促，需要及时治疗。" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        var germs = "", exu = "";
        var pts = [[188, 232], [200, 244], [192, 258], [206, 234], [182, 246], [198, 266], [210, 252], [186, 272]];
        pts.forEach(function (p, i) {
          germs += '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="3.2" fill="#dc2626" class="germ"/>';
          exu += '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="9" fill="#cbd5e1" class="exudate"/>';
        });
        o.innerHTML = exu + germs;
      }
    },
    {
      id: "append", sys: "digest", kind: "patho", name: "急性阑尾炎",
      desc: "阑尾腔被粪石或淋巴滤泡增生堵住后，腔内压力升高、细菌繁殖，阑尾充血水肿→化脓→可能穿孔，典型信号是转移性右下腹痛。",
      stages: [
        { t: 0, caption: "正常阑尾：细长小管，安静地挂在盲肠末端。" },
        { t: 4, caption: "腔内梗阻：粪石/增生堵塞管腔，阑尾开始充血水肿。" },
        { t: 8, caption: "化脓性阑尾炎：腔内积脓、张力升高，右下腹疼痛加剧伴发热。" },
        { t: 12, caption: "穿孔风险：脓头随时可能破溃引起腹膜炎——需尽快手术！" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<circle class="pus" cx="158" cy="462" r="5" fill="#fef9c3" stroke="#eab308"/>';
      }
    },
    {
      id: "stone", sys: "uri", kind: "patho", name: "肾结石梗阻",
      desc: "结石从肾盂掉进输尿管，卡在狭窄处造成急性梗阻，肾盂积水内压升高——突发腰腹部绞痛，常伴恶心呕吐、血尿。",
      stages: [
        { t: 0, caption: "肾盂内结石：多数小结石可随尿液自行排出。" },
        { t: 4, caption: "结石下移：掉入输尿管，随蠕动缓慢下行。" },
        { t: 8, caption: "急性梗阻：卡在输尿管狭窄段，肾盂积水、内压升高。" },
        { t: 12, caption: "肾绞痛发作：腰腹部剧烈绞痛、恶心呕吐、血尿——及时就医解痉止痛。" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<g class="hydro"><ellipse cx="194" cy="376" rx="16" ry="12" fill="#67e8f9" opacity=".55"/></g>' +
          '<circle class="stone" cx="192" cy="380" r="6" fill="#a8a29e" stroke="#57534e" stroke-width="1.5"/>' +
          '<g class="pain"><path d="M168,344 L162,332 M176,340 L172,326 M184,340 L184,326" stroke="#f59e0b" stroke-width="3" stroke-linecap="round"/></g>';
      }
    },
    {
      id: "immune", sys: "immune", kind: "physio", name: "免疫应答",
      desc: "病原体入侵后，白细胞从血液赶往现场吞噬围剿；淋巴结「哨站」接警后肿大增援——这就是发炎时淋巴结肿大的原因。",
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<g class="germ-i germ-die">' +
          '<circle cx="202" cy="384" r="4.5" fill="#dc2626"/><circle cx="216" cy="392" r="4" fill="#ef4444"/><circle cx="206" cy="399" r="3.6" fill="#b91c1c"/>' +
          "</g>" +
          '<circle class="wbc-a" cx="166" cy="350" r="8" fill="#f8fafc" stroke="#94a3b8" stroke-width="2"/>' +
          '<circle class="wbc-b" cx="288" cy="362" r="8" fill="#f8fafc" stroke="#94a3b8" stroke-width="2"/>' +
          '<circle class="node" cx="322" cy="334" r="12" fill="none" stroke="#16a34a" stroke-width="2.4"/>';
      }
    },
    {
      id: "insulin", sys: "endo", kind: "physio", name: "血糖调节",
      desc: "餐后血糖升高，胰岛立刻分泌胰岛素，像钥匙一样打开细胞之门，让葡萄糖进入细胞供能或储存，血糖随之回落。",
      setup: function (scene) {
        var o = overlay(scene);
        var dots = "";
        var offs = [[0, 0], [8, -8], [-6, 8], [14, 6], [-10, -4], [4, 12]];
        for (var i = 0; i < offs.length; i++) {
          dots += '<circle class="glu d' + (i + 1) + '" cx="' + (238 + offs[i][0]) + '" cy="' + (354 + offs[i][1]) + '" r="3.4" fill="#eab308"/>';
        }
        o.innerHTML =
          dots +
          '<circle class="ins-sig" cx="252" cy="362" r="9" fill="none" stroke="#16a34a" stroke-width="2.6"/>' +
          '<g class="cell-door"><circle cx="292" cy="404" r="14" fill="none" stroke="#16a34a" stroke-dasharray="4 4" stroke-width="2.4"/></g>' +
          '<text x="292" y="432" text-anchor="middle" font-size="10" fill="#5f768a">细胞（葡萄糖入内）</text>';
      }
    },
    {
      id: "urine", sys: "uri", kind: "physio", name: "尿液生成与排放",
      desc: "血液流经肾小球被滤过，经肾小管重吸收后成为尿液，沿输尿管缓缓流入膀胱储存，充盈到约 400ml 产生尿意。",
      setup: function (scene) {
        var o = overlay(scene);
        var drop = function (cls, begin) {
          return '<circle class="urine-drop ' + cls + '" r="3.2" fill="#0ea5e9">' +
            '<animateMotion dur="6s" begin="' + begin + '" repeatCount="indefinite" path="M196,412 Q202,446 220,460"/>' +
            "</circle>";
        };
        o.innerHTML =
          drop("", "0s") + drop("d2", "1.2s") + drop("d3", "2.4s") +
          '<circle class="urine-drop" r="3.2" fill="#38bdf8">' +
          '<animateMotion dur="6s" begin="0.6s" repeatCount="indefinite" path="M264,412 Q258,446 240,460"/>' +
          "</circle>";
      }
    },
    {
      id: "gas", sys: "resp", kind: "physio", name: "肺泡气体交换",
      desc: "吸入的氧气穿过薄薄的肺泡壁进入毛细血管，被红细胞送往全身；代谢产生的二氧化碳则反向逸出肺泡，随呼气排出。",
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<circle class="o2" cx="196" cy="272" r="3.6" fill="#ef4444"/>' +
          '<circle class="o2 d2" cx="203" cy="276" r="3.6" fill="#ef4444"/>' +
          '<circle class="o2 d3" cx="199" cy="281" r="3.6" fill="#ef4444"/>' +
          '<circle class="co2" cx="207" cy="296" r="3.6" fill="#3b82f6"/>' +
          '<circle class="co2 d2" cx="196" cy="298" r="3.6" fill="#3b82f6"/>' +
          '<text x="163" y="316" font-size="9.5" fill="#64748b">O₂入血 ↓</text>' +
          '<text x="215" y="316" font-size="9.5" fill="#64748b">CO₂排出 ↑</text>';
      }
    },
    {
      id: "reflex", sys: "nerve", kind: "physio", name: "膝跳反射（反射弧）",
      desc: "叩击髌腱，信号传入脊髓后不经过大脑、直接触发运动指令——小腿前踢。这类「短路反射」保护我们免受伤害。",
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<g class="tap"><path d="M188,592 L204,592 M196,584 L196,600" stroke="#f59e0b" stroke-width="3.4" stroke-linecap="round"/></g>' +
          '<path class="seg-up" d="M198,586 Q210,540 218,478" fill="none" stroke="#8b5cf6" stroke-width="4" stroke-linecap="round" stroke-dasharray="10 50"/>' +
          '<path class="seg-down" d="M218,478 Q206,534 194,580" fill="none" stroke="#0ea5e9" stroke-width="4" stroke-linecap="round" stroke-dasharray="10 50"/>' +
          '<g class="kick"><path d="M214,616 Q246,622 274,610" fill="none" stroke="#f59e0b" stroke-width="3.6" stroke-linecap="round" stroke-dasharray="2 6"/>' +
          '<text x="252" y="640" text-anchor="middle" font-size="10" fill="#b45309">小腿前踢</text></g>';
      }
    },
    {
      id: "gerd", sys: "digest", kind: "patho", name: "胃食管反流",
      desc: "贲门（胃的入口括约肌）该关不关时，胃酸和食物逆流而上，反复刺激食道下段黏膜——烧心、反酸，长期可致反流性食管炎。",
      stages: [
        { t: 0, caption: "正常状态：贲门括约肌紧闭，胃酸只在胃里干活。" },
        { t: 4, caption: "括约肌松弛：饱餐、躺平、腹压升高时关不严。" },
        { t: 8, caption: "胃酸反流：胃内容物冲进食道，胸骨后火烧样灼痛（烧心）。" },
        { t: 12, caption: "反流性食管炎：食道下段黏膜被反复灼伤——少食多餐、睡前 3 小时不进食、控制体重。" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<g class="acid">' +
          '<circle cx="262" cy="336" r="4" fill="#f97316"/><circle cx="272" cy="326" r="4" fill="#fb923c"/><circle cx="266" cy="345" r="3.6" fill="#ea580c"/>' +
          "</g>" +
          '<rect class="injure" x="243" y="240" width="10" height="46" rx="5" fill="#ef4444" opacity=".8"/>';
      }
    },
    {
      id: "asthma", sys: "resp", kind: "patho", name: "哮喘发作",
      desc: "气道遇到过敏原/冷空气等刺激后，平滑肌痉挛收缩、黏膜水肿、黏液增多，管腔急剧变窄——呼气困难、喘鸣、胸闷。",
      stages: [
        { t: 0, caption: "正常支气管：管腔通畅，气流畅通无阻。" },
        { t: 4, caption: "平滑肌痉挛：支气管壁收缩，管腔开始变窄。" },
        { t: 8, caption: "黏膜水肿+黏液增多：管腔进一步被挤压、堵塞。" },
        { t: 12, caption: "哮喘发作：气流严重受限，呼气性呼吸困难、喘鸣音——随身携带急救吸入剂！" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<g>' +
          '<circle cx="326" cy="118" r="27" fill="#fde68a" opacity=".55"/>' +
          '<circle class="mucus" cx="326" cy="118" r="20" fill="#a3e635" opacity=".5"/>' +
          '<circle class="lumen" cx="326" cy="118" r="15" fill="#fff"/>' +
          '<text x="326" y="158" text-anchor="middle" font-size="10" fill="#64748b">支气管截面</text>' +
          "</g>" +
          '<path class="flow flow-air flow-anim air-weak" d="M230,110 L230,176" fill="none"/>';
      }
    },
    {
      id: "hbp", sys: "circ", kind: "patho", name: "高血压",
      desc: "外周小血管长期收缩、管壁增厚变窄，血流阻力增大；心脏只能「加力泵血」，血压持续升高——心、脑、肾都是受害者。",
      stages: [
        { t: 0, caption: "正常血管：管壁弹性好、管腔宽敞，血压平稳。" },
        { t: 4, caption: "血管收缩/硬化：管腔变窄，血流阻力增大，血压升高。" },
        { t: 8, caption: "心脏加压泵血：心肌被迫增厚（左心室肥厚），泵效率开始下降。" },
        { t: 12, caption: "靶器官受损：长期高压伤及心、脑、肾、眼底——规律服药+限盐是根本。" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<g>' +
          '<circle cx="326" cy="118" r="27" fill="#fecaca" opacity=".7"/>' +
          '<circle class="lumen" cx="326" cy="118" r="17" fill="#fff"/>' +
          '<text x="326" y="158" text-anchor="middle" font-size="10" fill="#64748b">小动脉截面</text>' +
          "</g>" +
          '<g class="press">' +
          '<path d="M282,86 L270,74 M296,78 L292,62 M312,74 L314,58" stroke="#dc2626" stroke-width="3" stroke-linecap="round"/>' +
          '<text x="326" y="44" text-anchor="middle" font-size="11" fill="#dc2626" font-weight="bold">血压 ↑</text>' +
          "</g>";
      }
    },
    {
      id: "t2d", sys: "endo", kind: "patho", name: "2型糖尿病（胰岛素抵抗）",
      desc: "胰岛素这把「钥匙」还在分泌，但细胞上的「锁」变了形打不开——葡萄糖进不了细胞，堆在血液里；胰岛长期加班最终疲惫衰竭。",
      stages: [
        { t: 0, caption: "正常：胰岛素与受体结合，葡萄糖顺利进入细胞。" },
        { t: 4, caption: "胰岛素抵抗：受体（锁）对胰岛素不敏感，钥匙插不进去。" },
        { t: 8, caption: "血糖堆积：葡萄糖进不了细胞，滞留血液——血糖升高。" },
        { t: 12, caption: "胰岛衰竭：长期加班分泌胰岛素的胰岛细胞逐渐力竭——控饮食+运动+药物减轻负担。" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<g>' +
          '<circle cx="300" cy="330" r="22" fill="none" stroke="#0ea5e9" stroke-width="2.6" stroke-dasharray="5 4"/>' +
          '<rect x="274" y="322" width="8" height="16" rx="3" fill="#0ea5e9"/>' +
          '<text x="300" y="368" text-anchor="middle" font-size="10" fill="#64748b">细胞</text>' +
          "</g>" +
          '<g class="key"><circle cx="252" cy="330" r="7" fill="#16a34a"/><rect x="259" y="327" width="12" height="6" rx="2" fill="#16a34a"/></g>' +
          '<g class="glu-pile">' +
          '<circle cx="206" cy="300" r="4" fill="#eab308"/><circle cx="216" cy="294" r="4" fill="#eab308"/><circle cx="212" cy="308" r="4" fill="#eab308"/><circle cx="224" cy="302" r="4" fill="#eab308"/><circle cx="218" cy="314" r="4" fill="#eab308"/><circle cx="230" cy="310" r="4" fill="#eab308"/>' +
          '<text x="206" y="336" text-anchor="middle" font-size="10" fill="#b45309">葡萄糖滞留血液</text>' +
          "</g>" +
          '<g class="ins-pan"><circle cx="262" cy="362" r="10" fill="none" stroke="#16a34a" stroke-width="2.6"/></g>';
      }
    },
    {
      id: "gall", sys: "digest", kind: "patho", name: "胆结石发作",
      desc: "胆囊里的结石平时相安无事；一旦油腻餐触发胆囊猛烈收缩，结石卡在胆囊管口，胆汁排不出——右上腹剧痛（胆绞痛）。",
      stages: [
        { t: 0, caption: "结石静卧胆囊：多数人无症状（「沉默结石」）。" },
        { t: 4, caption: "油腻餐触发：胆囊收缩排出胆汁帮助消化脂肪。" },
        { t: 8, caption: "结石卡住胆囊管：胆汁排不出，胆囊内压骤升。" },
        { t: 12, caption: "胆绞痛发作：右上腹剧烈绞痛、可放射到右肩——反复发作建议外科评估。" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<circle class="g-stone" cx="238" cy="338" r="6.5" fill="#a8a29e" stroke="#57534e" stroke-width="1.5"/>' +
          '<g class="g-pain"><path d="M256,318 L250,306 M264,314 L262,300 M272,318 L276,306" stroke="#ef4444" stroke-width="3" stroke-linecap="round"/></g>';
      }
    },
    {
      id: "gastr", sys: "digest", kind: "patho", name: "胃炎与胃溃疡",
      desc: "幽门螺杆菌定植+胃酸/药物等攻击因素增强，胃黏膜屏障被破坏，从浅表炎症发展成糜烂、溃疡——规律性上腹痛，可并发出血、穿孔。",
      stages: [
        { t: 0, caption: "健康胃黏膜：完整的黏液屏障抵御胃酸。" },
        { t: 4, caption: "幽门螺杆菌定植：细菌破坏黏液层，炎症开始。" },
        { t: 8, caption: "糜烂加深：胃酸直接侵蚀胃壁，形成溃疡面——规律性上腹痛。" },
        { t: 12, caption: "溃疡加重：可并发出血（黑便/呕血）、穿孔——及时根除幽门螺杆菌+抑酸治疗。" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<g class="hpylori">' +
          '<path d="M262,318 q4,-5 8,0 q4,5 8,0" fill="none" stroke="#7c3aed" stroke-width="2.6" stroke-linecap="round"/>' +
          '<path d="M256,332 q4,-5 8,0 q4,5 8,0" fill="none" stroke="#7c3aed" stroke-width="2.6" stroke-linecap="round"/>' +
          "</g>" +
          '<ellipse class="erosion" cx="272" cy="334" rx="11" ry="6" fill="#dc2626"/>';
      }
    },
    {
      id: "stroke", sys: "nerve", kind: "patho", name: "脑卒中（中风）",
      desc: "心脏或颈动脉的血栓脱落后随血流冲入脑部，卡住脑血管，下游脑组织几分钟内开始缺血坏死——时间就是大脑，FAST 识别立即拨打 120。",
      stages: [
        { t: 0, caption: "血栓形成：多起源于心脏（房颤）或颈动脉斑块。" },
        { t: 4, caption: "血栓脱落：随血流向脑部移动。" },
        { t: 8, caption: "堵塞脑血管：下游脑组织供血中断，开始缺血坏死。" },
        { t: 12, caption: "FAST 识别中风：脸歪(F)、肢无力(A)、言语不清(S)——第一时间(T)拨打 120！" }
      ],
      setup: function (scene) {
        var o = overlay(scene);
        o.innerHTML =
          '<path d="M222,120 Q230,80 244,58 Q254,44 268,40" fill="none" stroke="#dc2626" stroke-width="5" opacity=".7" stroke-linecap="round"/>' +
          '<circle class="clot-go" cx="216" cy="150" r="7" fill="#7f1d1d"/>' +
          '<ellipse class="brain-dim" cx="258" cy="48" rx="20" ry="15" fill="#1e293b"/>' +
          '<g class="fast"><text x="118" y="142" font-size="9.5" fill="#dc2626" font-weight="bold">⚠ 口角歪斜 / 单侧肢无力 / 说话不清</text>' +
          '<text x="118" y="156" font-size="9.5" fill="#dc2626" font-weight="bold">立即拨打 120</text></g>';
      }
    }
  ];

  /* ---------- 工具 ---------- */
  function overlay(scene) {
    var o = scene.querySelector(".anat-anim-overlay");
    if (!o) {
      o = document.createElementNS("http://www.w3.org/2000/svg", "g");
      o.setAttribute("class", "anat-anim-overlay");
      var labels = scene.querySelector(".anat-labels");
      if (labels) scene.insertBefore(o, labels);
      else scene.appendChild(o);
    }
    o.innerHTML = "";
    return o;
  }
  function findAnim(id) {
    for (var i = 0; i < ANIMS.length; i++) if (ANIMS[i].id === id) return ANIMS[i];
    return null;
  }

  /* ---------- 播放器状态 ---------- */
  var current = null; /* {anim, scene, raf, t0, elapsed, paused, stageIdx, ctl} */

  function stop() {
    if (!current) return;
    if (current.raf) cancelAnimationFrame(current.raf);
    current.scene.classList.remove("anat-paused");
    var ov = current.scene.querySelector(".anat-anim-overlay");
    if (ov) ov.remove();
    ANIMS.forEach(function (a) {
      for (var i = 1; i <= 6; i++) current.scene.classList.remove("st" + i);
      current.scene.classList.remove("anat-play-" + a.id);
    });
    if (current.svg && current.svg.unpauseAnimations) current.svg.unpauseAnimations();
    current = null;
  }

  function renderCtl() {
    var c = current;
    c.ctl.innerHTML =
      '<div class="anat-anim-ctl">' +
      '<button type="button" class="anat-btn-toggle">' + (c.paused ? "▶ 播放" : "⏸ 暂停") + "</button>" +
      '<button type="button" class="anat-btn-replay">↺ 重播</button>' +
      '<span class="anat-anim-cap"></span>' +
      '<span class="anat-anim-step"></span>' +
      "</div>";
    c.ctl.querySelector(".anat-btn-toggle").addEventListener("click", function () { c.paused ? resume() : pause(); });
    c.ctl.querySelector(".anat-btn-replay").addEventListener("click", function () { play(c.anim.id, c.scene, c.ctl, c.infoSlot); });
    capEl().textContent = c.anim.desc;
  }
  function capEl() { return current.ctl.querySelector(".anat-anim-cap"); }
  function stepEl() { return current.ctl.querySelector(".anat-anim-step"); }
  function btnEl() { return current.ctl.querySelector(".anat-btn-toggle"); }

  function pause() {
    if (!current) return;
    current.paused = true;
    if (current.raf) cancelAnimationFrame(current.raf);
    current.scene.classList.add("anat-paused");
    if (current.svg && current.svg.pauseAnimations) current.svg.pauseAnimations();
    btnEl().textContent = "▶ 播放";
  }
  function resume() {
    if (!current) return;
    current.paused = false;
    current.t0 = performance.now() - current.elapsed * 1000;
    current.scene.classList.remove("anat-paused");
    if (current.svg && current.svg.unpauseAnimations) current.svg.unpauseAnimations();
    btnEl().textContent = "⏸ 暂停";
    tick();
  }

  function applyStage(idx) {
    var c = current, st = c.anim.stages[idx];
    if (idx === c.stageIdx) return;
    c.stageIdx = idx;
    capEl().textContent = st.caption;
    stepEl().textContent = (idx + 1) + "/" + c.anim.stages.length;
    for (var i = 1; i <= c.anim.stages.length; i++) c.scene.classList.remove("st" + i);
    c.scene.classList.add("st" + (idx + 1));
  }

  function tick() {
    var c = current;
    if (!c || c.paused) return;
    var now = performance.now();
    c.elapsed = (now - c.t0) / 1000;
    var stages = c.anim.stages;
    var total = stages[stages.length - 1].t + 4; /* 末步停留 4s */
    var t = c.elapsed % total;
    var idx = 0;
    for (var i = stages.length - 1; i >= 0; i--) {
      if (t >= stages[i].t) { idx = i; break; }
    }
    applyStage(idx);
    current.raf = requestAnimationFrame(tick);
  }

  function play(id, scene, ctl, infoSlot) {
    stop();
    injectCSS();
    var anim = findAnim(id);
    if (!anim || !scene) return;
    current = { anim: anim, scene: scene, svg: scene, ctl: ctl, infoSlot: infoSlot, t0: performance.now(), elapsed: 0, paused: false, stageIdx: -1 };
    scene.classList.add("anat-play-" + id);
    anim.setup(scene);
    renderCtl();
    if (anim.kind === "patho") {
      tick();
    } else {
      capEl().textContent = anim.desc;
      stepEl().textContent = "循环演示";
    }
  }

  /* ---------- 3D 渲染视频（scripts/bake-3d.sh 产出，/media/ 提供） ----------
  /* ---------- 3D 渲染视频（scripts/bake-3d.sh 产出，/media/ 提供） ----------
   * key = 关键词（匹配部位 id），value = 媒体文件名。构建后由 bake-3d.sh 产出。
   * 视频来源: Z-Anatomy 模型 (CC BY-SA 4.0) 渲染 — 见 external/z-anatomy/ATTRIBUTION.md
   */
  var MEDIA3D = {
    heart: "heart.webm",
    lungs: "lungs.webm",
    brain: "brain.webm",
    liver: "liver.webm",
    stomach: "stomach.webm",
    kidneys: "kidneys.webm"
  };

  /* ---------- chips 绑定（由 anatomy.js 的 renderModal 调用） ---------- */
  function mediaForSystem(sys) {
    var keys = [];
    window.AnatomyView.PARTS.forEach(function (p) {
      if (sys && p.sys !== sys) return;
      [p.id, p.unit].forEach(function (k) {
        if (MEDIA3D[k] && keys.indexOf(k) < 0) keys.push(k);
      });
    });
    return keys.map(function (k) { return { key: k, file: MEDIA3D[k] }; });
  }
  function playMedia(name, slot) {
    stop();
    slot.innerHTML =
      '<video class="anat-video" src="/media/' + name + '" controls autoplay loop muted playsinline></video>' +
      '<div class="anat-media-credit">3D 模型渲染 · Z-Anatomy (CC BY-SA 4.0) / BodyParts3D · <a href="https://github.com/Z-Anatomy" target="_blank" rel="noopener">来源</a></div>';
    var v = slot.querySelector("video");
    if (v) {
      v.addEventListener("error", function () {
        slot.innerHTML = '<div class="anat-info-hint">该 3D 动画尚未生成 — 运行 scripts/bake-3d.sh ' + name.replace(".webm", "") + ' 烘焙</div>';
      }, true);
    }
  }

  function forSystem(sys) {
    return ANIMS.filter(function (a) { return a.sys === sys; })
      .map(function (a) { return { id: a.id, name: a.name, kind: a.kind }; });
  }
  function all() {
    return ANIMS.map(function (a) { return { id: a.id, name: a.name, kind: a.kind, sys: a.sys }; });
  }
  function bindChips(chipsEl, svg, footEl, sys) {
    chipsEl.querySelectorAll("[data-anim]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var infoSlot = footEl.querySelector(".anat-info-slot");
        play(btn.getAttribute("data-anim"), svg, infoSlot, infoSlot);
        chipsEl.querySelectorAll(".anat-anim-chip").forEach(function (b) { b.classList.remove("active"); });
        btn.classList.add("active");
      });
    });
    chipsEl.querySelectorAll("[data-media]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var infoSlot = footEl.querySelector(".anat-info-slot");
        playMedia(btn.getAttribute("data-media"), infoSlot);
        chipsEl.querySelectorAll(".anat-anim-chip").forEach(function (b) { b.classList.remove("active"); });
        btn.classList.add("active");
      });
    });
  }

  window.AnatomyAnim = { play: play, stop: stop, forSystem: forSystem, all: all, mediaForSystem: mediaForSystem, bindChips: bindChips };
})();
