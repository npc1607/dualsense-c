const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

function buildExcalidrawJSON() {
  let idCounter = 1;
  const genId = (prefix) => `${prefix}_${idCounter++}_${Math.random().toString(36).substr(2, 6)}`;
  const elements = [];

  function addRect(cfg) {
    elements.push({
      id: cfg.id || genId('rect'),
      type: 'rectangle',
      x: cfg.x, y: cfg.y, width: cfg.width, height: cfg.height,
      angle: 0,
      strokeColor: cfg.strokeColor || '#1e1e1e',
      backgroundColor: cfg.backgroundColor || '#ffffff',
      fillStyle: 'solid',
      strokeWidth: cfg.strokeWidth || 2,
      strokeStyle: cfg.strokeStyle || 'solid',
      roughness: 1,
      opacity: 100,
      groupIds: [],
      frameId: null,
      roundness: { type: 3 },
      seed: Math.floor(Math.random() * 1000000),
      version: 1, versionNonce: 1, isDeleted: false,
      boundElements: null, updated: Date.now(), link: null, locked: false
    });
  }

  function addText(cfg) {
    elements.push({
      id: cfg.id || genId('text'),
      type: 'text',
      x: cfg.x, y: cfg.y, width: cfg.width, height: cfg.height,
      angle: 0,
      strokeColor: cfg.strokeColor || '#1e1e1e',
      backgroundColor: 'transparent',
      fillStyle: 'solid',
      strokeWidth: 1, strokeStyle: 'solid', roughness: 1, opacity: 100,
      groupIds: [], frameId: null, roundness: null,
      seed: Math.floor(Math.random() * 1000000),
      version: 1, versionNonce: 1, isDeleted: false, boundElements: null,
      updated: Date.now(), link: null, locked: false,
      text: cfg.text,
      fontSize: cfg.fontSize || 16,
      fontFamily: cfg.fontFamily || 1,
      textAlign: cfg.textAlign || 'center',
      verticalAlign: 'middle',
      containerId: null,
      originalText: cfg.text,
      lineHeight: 1.3
    });
  }

  function addArrow(cfg) {
    const endPoint = cfg.points[cfg.points.length - 1];
    elements.push({
      id: cfg.id || genId('arrow'),
      type: 'arrow',
      x: cfg.startX, y: cfg.startY,
      width: Math.abs(endPoint[0]),
      height: Math.abs(endPoint[1]),
      angle: 0,
      strokeColor: cfg.strokeColor || '#495057',
      backgroundColor: 'transparent',
      fillStyle: 'solid',
      strokeWidth: cfg.strokeWidth || 2,
      strokeStyle: cfg.strokeStyle || 'solid',
      roughness: 1, opacity: 100,
      groupIds: [], frameId: null, roundness: { type: 2 },
      seed: Math.floor(Math.random() * 1000000),
      version: 1, versionNonce: 1, isDeleted: false,
      points: cfg.points,
      startBinding: null, endBinding: null,
      startArrowhead: null, endArrowhead: 'arrow'
    });
  }

  // 大标题
  addText({
    x: 230, y: 35, width: 800, height: 45,
    text: "Audio AI 游戏事件与材质探测架构",
    fontSize: 32, strokeColor: "#1e1e1e", textAlign: "center"
  });
  addText({
    x: 230, y: 85, width: 800, height: 26,
    text: "基于轻量级 YAMNet ONNX 模型 (纯 CPU INT8) 的无遥测游戏感知与力反馈联动流水线",
    fontSize: 16.5, strokeColor: "#495057", textAlign: "center"
  });

  // 第一行：左边游戏进程，右边 Audio AI 探测器
  // 卡片 1: 游戏进程 (x: 50, y: 140, w: 340, h: 220)
  addRect({ x: 50, y: 140, width: 340, height: 220, backgroundColor: "#f8f9fa", strokeColor: "#868e96", strokeWidth: 2 });
  addText({ x: 60, y: 155, width: 320, height: 28, text: "游戏进程 (Game Engine)", fontSize: 20, strokeColor: "#212529" });
  addText({ x: 60, y: 183, width: 320, height: 20, text: "Unreal / Unity / 自研引擎 (48kHz 混音)", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 75, y: 215, width: 290, height: 24, text: "• 物理脚步材质声（水洼/铁网/木板/碎石）", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 245, width: 290, height: 24, text: "• 交火与战斗音效（枪声/机枪连发/爆炸波）", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 275, width: 290, height: 24, text: "• 主角受击、喘息与心跳音效", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 305, width: 290, height: 24, text: "• 零侵入：无需游戏 Mod，无需内存 Hook", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });

  // 连线 1 -> 2
  addArrow({ startX: 390, startY: 250, points: [[0, 0], [70, 0]], strokeColor: "#495057", strokeWidth: 2.2 });
  addText({ x: 395, y: 220, width: 60, height: 20, text: "声卡环回", fontSize: 13, strokeColor: "#495057" });

  // 卡片 2: Audio AI 伴生探测器 (x: 460, y: 140, w: 410, h: 220)
  addRect({ x: 460, y: 140, width: 410, height: 220, backgroundColor: "#ebfbee", strokeColor: "#0ca678", strokeWidth: 2 });
  addText({ x: 470, y: 155, width: 390, height: 28, text: "Audio AI 伴生探测器 (Sidecar)", fontSize: 20, strokeColor: "#0ca678" });
  addText({ x: 470, y: 183, width: 390, height: 20, text: "tools/yamnet_bridge (YAMNet ONNX INT8)", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 485, y: 215, width: 360, height: 24, text: "• WASAPI / PipeWire 实时无损环回录音 (16kHz)", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 485, y: 245, width: 360, height: 24, text: "• 短时傅里叶变换 (STFT) 提取 64 维 Mel 频谱", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 485, y: 275, width: 360, height: 24, text: "• YAMNet ONNX 极轻量推理 (纯 CPU 耗时 1~2ms)", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 485, y: 305, width: 360, height: 24, text: "• 指数平滑 (EMA) 降噪并过滤背景音乐干扰", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });

  // 连线 2 -> 3 (向下折返)
  addArrow({
    startX: 870, startY: 250,
    points: [[0, 0], [40, 0], [40, 160], [-20, 160]],
    strokeColor: "#1971c2", strokeWidth: 2.2
  });
  addText({ x: 920, y: 320, width: 120, height: 22, text: "UDP 9998 数据流", fontSize: 13, strokeColor: "#1971c2", textAlign: "left" });

  // 第二行：左边 dualsense-c 守护进程，右边 DualSense 控制器
  // 卡片 3: dualsense-c 守护进程 (x: 460, y: 400, w: 410, h: 220)
  addRect({ x: 460, y: 400, width: 410, height: 220, backgroundColor: "#e7f5ff", strokeColor: "#1971c2", strokeWidth: 2 });
  addText({ x: 470, y: 415, width: 390, height: 28, text: "dualsense-c 守护进程 (ds5d)", fontSize: 20, strokeColor: "#1971c2" });
  addText({ x: 470, y: 443, width: 390, height: 20, text: "Source: audio_ai ──► Rules ──► Sink 时钟", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 485, y: 475, width: 360, height: 24, text: "• source/audioai: 接收 JSON / DS5A 二进制数据帧", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 485, y: 505, width: 360, height: 24, text: "• 注入 signal.Set (surface_water, audio_gunshot...)", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 485, y: 535, width: 360, height: 24, text: "• rules 表达式求值：优先级裁决、脉冲锁存与衰减", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 485, y: 565, width: 360, height: 24, text: "• 125~250Hz 固定时钟调度 HID 报告与音频触感流", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });

  // 连线 3 -> 4 (向左)
  addArrow({ startX: 460, startY: 510, points: [[0, 0], [-70, 0]], strokeColor: "#495057", strokeWidth: 2.2 });
  addText({ x: 400, y: 485, width: 50, height: 20, text: "驱动下发", fontSize: 13, strokeColor: "#495057" });

  // 卡片 4: PS5 DualSense 控制器 (x: 50, y: 400, w: 340, h: 220)
  addRect({ x: 50, y: 400, width: 340, height: 220, backgroundColor: "#f1f3f5", strokeColor: "#343a40", strokeWidth: 2 });
  addText({ x: 60, y: 415, width: 320, height: 28, text: "DualSense 实体控制器", fontSize: 20, strokeColor: "#212529" });
  addText({ x: 60, y: 443, width: 320, height: 20, text: "自适应扳机 + 音圈高清触感", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 75, y: 475, width: 290, height: 24, text: "• 自适应扳机：瞄准阻尼挡墙 + 开火强击退", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 505, width: 290, height: 24, text: "• 音圈触感 (Ch3/4)：涉水微颤 + 金属空腔脆击", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 535, width: 290, height: 24, text: "• 双马达震动：爆炸低频重击 + 枪机机械后坐", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 565, width: 290, height: 24, text: "• RGB 灯条：残血红光闪烁 + 充电冰蓝呼吸", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });

  // 第三行: YAML 规则配置中心 (x: 50, y: 660, w: 820, h: 180)
  addRect({ x: 50, y: 660, width: 820, height: 180, backgroundColor: "#fff5f5", strokeColor: "#e03131", strokeWidth: 2 });
  addText({ x: 70, y: 678, width: 780, height: 30, text: "⚙️ profiles/audio-ai-demo.yaml 声明式联动规则", fontSize: 21, strokeColor: "#c92a2a" });
  addText({ x: 70, y: 708, width: 780, height: 22, text: "将模型识别出的高层语义事件，以毫秒级规则无缝映射为自适应阻力与振动纹理", fontSize: 14.5, strokeColor: "#495057" });
  addText({ x: 70, y: 742, width: 780, height: 24, text: "• source 块：声明 type: audio_ai、listen: 127.0.0.1:9998、signal_ttl: 600ms 超时失效防护", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 70, y: 772, width: 780, height: 24, text: "• 材质触感：when: 'surface_water > 0.4' ──► 左右马达驱动 60Hz 细腻水波微颤 (sine modulate)", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 70, y: 802, width: 780, height: 24, text: "• 战况反馈：when: 'audio_gunshot > 0.5' ──► 瞬间触发 hold: 120ms 右马达后坐力脉冲", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });

  // 向上注入箭头 (至 dualsense-c)
  addArrow({
    startX: 560, startY: 660,
    points: [[0, 0], [0, -32]],
    strokeColor: "#e03131", strokeWidth: 2, strokeStyle: "dashed"
  });
  addText({ x: 580, y: 635, width: 140, height: 20, text: "注入 YAML 映射策略", fontSize: 13, strokeColor: "#c92a2a", textAlign: "left" });

  return {
    type: "excalidraw",
    version: 2,
    source: "https://excalidraw.com",
    elements,
    appState: { gridSize: null, viewBackgroundColor: "#ffffff" },
    files: {}
  };
}

function buildSVG() {
  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1060 880" width="100%" height="100%">
  <defs>
    <filter id="shadow" x="-5%" y="-5%" width="110%" height="110%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.07"/>
    </filter>
    <filter id="red-shadow" x="-4%" y="-4%" width="108%" height="108%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="5" stdDeviation="8" flood-color="#e03131" flood-opacity="0.10"/>
    </filter>
    <marker id="arrow-gray" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#495057" />
    </marker>
    <marker id="arrow-blue" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#1971c2" />
    </marker>
    <marker id="arrow-red" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#e03131" />
    </marker>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif; font-weight: 800; fill: #1e1e1e; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 500; fill: #495057; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 19px; }
      .card-sub { font-family: "SF Mono", "Consolas", Menlo, monospace; font-size: 12.5px; fill: #868e96; }
      .card-item { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 14px; fill: #2b2b2b; }
      .tag-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 12.5px; font-weight: 600; fill: #495057; text-anchor: middle; }
      .blue-text { font-family: "SF Mono", "Consolas", Menlo, monospace; font-size: 12.5px; font-weight: 700; fill: #1971c2; text-anchor: middle; }
      .cfg-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13px; font-weight: 700; fill: #c92a2a; text-anchor: middle; }
    </style>
  </defs>

  <rect width="100%" height="100%" fill="#ffffff" rx="16"/>

  <!-- 顶部主副标题 -->
  <g transform="translate(530, 48)" text-anchor="middle">
    <text class="title" font-size="28" y="0">Audio AI 游戏事件与材质探测架构</text>
    <text class="subtitle" font-size="15" y="30">基于本地轻量级 YAMNet ONNX 模型 (纯 CPU INT8) 的无遥测游戏感知与力反馈联动流水线</text>
  </g>

  <!-- ================= 第一行流水线 ================= -->

  <!-- 卡片 1: 游戏进程 -->
  <g transform="translate(50, 130)" filter="url(#shadow)">
    <rect width="340" height="220" rx="14" fill="#f8f9fa" stroke="#adb5bd" stroke-width="2"/>
    <text class="card-title" x="170" y="36" text-anchor="middle" fill="#212529">游戏进程 (Game Engine)</text>
    <text class="card-sub" x="170" y="56" text-anchor="middle">Unreal / Unity / 自研引擎 (48kHz 混音)</text>
    <line x1="25" y1="72" x2="315" y2="72" stroke="#dee2e6" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• 物理脚步材质声 (水洼 / 铁网 / 木板 / 碎石)</text>
      <text y="28">• 交火与战斗音效 (枪声 / 机枪连发 / 爆炸波)</text>
      <text y="56">• 主角受击、喘息与心跳危机音效</text>
      <text y="84">• 零侵入：无需游戏 Mod，无需内存 Hook</text>
    </g>
  </g>

  <!-- 连线 1 -> 2 -->
  <line x1="390" y1="240" x2="450" y2="240" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="395" y="212" width="60" height="20" rx="4" fill="#f1f3f5"/>
  <text class="tag-text" x="425" y="226">声卡环回</text>

  <!-- 卡片 2: Audio AI 伴生探测器 -->
  <g transform="translate(460, 130)" filter="url(#shadow)">
    <rect width="450" height="220" rx="14" fill="#ebfbee" stroke="#0ca678" stroke-width="2"/>
    <text class="card-title" x="225" y="36" text-anchor="middle" fill="#0ca678">Audio AI 伴生探测器 (Sidecar)</text>
    <text class="card-sub" x="225" y="56" text-anchor="middle">tools/yamnet_bridge (YAMNet ONNX INT8)</text>
    <line x1="25" y1="72" x2="425" y2="72" stroke="#b2f2bb" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• WASAPI / PipeWire 实时无损环回录音 (16kHz 降采样)</text>
      <text y="28">• 短时傅里叶变换 (STFT) 提取 64 维 Mel 频谱特征</text>
      <text y="56">• YAMNet ONNX 极轻量推理 (纯 CPU 耗时仅 1~2ms)</text>
      <text y="84">• 指数平滑滤波 (EMA) 降噪并过滤背景音乐干扰</text>
    </g>
  </g>

  <!-- 连线 2 -> 3 (折返向下一行) -->
  <path d="M 910 240 L 970 240 Q 985 240 985 260 L 985 490 Q 985 510 965 510 L 920 510" fill="none" stroke="#1971c2" stroke-width="2.5" marker-end="url(#arrow-blue)"/>
  <rect x="925" y="360" width="115" height="24" rx="4" fill="#e7f5ff" stroke="#a5d8ff"/>
  <text class="blue-text" x="982" y="376">UDP 9998 传输</text>

  <!-- ================= 第二行流水线 ================= -->

  <!-- 卡片 3: dualsense-c 守护进程 -->
  <g transform="translate(460, 400)" filter="url(#shadow)">
    <rect width="450" height="220" rx="14" fill="#e7f5ff" stroke="#1971c2" stroke-width="2"/>
    <text class="card-title" x="225" y="36" text-anchor="middle" fill="#1971c2">dualsense-c 守护进程 (ds5d)</text>
    <text class="card-sub" x="225" y="56" text-anchor="middle">Source: audio_ai ──► Rules 引擎 ──► Sink 时钟</text>
    <line x1="25" y1="72" x2="425" y2="72" stroke="#a5d8ff" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• source/audioai 接收 JSON / DS5A 二进制高频数据帧</text>
      <text y="28">• 注入 signal.Set (surface_water, audio_gunshot...)</text>
      <text y="56">• rules 表达式实时求值：优先级裁决、脉冲锁存与平滑衰减</text>
      <text y="84">• 125~250Hz 固定时钟调度 HID 报告与音圈触感输出</text>
    </g>
  </g>

  <!-- 连线 3 -> 4 (向左) -->
  <line x1="460" y1="510" x2="400" y2="510" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="405" y="482" width="50" height="20" rx="4" fill="#f1f3f5"/>
  <text class="tag-text" x="430" y="496">驱动下发</text>

  <!-- 卡片 4: DualSense 实体控制器 -->
  <g transform="translate(50, 400)" filter="url(#shadow)">
    <rect width="340" height="220" rx="14" fill="#f1f3f5" stroke="#343a40" stroke-width="2"/>
    <text class="card-title" x="170" y="36" text-anchor="middle" fill="#212529">DualSense 实体控制器</text>
    <text class="card-sub" x="170" y="56" text-anchor="middle">自适应阻力扳机 + 音圈高清触感</text>
    <line x1="25" y1="72" x2="315" y2="72" stroke="#ced4da" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• 自适应扳机：瞄准阻尼挡墙 + 开火强击退断点</text>
      <text y="28">• 音圈触感 (Ch3/4)：涉水微颤 + 金属空腔清脆回响</text>
      <text y="56">• 双马达震动：爆炸低频重击 + 枪机机械后坐力</text>
      <text y="84">• RGB 灯条：残血红光闪烁 + 充电冰蓝呼吸</text>
    </g>
  </g>

  <!-- ================= 第三行: 声明式联动配置中心 ================= -->

  <g transform="translate(50, 660)" filter="url(#red-shadow)">
    <rect width="860" height="185" rx="16" fill="#fff5f5" stroke="#e03131" stroke-width="2.5"/>
    <text class="card-title" x="430" y="35" text-anchor="middle" fill="#c92a2a" font-size="20">⚙️ profiles/audio-ai-demo.yaml 声明式联动规则</text>
    <text class="card-sub" x="430" y="56" text-anchor="middle" font-size="13.5">将模型识别出的高层语义事件，以毫秒级规则无缝映射为自适应阻力与振动纹理</text>
    <line x1="30" y1="72" x2="830" y2="72" stroke="#ffc9c9" stroke-width="1.5" stroke-dasharray="4,4"/>
    
    <g class="card-item" transform="translate(32, 102)" font-size="14.5">
      <text y="0">• <tspan font-weight="700">source 块</tspan>：声明 type: audio_ai、listen: 127.0.0.1:9998、signal_ttl: 600ms 超时失效安全保护</text>
      <text y="28">• <tspan font-weight="700">材质触感</tspan>：when: 'surface_water > 0.4' ──► 左右马达驱动 60Hz 细腻水波微颤 (sine modulate)</text>
      <text y="56">• <tspan font-weight="700">战况反馈</tspan>：when: 'audio_gunshot > 0.5' ──► 瞬间触发 hold: 120ms 右马达后坐力脉冲</text>
    </g>
  </g>

  <!-- 向上注入连线 -->
  <path d="M 685 660 L 685 628" fill="none" stroke="#e03131" stroke-width="2" stroke-dasharray="5,5" marker-end="url(#arrow-red)"/>
  <rect x="620" y="632" width="130" height="22" rx="4" fill="#ffe3e3" stroke="#ffa8a8"/>
  <text class="cfg-text" x="685" y="647">注入 YAML 映射策略</text>

</svg>
`;
}

function main() {
  const docsDir = path.resolve(__dirname, '../docs');
  if (!fs.existsSync(docsDir)) {
    fs.mkdirSync(docsDir, { recursive: true });
  }

  const excalidrawContent = JSON.stringify(buildExcalidrawJSON(), null, 2);
  const excalidrawPath = path.join(docsDir, 'audio-ai-detector.excalidraw');
  fs.writeFileSync(excalidrawPath, excalidrawContent, 'utf-8');
  console.log(`✓ 成功生成 Excalidraw 原文件: ${excalidrawPath}`);

  const svgContent = buildSVG();
  const svgPath = path.join(docsDir, 'audio-ai-detector.svg');
  fs.writeFileSync(svgPath, svgContent, 'utf-8');
  console.log(`✓ 成功生成 SVG 矢量图: ${svgPath}`);

  const pngPath = path.join(docsDir, 'audio-ai-detector.png');
  try {
    execSync(`rsvg-convert -w 2400 "${svgPath}" -o "${pngPath}"`);
    console.log(`✓ 成功导出高清 PNG: ${pngPath}`);
  } catch (err) {
    console.warn(`! PNG 导出失败: ${err.message}`);
  }
}

main();
