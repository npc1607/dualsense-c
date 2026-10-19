const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

// 1. 生成标准的 Excalidraw JSON 数据
function buildExcalidrawJSON() {
  let idCounter = 1;
  const genId = (prefix) => `${prefix}_${idCounter++}_${Math.random().toString(36).substr(2, 6)}`;

  const elements = [];

  function addRect({ id, x, y, width, height, strokeColor = '#1e1e1e', backgroundColor = '#ffffff', fillStyle = 'solid', strokeWidth = 2, strokeStyle = 'solid', roughness = 1, roundness = { type: 3 }, opacity = 100 }) {
    elements.push({
      id: id || genId('rect'),
      type: 'rectangle',
      x, y, width, height,
      angle: 0,
      strokeColor,
      backgroundColor,
      fillStyle,
      strokeWidth,
      strokeStyle,
      roughness,
      opacity,
      groupIds: [],
      frameId: null,
      roundness,
      seed: Math.floor(Math.random() * 1000000),
      version: 1,
      versionNonce: 1,
      isDeleted: false,
      boundElements: null,
      updated: Date.now(),
      link: null,
      locked: false
    });
  }

  function addText({ id, x, y, width, height, text, fontSize = 16, fontFamily = 1, textAlign = 'center', verticalAlign = 'middle', strokeColor = '#1e1e1e', opacity = 100 }) {
    elements.push({
      id: id || genId('text'),
      type: 'text',
      x, y, width, height,
      angle: 0,
      strokeColor,
      backgroundColor: 'transparent',
      fillStyle: 'solid',
      strokeWidth: 1,
      strokeStyle: 'solid',
      roughness: 1,
      opacity,
      groupIds: [],
      frameId: null,
      roundness: null,
      seed: Math.floor(Math.random() * 1000000),
      version: 1,
      versionNonce: 1,
      isDeleted: false,
      boundElements: null,
      updated: Date.now(),
      link: null,
      locked: false,
      text,
      fontSize,
      fontFamily,
      textAlign,
      verticalAlign,
      containerId: null,
      originalText: text,
      lineHeight: 1.25
    });
  }

  function addArrow({ id, startX, startY, points, strokeColor = '#1e1e1e', strokeWidth = 2, strokeStyle = 'solid', roughness = 1 }) {
    const endPoint = points[points.length - 1];
    elements.push({
      id: id || genId('arrow'),
      type: 'arrow',
      x: startX,
      y: startY,
      width: Math.abs(endPoint[0]),
      height: Math.abs(endPoint[1]),
      angle: 0,
      strokeColor,
      backgroundColor: 'transparent',
      fillStyle: 'solid',
      strokeWidth,
      strokeStyle,
      roughness,
      opacity: 100,
      groupIds: [],
      frameId: null,
      roundness: { type: 2 },
      seed: Math.floor(Math.random() * 1000000),
      version: 1,
      versionNonce: 1,
      isDeleted: false,
      points,
      startBinding: null,
      endBinding: null,
      startArrowhead: null,
      endArrowhead: 'arrow'
    });
  }

  // 大标题区
  addText({
    x: 350, y: 35, width: 800, height: 40,
    text: "dualsense-c 架构设计与数据流",
    fontSize: 28, strokeColor: "#1e1e1e", textAlign: "center"
  });
  addText({
    x: 400, y: 80, width: 700, height: 24,
    text: "PS5 DualSense 自适应扳机与力反馈四层驱动流水线",
    fontSize: 16, strokeColor: "#495057", textAlign: "center"
  });

  // 流水线卡片坐标定义
  const cards = [
    {
      title: "游戏 / 输入源",
      sub: "Game & Inputs",
      x: 40, y: 150, w: 180, h: 260,
      bg: "#f8f9fa", stroke: "#495057",
      items: [
        "FPS 扳机输入 (L2/R2)",
        "UDP 遥测 (Forza/BeamNG)",
        "Proton 共享内存 (AC/ACC)",
        "虚拟仿真源 (sim)"
      ]
    },
    {
      title: "① Source 适配器",
      sub: "internal/source",
      x: 280, y: 150, w: 200, h: 260,
      bg: "#ebfbee", stroke: "#0ca678",
      items: [
        "类型: trigger | udp | sim",
        "原始遥测协议解析",
        "按键行程与按压提取",
        "打时间戳与频率控制"
      ]
    },
    {
      title: "② signal.Set",
      sub: "internal/signal",
      x: 540, y: 150, w: 200, h: 260,
      bg: "#e7f5ff", stroke: "#1971c2",
      items: [
        "标准信号字典 (40+ 项)",
        "归一化处理 [0, 1]",
        "滤波: deadzone & gamma",
        "变化率限制 (slew rate)",
        "超时失效判定 (signal_ttl)"
      ]
    },
    {
      title: "③ rules 规则引擎",
      sub: "internal/rules",
      x: 800, y: 150, w: 210, h: 260,
      bg: "#f3f0ff", stroke: "#7048e8",
      items: [
        "Expr 表达式编译求值",
        "边沿检测 (pressed/released)",
        "优先级仲裁 (priority 覆盖)",
        "脉冲锁存 (hold)",
        "平滑释放衰减 (release)"
      ]
    },
    {
      title: "④ sink 输出时钟",
      sub: "internal/sink",
      x: 1070, y: 150, w: 200, h: 260,
      bg: "#fff4e6", stroke: "#e8590c",
      items: [
        "125 ~ 250 Hz 固定 Tick",
        "组装 DualSense HID 报告",
        "扳机 11 字节效果映射",
        "马达正弦/方波调幅",
        "热插拔与安全退出复位"
      ]
    },
    {
      title: "DualSense 控制器",
      sub: "PS5 Controller",
      x: 1330, y: 150, w: 180, h: 260,
      bg: "#f1f3f5", stroke: "#212529",
      items: [
        "自适应扳机 (L2 / R2)",
        "音圈双震动马达",
        "RGB 状态指示灯条",
        "Linux /dev/hidraw",
        "Windows Win32 HID"
      ]
    }
  ];

  // 绘制各个卡片
  cards.forEach(c => {
    addRect({
      x: c.x, y: c.y, width: c.w, height: c.h,
      backgroundColor: c.bg, strokeColor: c.stroke,
      strokeWidth: 2, fillStyle: 'solid', roughness: 1, roundness: { type: 3 }
    });
    // 标题
    addText({
      x: c.x + 5, y: c.y + 15, width: c.w - 10, height: 24,
      text: c.title, fontSize: 17, strokeColor: c.stroke, textAlign: 'center'
    });
    // 子标题
    addText({
      x: c.x + 5, y: c.y + 40, width: c.w - 10, height: 18,
      text: c.sub, fontSize: 12, strokeColor: '#868e96', textAlign: 'center'
    });
    // 分割线
    elements.push({
      id: genId('line'),
      type: 'line',
      x: c.x + 15, y: c.y + 65,
      width: c.w - 30, height: 0,
      angle: 0,
      strokeColor: c.stroke,
      backgroundColor: 'transparent',
      fillStyle: 'solid',
      strokeWidth: 1,
      strokeStyle: 'dashed',
      roughness: 1,
      opacity: 60,
      points: [[0, 0], [c.w - 30, 0]],
      seed: Math.floor(Math.random() * 1000000),
      version: 1, versionNonce: 1, isDeleted: false
    });
    // 列表条目
    let curY = c.y + 75;
    c.items.forEach(item => {
      addText({
        x: c.x + 12, y: curY, width: c.w - 24, height: 22,
        text: `• ${item}`, fontSize: 12, strokeColor: '#343a40', textAlign: 'left'
      });
      curY += 28;
    });
  });

  // 主流水线横向箭头与标签
  const arrows = [
    { startX: 220, startY: 280, dx: 60, dy: 0, label: "输入流", lx: 225, ly: 250 },
    { startX: 480, startY: 280, dx: 60, dy: 0, label: "归一化", lx: 485, ly: 250 },
    { startX: 740, startY: 280, dx: 60, dy: 0, label: "标准信号", lx: 742, ly: 250 },
    { startX: 1010, startY: 280, dx: 60, dy: 0, label: "输出规则", lx: 1012, ly: 250 },
    { startX: 1270, startY: 280, dx: 60, dy: 0, label: "HID 报告", lx: 1272, ly: 250 }
  ];

  arrows.forEach(a => {
    addArrow({
      startX: a.startX, startY: a.startY,
      points: [[0, 0], [a.dx, a.dy]],
      strokeColor: '#495057', strokeWidth: 2
    });
    addText({
      x: a.lx, y: a.ly, width: 50, height: 18,
      text: a.label, fontSize: 11, strokeColor: '#495057', textAlign: 'center'
    });
  });

  // 底部配置卡片 (profiles/*.yaml)
  const confX = 400, confY = 480, confW = 680, confH = 150;
  addRect({
    x: confX, y: confY, width: confW, height: confH,
    backgroundColor: '#fff5f5', strokeColor: '#e03131',
    strokeWidth: 2, fillStyle: 'solid', roughness: 1, roundness: { type: 3 }, strokeStyle: 'solid'
  });
  addText({
    x: confX + 10, y: confY + 15, width: confW - 20, height: 26,
    text: "profiles/*.yaml (声明式游戏配置)", fontSize: 18, strokeColor: "#c92a2a", textAlign: "center"
  });
  addText({
    x: confX + 10, y: confY + 45, width: confW - 20, height: 18,
    text: "定义数据源参数、信号滤波曲线、触发条件表达式与力反馈效果", fontSize: 13, strokeColor: "#868e96", textAlign: "center"
  });
  addText({
    x: confX + 40, y: confY + 75, width: confW - 80, height: 20,
    text: "• Source 配置：数据源类型 (trigger/udp/sim)、监听端口、解码器、动态字段偏移", fontSize: 12, strokeColor: "#2b2b2b", textAlign: "left"
  });
  addText({
    x: confX + 40, y: confY + 98, width: confW - 80, height: 20,
    text: "• Shape 整形：死区 (deadzone)、gamma 非线性曲线、升降 slew rate 平滑", fontSize: 12, strokeColor: "#2b2b2b", textAlign: "left"
  });
  addText({
    x: confX + 40, y: confY + 121, width: confW - 80, height: 20,
    text: "• Rules 映射：触发条件 when、目标通道、模式 (rigid/weapon/feedback...)、优先级 priority", fontSize: 12, strokeColor: "#2b2b2b", textAlign: "left"
  });

  // 向上箭头：从配置文件分别指入 Source 适配器与 Rules 引擎
  // 箭头 1：指向 Source
  addArrow({
    startX: 520, startY: 480,
    points: [[0, 0], [-140, -70]],
    strokeColor: '#e03131', strokeWidth: 2, strokeStyle: 'dashed'
  });
  addText({
    x: 340, y: 440, width: 140, height: 18,
    text: "① 注入 Source 规格", fontSize: 11, strokeColor: "#c92a2a", textAlign: "center"
  });

  // 箭头 2：指向 Rules
  addArrow({
    startX: 910, startY: 480,
    points: [[0, 0], [0, -70]],
    strokeColor: '#e03131', strokeWidth: 2, strokeStyle: 'dashed'
  });
  addText({
    x: 880, y: 440, width: 140, height: 18,
    text: "② 注入规则与手感参数", fontSize: 11, strokeColor: "#c92a2a", textAlign: "center"
  });

  return {
    type: "excalidraw",
    version: 2,
    source: "https://excalidraw.com",
    elements,
    appState: {
      gridSize: null,
      viewBackgroundColor: "#ffffff"
    },
    files: {}
  };
}

// 2. 生成现代化 Excalidraw 质感的高保真 SVG
function buildSVG() {
  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1560 670" width="100%" height="100%">
  <defs>
    <!-- 阴影与手绘风格滤镜 -->
    <filter id="card-shadow" x="-4%" y="-4%" width="108%" height="108%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.06"/>
    </filter>
    <filter id="accent-shadow" x="-5%" y="-5%" width="110%" height="110%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="6" stdDeviation="8" flood-color="#e03131" flood-opacity="0.12"/>
    </filter>
    <marker id="arrow-gray" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
      <path d="M 0 1 L 7 4 L 0 7 Z" fill="#495057" />
    </marker>
    <marker id="arrow-red" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
      <path d="M 0 1 L 7 4 L 0 7 Z" fill="#e03131" />
    </marker>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif; font-weight: 700; fill: #1e1e1e; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", sans-serif; font-weight: 500; fill: #495057; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 16px; }
      .card-sub { font-family: "SF Mono", "Consolas", Menlo, monospace; font-size: 11px; fill: #868e96; }
      .card-body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 12px; fill: #343a40; }
      .flow-tag { font-family: "SF Mono", "Consolas", Menlo, monospace; font-size: 11px; fill: #495057; font-weight: 600; text-anchor: middle; }
      .cfg-tag { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 11px; font-weight: 600; fill: #c92a2a; text-anchor: middle; }
    </style>
  </defs>

  <!-- 背景底色 -->
  <rect width="100%" height="100%" fill="#ffffff" rx="16"/>

  <!-- 顶部标题区域 -->
  <g transform="translate(780, 50)" text-anchor="middle">
    <text class="title" font-size="28" y="0">dualsense-c 架构设计与数据流</text>
    <text class="subtitle" font-size="15" y="30">PS5 DualSense 自适应扳机与力反馈四层驱动流水线</text>
  </g>

  <!-- ================= 主流水线卡片 ================= -->

  <!-- 卡片 0: 游戏 / 输入源 -->
  <g transform="translate(40, 140)" filter="url(#card-shadow)">
    <rect width="180" height="260" rx="12" fill="#f8f9fa" stroke="#adb5bd" stroke-width="2"/>
    <text class="card-title" x="90" y="35" text-anchor="middle" fill="#212529">游戏 / 输入源</text>
    <text class="card-sub" x="90" y="55" text-anchor="middle">Game &amp; Inputs</text>
    <line x1="20" y1="70" x2="160" y2="70" stroke="#dee2e6" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(16, 95)">
      <text y="0">• FPS 扳机行程驱动</text>
      <text y="28">• UDP 原生遥测</text>
      <text y="46" font-size="11" fill="#868e96" x="12">Forza / BeamNG / DiRT</text>
      <text y="74">• Proton 共享内存</text>
      <text y="92" font-size="11" fill="#868e96" x="12">AC / ACC / AC Rally</text>
      <text y="120">• 仿真模拟器 (sim)</text>
    </g>
  </g>

  <!-- 连线 0 -> 1 -->
  <line x1="220" y1="270" x2="274" y2="270" stroke="#495057" stroke-width="2" marker-end="url(#arrow-gray)"/>
  <text class="flow-tag" x="247" y="258">原始输入</text>

  <!-- 卡片 1: Source 适配器 -->
  <g transform="translate(280, 140)" filter="url(#card-shadow)">
    <rect width="200" height="260" rx="12" fill="#ebfbee" stroke="#0ca678" stroke-width="2"/>
    <text class="card-title" x="100" y="35" text-anchor="middle" fill="#0ca678">① Source 适配器</text>
    <text class="card-sub" x="100" y="55" text-anchor="middle">internal/source</text>
    <line x1="20" y1="70" x2="180" y2="70" stroke="#b2f2bb" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(18, 98)">
      <text y="0">• 类型: trigger / udp / sim</text>
      <text y="28">• 原始遥测协议解码</text>
      <text y="56">• 扳机按键行程提取</text>
      <text y="84">• 打时间戳与更新追踪</text>
      <text y="112">• 异步采样与限速</text>
    </g>
  </g>

  <!-- 连线 1 -> 2 -->
  <line x1="480" y1="270" x2="534" y2="270" stroke="#495057" stroke-width="2" marker-end="url(#arrow-gray)"/>
  <text class="flow-tag" x="507" y="258">归一化</text>

  <!-- 卡片 2: signal.Set -->
  <g transform="translate(540, 140)" filter="url(#card-shadow)">
    <rect width="200" height="260" rx="12" fill="#e7f5ff" stroke="#1971c2" stroke-width="2"/>
    <text class="card-title" x="100" y="35" text-anchor="middle" fill="#1971c2">② signal.Set</text>
    <text class="card-sub" x="100" y="55" text-anchor="middle">internal/signal</text>
    <line x1="20" y1="70" x2="180" y2="70" stroke="#a5d8ff" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(18, 98)">
      <text y="0">• 标准信号字典 (40+ 项)</text>
      <text y="28">• 统一归一化 [0, 1]</text>
      <text y="56">• deadzone / gamma 整形</text>
      <text y="84">• slew rate 升降限速</text>
      <text y="112">• signal_ttl 超时失效</text>
    </g>
  </g>

  <!-- 连线 2 -> 3 -->
  <line x1="740" y1="270" x2="794" y2="270" stroke="#495057" stroke-width="2" marker-end="url(#arrow-gray)"/>
  <text class="flow-tag" x="767" y="258">标准信号</text>

  <!-- 卡片 3: rules 规则引擎 -->
  <g transform="translate(800, 140)" filter="url(#card-shadow)">
    <rect width="210" height="260" rx="12" fill="#f3f0ff" stroke="#7048e8" stroke-width="2"/>
    <text class="card-title" x="105" y="35" text-anchor="middle" fill="#7048e8">③ rules 规则引擎</text>
    <text class="card-sub" x="105" y="55" text-anchor="middle">internal/rules</text>
    <line x1="20" y1="70" x2="190" y2="70" stroke="#d0bfff" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(18, 98)">
      <text y="0">• Expr 字节码实时求值</text>
      <text y="28">• 边沿检测 (pressed/held)</text>
      <text y="56">• priority 优先级裁决</text>
      <text y="84">• 脉冲锁存 (hold 保持)</text>
      <text y="112">• 平滑释放 (release 衰减)</text>
    </g>
  </g>

  <!-- 连线 3 -> 4 -->
  <line x1="1010" y1="270" x2="1064" y2="270" stroke="#495057" stroke-width="2" marker-end="url(#arrow-gray)"/>
  <text class="flow-tag" x="1037" y="258">效果输出</text>

  <!-- 卡片 4: sink 固定时钟 -->
  <g transform="translate(1070, 140)" filter="url(#card-shadow)">
    <rect width="200" height="260" rx="12" fill="#fff4e6" stroke="#e8590c" stroke-width="2"/>
    <text class="card-title" x="100" y="35" text-anchor="middle" fill="#e8590c">④ sink 输出时钟</text>
    <text class="card-sub" x="100" y="55" text-anchor="middle">internal/sink</text>
    <line x1="20" y1="70" x2="180" y2="70" stroke="#ffc078" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(18, 98)">
      <text y="0">• 125 ~ 250 Hz Tick 驱动</text>
      <text y="28">• 组装 DualSense HID 报告</text>
      <text y="56">• 扳机 11B 效果与参数</text>
      <text y="84">• 马达正弦/方波调幅</text>
      <text y="112">• 设备热插拔 / 退出安全重置</text>
    </g>
  </g>

  <!-- 连线 4 -> 5 -->
  <line x1="1270" y1="270" x2="1324" y2="270" stroke="#495057" stroke-width="2" marker-end="url(#arrow-gray)"/>
  <text class="flow-tag" x="1297" y="258">HID 报告</text>

  <!-- 卡片 5: DualSense 硬件 -->
  <g transform="translate(1330, 140)" filter="url(#card-shadow)">
    <rect width="180" height="260" rx="12" fill="#f1f3f5" stroke="#343a40" stroke-width="2"/>
    <text class="card-title" x="90" y="35" text-anchor="middle" fill="#212529">DualSense</text>
    <text class="card-sub" x="90" y="55" text-anchor="middle">PS5 Controller</text>
    <line x1="20" y1="70" x2="160" y2="70" stroke="#ced4da" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(18, 98)">
      <text y="0">• 自适应扳机 (L2 / R2)</text>
      <text y="28">• 左右触觉震动马达</text>
      <text y="56">• RGB 状态指示灯条</text>
      <text y="84">• Linux /dev/hidraw</text>
      <text y="112">• Windows Win32 HID</text>
    </g>
  </g>

  <!-- ================= 底部配置驱动区 ================= -->

  <g transform="translate(380, 470)" filter="url(#accent-shadow)">
    <rect width="760" height="150" rx="14" fill="#fff5f5" stroke="#e03131" stroke-width="2"/>
    <text class="card-title" x="380" y="32" text-anchor="middle" fill="#c92a2a" font-size="17">profiles/*.yaml（声明式游戏配置）</text>
    <text class="card-sub" x="380" y="52" text-anchor="middle">解耦游戏特性与手柄硬件控制，支持热拔插切换与离线校验</text>
    <line x1="30" y1="64" x2="730" y2="64" stroke="#ffc9c9" stroke-width="1.5" stroke-dasharray="4,4"/>
    
    <g class="card-body" transform="translate(30, 84)">
      <text y="0">• <tspan font-weight="700">source 块</tspan>：声明输入源类型 (trigger / udp / sim)、监听端口、解码器 (forza / outgauge / dirt) 与字段动态偏移</text>
      <text y="24">• <tspan font-weight="700">shape 块</tspan>：设定油门/刹车/方向盘的死区 (deadzone)、非线性响应 (gamma) 以及升降变化率 (slew rate)</text>
      <text y="48">• <tspan font-weight="700">rules 块</tspan>：声明触发条件 when (Expr 表达式)、目标部件、扳机模式 (rigid / feedback / weapon) 与优先级 priority</text>
    </g>
  </g>

  <!-- 配置流向上连接线 -->
  <!-- 箭头 1: 注入 Source -->
  <path d="M 540 470 L 540 435 Q 540 420 515 415 L 430 404" fill="none" stroke="#e03131" stroke-width="2" stroke-dasharray="5,5" marker-end="url(#arrow-red)"/>
  <rect x="420" y="435" width="130" height="22" rx="4" fill="#ffe3e3" stroke="#ffa8a8" stroke-width="1"/>
  <text class="cfg-tag" x="485" y="450">① 注入数据源规格</text>

  <!-- 箭头 2: 注入 Rules -->
  <path d="M 980 470 L 980 404" fill="none" stroke="#e03131" stroke-width="2" stroke-dasharray="5,5" marker-end="url(#arrow-red)"/>
  <rect x="915" y="425" width="130" height="22" rx="4" fill="#ffe3e3" stroke="#ffa8a8" stroke-width="1"/>
  <text class="cfg-tag" x="980" y="440">② 注入规则与手感</text>

</svg>
`;
}

function main() {
  const docsDir = path.resolve(__dirname, '../docs');
  if (!fs.existsSync(docsDir)) {
    fs.mkdirSync(docsDir, { recursive: true });
  }

  // 1. 写出 .excalidraw
  const excalidrawContent = JSON.stringify(buildExcalidrawJSON(), null, 2);
  const excalidrawPath = path.join(docsDir, 'architecture.excalidraw');
  fs.writeFileSync(excalidrawPath, excalidrawContent, 'utf-8');
  console.log(`✓ 成功生成 Excalidraw 原文件: ${excalidrawPath}`);

  // 2. 写出 .svg
  const svgContent = buildSVG();
  const svgPath = path.join(docsDir, 'architecture.svg');
  fs.writeFileSync(svgPath, svgContent, 'utf-8');
  console.log(`✓ 成功生成 SVG 矢量图: ${svgPath}`);

  // 3. 用 rsvg-convert 转换为 PNG 高清图
  const pngPath = path.join(docsDir, 'architecture.png');
  try {
    execSync(`rsvg-convert -w 2400 "${svgPath}" -o "${pngPath}"`);
    console.log(`✓ 成功生成高清 PNG 图: ${pngPath}`);
  } catch (err) {
    console.warn(`! rsvg-convert 失败或未安装，已跳过 PNG 导出: ${err.message}`);
  }
}

main();
