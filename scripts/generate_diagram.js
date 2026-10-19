const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

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
      lineHeight: 1.3
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

  // 顶部大标题
  addText({
    x: 180, y: 35, width: 800, height: 45,
    text: "dualsense-c 架构设计与数据流",
    fontSize: 32, strokeColor: "#1e1e1e", textAlign: "center"
  });
  addText({
    x: 230, y: 85, width: 700, height: 26,
    text: "PS5 DualSense 自适应扳机与力反馈四层驱动流水线",
    fontSize: 18, strokeColor: "#495057", textAlign: "center"
  });

  // 第一行 3 个卡片
  // 卡片 1: 游戏 / 输入源 (x: 50, y: 140, w: 320, h: 220)
  addRect({ x: 50, y: 140, width: 320, height: 220, backgroundColor: "#f8f9fa", strokeColor: "#868e96", strokeWidth: 2 });
  addText({ x: 60, y: 155, width: 300, height: 28, text: "游戏 / 输入源 (Inputs)", fontSize: 20, strokeColor: "#212529" });
  addText({ x: 75, y: 195, width: 280, height: 24, text: "• FPS 扳机物理行程驱动 (L2/R2)", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 225, width: 280, height: 24, text: "• 原生 UDP 遥测 (Forza / BeamNG / DiRT)", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 255, width: 280, height: 24, text: "• Proton 共享内存桥 (AC / ACC / AC Rally)", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 285, width: 280, height: 24, text: "• 离线仿真测试源 (sim / bridgesim)", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });

  // 箭头 1 -> 2
  addArrow({ startX: 370, startY: 250, points: [[0, 0], [60, 0]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 375, y: 220, width: 50, height: 22, text: "原始数据", fontSize: 13, strokeColor: "#495057" });

  // 卡片 2: ① Source 适配器 (x: 430, y: 140, w: 320, h: 220)
  addRect({ x: 430, y: 140, width: 320, height: 220, backgroundColor: "#ebfbee", strokeColor: "#0ca678", strokeWidth: 2 });
  addText({ x: 440, y: 155, width: 300, height: 28, text: "① Source 适配器", fontSize: 20, strokeColor: "#0ca678" });
  addText({ x: 440, y: 183, width: 300, height: 20, text: "internal/source", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 455, y: 210, width: 280, height: 24, text: "• 支持模式: trigger / udp / audio_ai / sim", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 455, y: 240, width: 280, height: 24, text: "• 协议包解析与动态字段映射", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 455, y: 270, width: 280, height: 24, text: "• 扳机行程按键采样与打时间戳", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 455, y: 300, width: 280, height: 24, text: "• 采样率控制与异步通道分发", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });

  // 箭头 2 -> 3
  addArrow({ startX: 750, startY: 250, points: [[0, 0], [60, 0]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 755, y: 220, width: 50, height: 22, text: "归一化", fontSize: 13, strokeColor: "#495057" });

  // 卡片 3: ② signal.Set (x: 810, y: 140, w: 320, h: 220)
  addRect({ x: 810, y: 140, width: 320, height: 220, backgroundColor: "#e7f5ff", strokeColor: "#1971c2", strokeWidth: 2 });
  addText({ x: 820, y: 155, width: 300, height: 28, text: "② signal.Set 信号集", fontSize: 20, strokeColor: "#1971c2" });
  addText({ x: 820, y: 183, width: 300, height: 20, text: "internal/signal", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 835, y: 210, width: 280, height: 24, text: "• 40+ 标准信号字典（归一化 0~1）", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 835, y: 240, width: 280, height: 24, text: "• deadzone 死区与 gamma 非线性", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 835, y: 270, width: 280, height: 24, text: "• slew rate 升降速率平滑限制", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 835, y: 300, width: 280, height: 24, text: "• signal_ttl 超时自动失效防锁死", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });

  // 第一行到第二行的拐角连线 (从 signal.Set 右侧折返向下到 rules 规则引擎)
  addArrow({
    startX: 970, startY: 360,
    points: [[0, 0], [0, 50]],
    strokeColor: "#495057", strokeWidth: 2
  });
  addText({ x: 980, y: 375, width: 90, height: 22, text: "标准信号流", fontSize: 13, strokeColor: "#495057", textAlign: "left" });

  // 第二行 3 个卡片
  // 卡片 4: ③ rules 规则引擎 (x: 810, y: 410, w: 320, h: 220)
  addRect({ x: 810, y: 410, width: 320, height: 220, backgroundColor: "#f3f0ff", strokeColor: "#7048e8", strokeWidth: 2 });
  addText({ x: 820, y: 425, width: 300, height: 28, text: "③ rules 规则引擎", fontSize: 20, strokeColor: "#7048e8" });
  addText({ x: 820, y: 453, width: 300, height: 20, text: "internal/rules", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 835, y: 480, width: 280, height: 24, text: "• Expr 表达式字节码实时求值", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 835, y: 510, width: 280, height: 24, text: "• 边沿按压检测 (pressed / released)", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 835, y: 540, width: 280, height: 24, text: "• priority 优先级整组覆盖仲裁", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 835, y: 570, width: 280, height: 24, text: "• 脉冲锁存 (hold) 与 平滑衰减 (release)", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });

  // 箭头 4 -> 5 (向左)
  addArrow({ startX: 810, startY: 520, points: [[0, 0], [-60, 0]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 755, y: 490, width: 50, height: 22, text: "效果指令", fontSize: 13, strokeColor: "#495057" });

  // 卡片 5: ④ sink 输出时钟 (x: 430, y: 410, w: 320, h: 220)
  addRect({ x: 430, y: 410, width: 320, height: 220, backgroundColor: "#fff4e6", strokeColor: "#e8590c", strokeWidth: 2 });
  addText({ x: 440, y: 425, width: 300, height: 28, text: "④ sink 输出时钟", fontSize: 20, strokeColor: "#e8590c" });
  addText({ x: 440, y: 453, width: 300, height: 20, text: "internal/sink", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 455, y: 480, width: 280, height: 24, text: "• 125 ~ 250 Hz 固定时钟 Tick 循环", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 455, y: 510, width: 280, height: 24, text: "• 组装 DualSense HID 输出报告", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 455, y: 540, width: 280, height: 24, text: "• 扳机 11 字节效果映射 (8 种模式)", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 455, y: 570, width: 280, height: 24, text: "• 热插拔恢复与退出零阻力安全复位", fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left" });

  // 箭头 5 -> 6 (向左)
  addArrow({ startX: 430, startY: 520, points: [[0, 0], [-60, 0]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 375, y: 490, width: 50, height: 22, text: "HID 报告", fontSize: 13, strokeColor: "#495057" });

  // 卡片 6: DualSense 控制器 (x: 50, y: 410, w: 320, h: 220)
  addRect({ x: 50, y: 410, width: 320, height: 220, backgroundColor: "#f1f3f5", strokeColor: "#343a40", strokeWidth: 2 });
  addText({ x: 60, y: 425, width: 300, height: 28, text: "DualSense 控制器", fontSize: 20, strokeColor: "#212529" });
  addText({ x: 60, y: 453, width: 300, height: 20, text: "PS5 Controller", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 75, y: 480, width: 280, height: 24, text: "• 自适应阻力扳机 (L2 / R2 挡墙与阻尼)", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 510, width: 280, height: 24, text: "• 双触觉震动马达 (Haptic / Rumble)", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 540, width: 280, height: 24, text: "• RGB 运行状态指示灯 (Lightbar)", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });
  addText({ x: 75, y: 570, width: 280, height: 24, text: "• 通道: Linux hidraw / Windows HID", fontSize: 15, strokeColor: "#343a40", textAlign: "left" });

  // 第三行: profiles/*.yaml 声明式配置中心 (x: 50, y: 670, w: 1080, h: 180)
  addRect({ x: 50, y: 670, width: 1080, height: 180, backgroundColor: "#fff5f5", strokeColor: "#e03131", strokeWidth: 2 });
  addText({ x: 100, y: 688, width: 980, height: 30, text: "⚙️ profiles/*.yaml 声明式游戏配置中心", fontSize: 22, strokeColor: "#c92a2a" });
  addText({ x: 100, y: 720, width: 980, height: 22, text: "游戏差异完全由 YAML 定义，无需修改或重新编译程序；支持离线静态编译校验与启动热加载", fontSize: 15, strokeColor: "#495057" });

  addText({ x: 80, y: 755, width: 1000, height: 24, text: "• source 块：声明输入源类型 (trigger / udp / audio_ai / sim)、监听端口、内置解码器与动态字段偏移", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 785, width: 1000, height: 24, text: "• shape 块：配置各信号的物理死区 (deadzone)、非线性响应曲线 (gamma) 以及上升/下降平滑速率 (slew rate)", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 815, width: 1000, height: 24, text: "• rules 块：声明激活条件 when (Expr 表达式)、目标部件、扳机模式 (rigid/feedback/weapon) 与优先级 priority", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });

  // 向上箭头 1: 配置中心指向 ① Source 适配器
  addArrow({
    startX: 590, startY: 670,
    points: [[0, 0], [0, -30], [-10, -30], [-10, -300]],
    strokeColor: "#e03131", strokeWidth: 2, strokeStyle: "dashed"
  });
  addText({ x: 510, y: 375, width: 140, height: 22, text: "① 注入 Source 规格", fontSize: 13, strokeColor: "#c92a2a" });

  // 向上箭头 2: 配置中心指向 ③ rules 规则引擎
  addArrow({
    startX: 950, startY: 670,
    points: [[0, 0], [0, -40]],
    strokeColor: "#e03131", strokeWidth: 2, strokeStyle: "dashed"
  });
  addText({ x: 960, y: 640, width: 150, height: 22, text: "② 注入触发规则与参数", fontSize: 13, strokeColor: "#c92a2a", textAlign: "left" });

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

function buildSVG() {
  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1180 890" width="100%" height="100%">
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
    <marker id="arrow-red" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#e03131" />
    </marker>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif; font-weight: 800; fill: #1e1e1e; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 500; fill: #495057; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 19px; }
      .card-sub { font-family: "SF Mono", "Consolas", Menlo, monospace; font-size: 13px; fill: #868e96; }
      .card-item { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 14.5px; fill: #2b2b2b; }
      .tag-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13px; font-weight: 600; fill: #495057; text-anchor: middle; }
      .cfg-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13px; font-weight: 700; fill: #c92a2a; text-anchor: middle; }
    </style>
  </defs>

  <rect width="100%" height="100%" fill="#ffffff" rx="16"/>

  <!-- 顶部主副标题 -->
  <g transform="translate(590, 50)" text-anchor="middle">
    <text class="title" font-size="30" y="0">dualsense-c 架构设计与数据流</text>
    <text class="subtitle" font-size="16" y="32">PS5 DualSense 自适应扳机与力反馈四层驱动流水线</text>
  </g>

  <!-- ================= 第一行流水线 (从左到右) ================= -->

  <!-- 卡片 1: 游戏 / 输入源 -->
  <g transform="translate(50, 130)" filter="url(#shadow)">
    <rect width="320" height="220" rx="14" fill="#f8f9fa" stroke="#adb5bd" stroke-width="2"/>
    <text class="card-title" x="160" y="36" text-anchor="middle" fill="#212529">游戏 / 输入源</text>
    <text class="card-sub" x="160" y="56" text-anchor="middle">Game &amp; Physical Inputs</text>
    <line x1="25" y1="72" x2="295" y2="72" stroke="#dee2e6" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• FPS 扳机物理行程驱动 (L2/R2)</text>
      <text y="28">• 原生 UDP 遥测 (Forza / BeamNG)</text>
      <text y="56">• Proton 共享内存 (AC / ACC / ACR)</text>
      <text y="84">• 仿真测试数据源 (sim / bridgesim)</text>
    </g>
  </g>

  <!-- 箭头 1 -> 2 -->
  <line x1="370" y1="240" x2="420" y2="240" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="375" y="212" width="50" height="20" rx="4" fill="#f1f3f5"/>
  <text class="tag-text" x="400" y="226">原始输入</text>

  <!-- 卡片 2: ① Source 适配器 -->
  <g transform="translate(430, 130)" filter="url(#shadow)">
    <rect width="320" height="220" rx="14" fill="#ebfbee" stroke="#0ca678" stroke-width="2"/>
    <text class="card-title" x="160" y="36" text-anchor="middle" fill="#0ca678">① Source 适配器</text>
    <text class="card-sub" x="160" y="56" text-anchor="middle">internal/source</text>
    <line x1="25" y1="72" x2="295" y2="72" stroke="#b2f2bb" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• 支持类型: trigger / udp / audio_ai / sim</text>
      <text y="28">• 原始遥测协议解码与动态字段解析</text>
      <text y="56">• 扳机按键行程采样与时间戳记录</text>
      <text y="84">• 数据刷新率控制与异常容错保护</text>
    </g>
  </g>

  <!-- 箭头 2 -> 3 -->
  <line x1="750" y1="240" x2="800" y2="240" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="755" y="212" width="50" height="20" rx="4" fill="#f1f3f5"/>
  <text class="tag-text" x="780" y="226">归一化</text>

  <!-- 卡片 3: ② signal.Set -->
  <g transform="translate(810, 130)" filter="url(#shadow)">
    <rect width="320" height="220" rx="14" fill="#e7f5ff" stroke="#1971c2" stroke-width="2"/>
    <text class="card-title" x="160" y="36" text-anchor="middle" fill="#1971c2">② signal.Set 信号集</text>
    <text class="card-sub" x="160" y="56" text-anchor="middle">internal/signal</text>
    <line x1="25" y1="72" x2="295" y2="72" stroke="#a5d8ff" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• 40+ 标准信号字典（统一归一至 0~1）</text>
      <text y="28">• deadzone 死区与 gamma 非线性滤波</text>
      <text y="56">• slew rate 升降速率平滑限制</text>
      <text y="84">• signal_ttl 超时自动失效防锁死</text>
    </g>
  </g>

  <!-- 第一行到第二行折返连线 -->
  <path d="M 970 350 L 970 395" fill="none" stroke="#495057" stroke-width="2.5" marker-end="url(#arrow-gray)"/>
  <rect x="980" y="362" width="80" height="22" rx="4" fill="#e7f5ff" stroke="#a5d8ff"/>
  <text class="tag-text" x="1020" y="377" fill="#1971c2">标准信号流</text>

  <!-- ================= 第二行流水线 (从右到左) ================= -->

  <!-- 卡片 4: ③ rules 规则引擎 -->
  <g transform="translate(810, 400)" filter="url(#shadow)">
    <rect width="320" height="220" rx="14" fill="#f3f0ff" stroke="#7048e8" stroke-width="2"/>
    <text class="card-title" x="160" y="36" text-anchor="middle" fill="#7048e8">③ rules 规则引擎</text>
    <text class="card-sub" x="160" y="56" text-anchor="middle">internal/rules</text>
    <line x1="25" y1="72" x2="295" y2="72" stroke="#d0bfff" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• Expr 表达式实时求值 (when 条件)</text>
      <text y="28">• 边沿按压判定 (pressed / released)</text>
      <text y="56">• priority 优先级整组覆盖仲裁</text>
      <text y="84">• 脉冲锁存 (hold) 与 平滑衰减 (release)</text>
    </g>
  </g>

  <!-- 箭头 4 -> 5 (向左) -->
  <line x1="810" y1="510" x2="760" y2="510" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="765" y="482" width="50" height="20" rx="4" fill="#f1f3f5"/>
  <text class="tag-text" x="790" y="496">效果指令</text>

  <!-- 卡片 5: ④ sink 输出时钟 -->
  <g transform="translate(430, 400)" filter="url(#shadow)">
    <rect width="320" height="220" rx="14" fill="#fff4e6" stroke="#e8590c" stroke-width="2"/>
    <text class="card-title" x="160" y="36" text-anchor="middle" fill="#e8590c">④ sink 输出时钟</text>
    <text class="card-sub" x="160" y="56" text-anchor="middle">internal/sink</text>
    <line x1="25" y1="72" x2="295" y2="72" stroke="#ffc078" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• 125 ~ 250 Hz 固定时钟 Tick 循环</text>
      <text y="28">• 组装 DualSense HID 硬件输出报告</text>
      <text y="56">• 11 字节扳机模式映射 (8 种手感模式)</text>
      <text y="84">• 设备热插拔管理与退出零阻力复位</text>
    </g>
  </g>

  <!-- 箭头 5 -> 6 (向左) -->
  <line x1="430" y1="510" x2="380" y2="510" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="385" y="482" width="50" height="20" rx="4" fill="#f1f3f5"/>
  <text class="tag-text" x="410" y="496">HID 报告</text>

  <!-- 卡片 6: DualSense 控制器 -->
  <g transform="translate(50, 400)" filter="url(#shadow)">
    <rect width="320" height="220" rx="14" fill="#f1f3f5" stroke="#343a40" stroke-width="2"/>
    <text class="card-title" x="160" y="36" text-anchor="middle" fill="#212529">DualSense 控制器</text>
    <text class="card-sub" x="160" y="56" text-anchor="middle">PS5 Physical Hardware</text>
    <line x1="25" y1="72" x2="295" y2="72" stroke="#ced4da" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-item" transform="translate(24, 102)">
      <text y="0">• 自适应扳机阻尼 (L2/R2 挡墙与震颤)</text>
      <text y="28">• 左右触觉震动马达 (Haptic / Rumble)</text>
      <text y="56">• RGB 运行状态指示灯 (Lightbar)</text>
      <text y="84">• Linux /dev/hidraw 与 Windows HID 接口</text>
    </g>
  </g>

  <!-- ================= 第三行: 声明式配置中心 ================= -->

  <g transform="translate(50, 660)" filter="url(#red-shadow)">
    <rect width="1080" height="190" rx="16" fill="#fff5f5" stroke="#e03131" stroke-width="2.5"/>
    <text class="card-title" x="540" y="36" text-anchor="middle" fill="#c92a2a" font-size="22">⚙️ profiles/*.yaml（声明式游戏配置中心）</text>
    <text class="card-sub" x="540" y="58" text-anchor="middle" font-size="14">游戏差异与控制策略完全由 YAML 定义，零代码修改；支持启动期静态校验与热加载</text>
    <line x1="30" y1="74" x2="1050" y2="74" stroke="#ffc9c9" stroke-width="1.5" stroke-dasharray="4,4"/>
    
    <g class="card-item" transform="translate(35, 104)" font-size="15">
      <text y="0">• <tspan font-weight="700">source 块</tspan>：声明输入源类型 (trigger / udp / audio_ai / sim)、监听端口、内置解码器与动态字段偏移</text>
      <text y="28">• <tspan font-weight="700">shape 块</tspan>：配置各物理信号的死区 (deadzone)、非线性响应曲线 (gamma) 以及上升/下降平滑速率 (slew rate)</text>
      <text y="56">• <tspan font-weight="700">rules 块</tspan>：声明触发条件 when (Expr 表达式)、目标部件、扳机模式 (rigid / feedback / weapon) 与优先级 priority</text>
    </g>
  </g>

  <!-- 配置中心向上连线 1 (至 Source) -->
  <path d="M 590 660 L 590 358" fill="none" stroke="#e03131" stroke-width="2" stroke-dasharray="5,5" marker-end="url(#arrow-red)"/>
  <rect x="525" y="362" width="130" height="22" rx="4" fill="#ffe3e3" stroke="#ffa8a8"/>
  <text class="cfg-text" x="590" y="377">① 注入数据源规格</text>

  <!-- 配置中心向上连线 2 (至 Rules) -->
  <path d="M 970 660 L 970 628" fill="none" stroke="#e03131" stroke-width="2" stroke-dasharray="5,5" marker-end="url(#arrow-red)"/>
  <rect x="905" y="632" width="130" height="22" rx="4" fill="#ffe3e3" stroke="#ffa8a8"/>
  <text class="cfg-text" x="970" y="647">② 注入规则与手感</text>

</svg>
`;
}

function main() {
  const docsDir = path.resolve(__dirname, '../docs');
  if (!fs.existsSync(docsDir)) {
    fs.mkdirSync(docsDir, { recursive: true });
  }

  const excalidrawContent = JSON.stringify(buildExcalidrawJSON(), null, 2);
  const excalidrawPath = path.join(docsDir, 'architecture.excalidraw');
  fs.writeFileSync(excalidrawPath, excalidrawContent, 'utf-8');
  console.log(`✓ 成功更新 Excalidraw 原文件: ${excalidrawPath}`);

  const svgContent = buildSVG();
  const svgPath = path.join(docsDir, 'architecture.svg');
  fs.writeFileSync(svgPath, svgContent, 'utf-8');
  console.log(`✓ 成功更新 SVG 矢量图: ${svgPath}`);

  const pngPath = path.join(docsDir, 'architecture.png');
  try {
    execSync(`rsvg-convert -w 2400 "${svgPath}" -o "${pngPath}"`);
    console.log(`✓ 成功导出高清 PNG: ${pngPath}`);
  } catch (err) {
    console.warn(`! PNG 导出失败: ${err.message}`);
  }
}

main();
