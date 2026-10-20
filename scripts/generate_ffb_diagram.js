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
      roughness: 1, opacity: 100,
      groupIds: [], frameId: null, roundness: { type: 3 },
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
    x: 180, y: 35, width: 800, height: 45,
    text: "力反馈镜像与虚拟手柄架构",
    fontSize: 30, strokeColor: "#1e1e1e", textAlign: "center"
  });
  addText({
    x: 180, y: 85, width: 800, height: 26,
    text: "ds5vpad 虚拟手柄独占注入与游戏力反馈 (FF_RUMBLE) 回传驱动闭环",
    fontSize: 16, strokeColor: "#495057", textAlign: "center"
  });

  // 第一行: 输入链路 (从左到右: 物理手柄 -> ds5vpad -> 虚拟手柄 -> 游戏)
  // 卡片 1: 物理 DualSense (x: 50, y: 140, w: 230, h: 220)
  addRect({ x: 50, y: 140, width: 230, height: 220, backgroundColor: "#f1f3f5", strokeColor: "#343a40", strokeWidth: 2 });
  addText({ x: 60, y: 155, width: 210, height: 28, text: "物理 DualSense 手柄", fontSize: 18, strokeColor: "#212529" });
  addText({ x: 60, y: 183, width: 210, height: 20, text: "/dev/input/eventX", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 70, y: 215, width: 190, height: 24, text: "• 物理按键与双摇杆输入", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 70, y: 245, width: 190, height: 24, text: "• L2/R2 模拟行程采样", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 70, y: 275, width: 190, height: 24, text: "• 最终执行扳机阻力与震动", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });

  // 连线 1 -> 2
  addArrow({ startX: 280, startY: 250, points: [[0, 0], [50, 0]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 285, y: 222, width: 50, height: 20, text: "独占接管", fontSize: 12, strokeColor: "#495057" });

  // 卡片 2: ds5vpad 中继守护进程 (x: 330, y: 140, w: 260, h: 220)
  addRect({ x: 330, y: 140, width: 260, height: 220, backgroundColor: "#e7f5ff", strokeColor: "#1971c2", strokeWidth: 2 });
  addText({ x: 340, y: 155, width: 240, height: 28, text: "ds5vpad 中继进程", fontSize: 18, strokeColor: "#1971c2" });
  addText({ x: 340, y: 183, width: 240, height: 20, text: "cmd/ds5vpad (uinput)", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 350, y: 215, width: 220, height: 24, text: "• EVIOCGRAB 独占物理手柄", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 350, y: 245, width: 220, height: 24, text: "• 转换摇杆与 DirectInput 踏板", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 350, y: 275, width: 220, height: 24, text: "• 拦截内核 UI_FF_UPLOAD 事件", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });

  // 连线 2 -> 3
  addArrow({ startX: 590, startY: 250, points: [[0, 0], [50, 0]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 595, y: 222, width: 50, height: 20, text: "透明转发", fontSize: 12, strokeColor: "#495057" });

  // 卡片 3: 虚拟手柄节点 (x: 640, y: 140, w: 230, h: 220)
  addRect({ x: 640, y: 140, width: 230, height: 220, backgroundColor: "#ebfbee", strokeColor: "#0ca678", strokeWidth: 2 });
  addText({ x: 650, y: 155, width: 210, height: 28, text: "虚拟 Xbox 控制器", fontSize: 18, strokeColor: "#0ca678" });
  addText({ x: 650, y: 183, width: 210, height: 20, text: "/dev/uinput 动态注入", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 660, y: 215, width: 190, height: 24, text: "• 暴露标准兼容力反馈属性", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 660, y: 245, width: 190, height: 24, text: "• 绕过 Steam Input 设备抢占", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 660, y: 275, width: 190, height: 24, text: "• 接收游戏 FF 震动数据包", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });

  // 连线 3 -> 4
  addArrow({ startX: 870, startY: 250, points: [[0, 0], [50, 0]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 875, y: 222, width: 50, height: 20, text: "手柄识别", fontSize: 12, strokeColor: "#495057" });

  // 卡片 4: 游戏进程 (x: 920, y: 140, w: 190, h: 220)
  addRect({ x: 920, y: 140, width: 190, height: 220, backgroundColor: "#f8f9fa", strokeColor: "#868e96", strokeWidth: 2 });
  addText({ x: 930, y: 155, width: 170, height: 28, text: "游戏进程", fontSize: 18, strokeColor: "#212529" });
  addText({ x: 930, y: 183, width: 170, height: 20, text: "Proton / Native Game", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 940, y: 215, width: 150, height: 24, text: "• 消费手柄轴输入", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 940, y: 245, width: 150, height: 24, text: "• 发生受击/开火震动", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 940, y: 275, width: 150, height: 24, text: "• 调用 FF 震动接口", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });

  // 第二行: 力反馈回传链路 (从右到左: 游戏 -> ds5vpad -> ds5d -> 物理手柄)
  // 连线: 游戏下发震动到 ds5vpad
  addArrow({
    startX: 1015, startY: 360,
    points: [[0, 0], [0, 45], [-555, 45]],
    strokeColor: "#dc2626", strokeWidth: 2.2, strokeStyle: "dashed"
  });
  addText({ x: 740, y: 388, width: 240, height: 20, text: "游戏下发 FF_RUMBLE 震动调用", fontSize: 12.5, strokeColor: "#dc2626" });

  // 卡片 5: ds5d 守护进程 (x: 330, y: 450, w: 260, h: 180)
  addRect({ x: 330, y: 450, width: 260, height: 180, backgroundColor: "#f3f0ff", strokeColor: "#7048e8", strokeWidth: 2 });
  addText({ x: 340, y: 465, width: 240, height: 28, text: "ds5d 守护进程", fontSize: 18, strokeColor: "#7048e8" });
  addText({ x: 340, y: 493, width: 240, height: 20, text: "Source: ffb ──► Rules 引擎", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 350, y: 525, width: 220, height: 24, text: "• 接收 UDP 9998 力反馈信号", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 350, y: 555, width: 220, height: 24, text: "• 提取 ffb_left 与 ffb_right", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 350, y: 585, width: 220, height: 24, text: "• 规则映射为自适应阻尼与触感", fontSize: 14, strokeColor: "#2b2b2b", textAlign: "left" });

  // 连线: ds5vpad 发送到 ds5d
  addArrow({
    startX: 460, startY: 360,
    points: [[0, 0], [0, 90]],
    strokeColor: "#1971c2", strokeWidth: 2.2
  });
  addText({ x: 470, y: 400, width: 140, height: 20, text: "UDP 9998 回传", fontSize: 12.5, strokeColor: "#1971c2", textAlign: "left" });

  // 连线: ds5d 输出到物理 DualSense
  addArrow({
    startX: 330, startY: 540,
    points: [[0, 0], [-165, 0], [-165, -180]],
    strokeColor: "#16a34a", strokeWidth: 2.2
  });
  addText({ x: 175, y: 518, width: 140, height: 20, text: "HID 报告写入驱动", fontSize: 12.5, strokeColor: "#16a34a" });

  // 底部说明条
  addRect({ x: 50, y: 660, width: 1060, height: 80, backgroundColor: "#fff5f5", strokeColor: "#e03131", strokeWidth: 1.5 });
  addText({ x: 70, y: 676, width: 1020, height: 24, text: "设计初衷：解决 Steam Input 抢占物理手柄、无遥测单机游戏实现自适应后坐力与高保真震动的核心工程闭环", fontSize: 14, strokeColor: "#c92a2a", textAlign: "left" });
  addText({ x: 70, y: 704, width: 1020, height: 22, text: "运行机制：物理手柄透明转发至虚拟手柄，游戏与虚拟手柄通信，捕获的 FF_RUMBLE 幅度经由 UDP 实时注入守护进程驱动真机", fontSize: 13, strokeColor: "#475569", textAlign: "left" });

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
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1160 770" width="100%" height="100%">
  <defs>
    <filter id="shadow" x="-3%" y="-3%" width="106%" height="106%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.06"/>
    </filter>
    <filter id="red-shadow" x="-2%" y="-2%" width="104%" height="104%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="3" stdDeviation="5" flood-color="#dc2626" flood-opacity="0.08"/>
    </filter>
    <marker id="arrow-gray" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#475569" />
    </marker>
    <marker id="arrow-blue" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#2563eb" />
    </marker>
    <marker id="arrow-red" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#dc2626" />
    </marker>
    <marker id="arrow-green" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#16a34a" />
    </marker>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif; font-weight: 800; fill: #0f172a; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 500; fill: #475569; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 17px; }
      .card-sub { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; fill: #64748b; }
      .card-body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13.5px; fill: #1e293b; }
      .tag-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 11.5px; font-weight: 600; fill: #475569; text-anchor: middle; }
    </style>
  </defs>

  <rect width="100%" height="100%" fill="#f8fafc" rx="16"/>

  <!-- 顶部主副标题 -->
  <g transform="translate(580, 48)" text-anchor="middle">
    <text class="title" font-size="28" y="0">力反馈镜像与虚拟手柄架构</text>
    <text class="subtitle" font-size="15" y="30">ds5vpad 虚拟手柄独占注入与游戏力反馈 (FF_RUMBLE) 回传驱动闭环</text>
  </g>

  <!-- ================= 第一行: 输入正向流 ================= -->

  <!-- 卡片 1: 物理手柄 -->
  <g transform="translate(50, 130)" filter="url(#shadow)">
    <rect width="230" height="220" rx="12" fill="#ffffff" stroke="#334155" stroke-width="2"/>
    <text class="card-title" x="20" y="36" fill="#0f172a">物理 DualSense 手柄</text>
    <text class="card-sub" x="20" y="56">/dev/input/eventX</text>
    <line x1="20" y1="70" x2="210" y2="70" stroke="#e2e8f0" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(20, 98)">
      <text y="0">• 物理按键与双摇杆输入</text>
      <text y="26">• L2/R2 模拟行程采样</text>
      <text y="52">• 最终执行扳机阻力与震动</text>
      <text y="78">• 硬件执行终端</text>
    </g>
  </g>

  <!-- 连线 1 -> 2 -->
  <line x1="280" y1="240" x2="330" y2="240" stroke="#475569" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="285" y="214" width="45" height="18" rx="3" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="tag-text" x="307" y="227">独占</text>

  <!-- 卡片 2: ds5vpad -->
  <g transform="translate(330, 130)" filter="url(#shadow)">
    <rect width="260" height="220" rx="12" fill="#ffffff" stroke="#2563eb" stroke-width="2"/>
    <text class="card-title" x="20" y="36" fill="#1d4ed8">ds5vpad 中继进程</text>
    <text class="card-sub" x="20" y="56">cmd/ds5vpad (uinput 驱动)</text>
    <line x1="20" y1="70" x2="240" y2="70" stroke="#e2e8f0" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(20, 98)">
      <text y="0">• EVIOCGRAB 独占物理手柄</text>
      <text y="26">• 转换摇杆与 DirectInput 踏板</text>
      <text y="52">• 动态注册 uinput 虚拟设备</text>
      <text y="78">• 拦截内核 UI_FF_UPLOAD 事件</text>
    </g>
  </g>

  <!-- 连线 2 -> 3 -->
  <line x1="590" y1="240" x2="640" y2="240" stroke="#475569" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="595" y="214" width="45" height="18" rx="3" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="tag-text" x="617" y="227">转发</text>

  <!-- 卡片 3: 虚拟手柄 -->
  <g transform="translate(640, 130)" filter="url(#shadow)">
    <rect width="230" height="220" rx="12" fill="#ffffff" stroke="#16a34a" stroke-width="2"/>
    <text class="card-title" x="20" y="36" fill="#15803d">虚拟 Xbox 360 手柄</text>
    <text class="card-sub" x="20" y="56">/dev/uinput 设备节点</text>
    <line x1="20" y1="70" x2="210" y2="70" stroke="#e2e8f0" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(20, 98)">
      <text y="0">• 暴露标准兼容力反馈属性</text>
      <text y="26">• 绕过 Steam 物理设备占用</text>
      <text y="52">• 接收游戏 FF 震动数据包</text>
      <text y="78">• 全游戏通用识别</text>
    </g>
  </g>

  <!-- 连线 3 -> 4 -->
  <line x1="870" y1="240" x2="920" y2="240" stroke="#475569" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="875" y="214" width="45" height="18" rx="3" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="tag-text" x="897" y="227">识别</text>

  <!-- 卡片 4: 游戏进程 -->
  <g transform="translate(920, 130)" filter="url(#shadow)">
    <rect width="190" height="220" rx="12" fill="#ffffff" stroke="#64748b" stroke-width="2"/>
    <text class="card-title" x="20" y="36" fill="#334155">游戏进程</text>
    <text class="card-sub" x="20" y="56">Proton / Native Game</text>
    <line x1="20" y1="70" x2="170" y2="70" stroke="#e2e8f0" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(20, 98)">
      <text y="0">• 消费手柄轴输入</text>
      <text y="26">• 发生受击/开火震动</text>
      <text y="52">• 下发 FF 震动指令</text>
      <text y="78">• 无感知运行</text>
    </g>
  </g>

  <!-- ================= 第二行: 力反馈回传流 ================= -->

  <!-- 连线: 游戏下发震动到 ds5vpad -->
  <path d="M 1015 350 L 1015 390 L 460 390" fill="none" stroke="#dc2626" stroke-width="2.2" stroke-dasharray="5,5" marker-end="url(#arrow-red)"/>
  <rect x="670" y="378" width="220" height="22" rx="4" fill="#ffffff" stroke="#fca5a5"/>
  <text class="tag-text" x="780" y="393" fill="#dc2626">游戏下发 FF_RUMBLE 震动调用</text>

  <!-- 卡片 5: ds5d 守护进程 -->
  <g transform="translate(330, 440)" filter="url(#shadow)">
    <rect width="260" height="180" rx="12" fill="#ffffff" stroke="#7c3aed" stroke-width="2"/>
    <text class="card-title" x="20" y="34" fill="#6d28d9">ds5d 守护进程</text>
    <text class="card-sub" x="20" y="54">Source: ffb ──► Rules 引擎</text>
    <line x1="20" y1="68" x2="240" y2="68" stroke="#e2e8f0" stroke-width="1.5" stroke-dasharray="4,4"/>
    <g class="card-body" transform="translate(20, 94)">
      <text y="0">• 接收 UDP 9998 力反馈信号</text>
      <text y="26">• 提取 ffb_left 与 ffb_right</text>
      <text y="52">• 规则映射为自适应阻尼与触感</text>
    </g>
  </g>

  <!-- 连线: ds5vpad 发送到 ds5d -->
  <line x1="460" y1="350" x2="460" y2="435" stroke="#2563eb" stroke-width="2.2" marker-end="url(#arrow-blue)"/>
  <rect x="470" y="380" width="105" height="22" rx="4" fill="#ffffff" stroke="#93c5fd"/>
  <text class="tag-text" x="522" y="395" fill="#1d4ed8">UDP 9998 回传</text>

  <!-- 连线: ds5d 驱动物理手柄 -->
  <path d="M 330 530 L 165 530 L 165 355" fill="none" stroke="#16a34a" stroke-width="2.2" marker-end="url(#arrow-green)"/>
  <rect x="180" y="518" width="130" height="22" rx="4" fill="#ffffff" stroke="#86efac"/>
  <text class="tag-text" x="245" y="533" fill="#15803d">物理力反馈驱动</text>

  <!-- 底部说明框 -->
  <g transform="translate(50, 650)" filter="url(#red-shadow)">
    <rect width="1060" height="85" rx="10" fill="#ffffff" stroke="#dc2626" stroke-width="1.5"/>
    <text class="card-body" x="24" y="32" font-weight="700" fill="#dc2626">设计初衷：解决 Steam Input 抢占物理手柄、无遥测单机游戏实现自适应后坐力与高保真震动的核心工程闭环</text>
    <text class="card-body" x="24" y="58" fill="#475569">运行机制：物理手柄透明转发至虚拟手柄，游戏与虚拟手柄通信，捕获的 FF_RUMBLE 幅度经由 UDP 实时注入守护进程驱动真机</text>
  </g>

</svg>
`;
}

function main() {
  const docsDir = path.resolve(__dirname, '../docs');
  const wikiDir = path.resolve(__dirname, '../wiki');

  const excalidrawContent = JSON.stringify(buildExcalidrawJSON(), null, 2);
  const excalidrawPath = path.join(docsDir, 'ffb-virtual-pad.excalidraw');
  fs.writeFileSync(excalidrawPath, excalidrawContent, 'utf-8');
  console.log(`[generate] Excalidraw generated: ${excalidrawPath}`);

  const svgContent = buildSVG();
  const svgPath = path.join(docsDir, 'ffb-virtual-pad.svg');
  fs.writeFileSync(svgPath, svgContent, 'utf-8');
  console.log(`[generate] SVG generated: ${svgPath}`);

  // 同步一份到 wiki 目录供 wiki 发布
  fs.writeFileSync(path.join(wikiDir, 'ffb-virtual-pad.svg'), svgContent, 'utf-8');

  const pngPath = path.join(docsDir, 'ffb-virtual-pad.png');
  try {
    execSync(`rsvg-convert -w 2400 "${svgPath}" -o "${pngPath}"`);
    console.log(`[generate] PNG generated: ${pngPath}`);
  } catch (err) {
    console.warn(`[warn] rsvg-convert failed: ${err.message}`);
  }
}

main();
