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
    ".anat-play-stone .pain { opacity:0; transition: opacity 1s; }"
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

  /* ---------- chips 绑定（由 anatomy.js 的 renderModal 调用） ---------- */
  function forSystem(sys) {
    return ANIMS.filter(function (a) { return a.sys === sys; })
      .map(function (a) { return { id: a.id, name: a.name, kind: a.kind }; });
  }
  function all() {
    return ANIMS.map(function (a) { return { id: a.id, name: a.name, kind: a.kind, sys: a.sys }; });
  }
  function bindChips(chipsEl, svg, footEl) {
    chipsEl.querySelectorAll("[data-anim]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var infoSlot = footEl.querySelector(".anat-info-slot");
        play(btn.getAttribute("data-anim"), svg, infoSlot, infoSlot);
        chipsEl.querySelectorAll(".anat-anim-chip").forEach(function (b) { b.classList.remove("active"); });
        btn.classList.add("active");
      });
    });
  }

  window.AnatomyAnim = { play: play, stop: stop, forSystem: forSystem, all: all, bindChips: bindChips };
})();
