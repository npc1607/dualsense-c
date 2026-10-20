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
      groupIds: [], frameId: null,
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
    x: 150, y: 35, width: 800, height: 45,
    text: "RBR 控制器配置文件 (pfsam.acm) 二进制结构",
    fontSize: 30, strokeColor: "#1e1e1e", textAlign: "center"
  });
  addText({
    x: 150, y: 85, width: 800, height: 26,
    text: "Richard Burns Rally 经典 DirectInput 设备识别与定长 12 字节绑定三元组布局",
    fontSize: 16, strokeColor: "#495057", textAlign: "center"
  });

  // 区块 1: 文件头部 Header (x: 50, y: 135, w: 1000, h: 180)
  addRect({ x: 50, y: 135, width: 1000, height: 180, backgroundColor: "#e7f5ff", strokeColor: "#1971c2", strokeWidth: 2 });
  addText({ x: 70, y: 150, width: 960, height: 28, text: "1. 文件头部 Header (20 字节, 0x00..0x13)", fontSize: 20, strokeColor: "#1971c2" });
  addText({ x: 70, y: 178, width: 960, height: 20, text: "固定魔数识别与元数据总量定义 (全部采用 32 位小端无符号整数 uint32_t)", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 80, y: 210, width: 940, height: 24, text: "• 0x00..0x03 : PAMA 魔数标识 (char[4], 0x50414D41)", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 236, width: 940, height: 24, text: "• 0x04..0x07 : 引擎版本号 (uint32_t, 默认 0x00000300)", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 262, width: 940, height: 24, text: "• 0x08..0x0B : 配置套数 Config Count (uint32_t = 1)   |   0x0C..0x0F : 设备总数 Device Count (uint32_t = 3)   |   0x10..0x13 : 绑定项总数 (uint32_t = N)", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });

  // 连线 1 -> 2
  addArrow({ startX: 550, startY: 315, points: [[0, 0], [0, 45]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 560, y: 326, width: 180, height: 20, text: "依据 Device Count 展开设备表", fontSize: 12.5, strokeColor: "#495057", textAlign: "left" });

  // 区块 2: 设备描述表 Device Table (x: 50, y: 365, w: 1000, h: 190)
  addRect({ x: 50, y: 365, width: 1000, height: 190, backgroundColor: "#ebfbee", strokeColor: "#0ca678", strokeWidth: 2 });
  addText({ x: 70, y: 380, width: 960, height: 28, text: "2. 设备描述表 Device Table (Device Count × 20 字节, 0x14 起)", fontSize: 20, strokeColor: "#0ca678" });
  addText({ x: 70, y: 408, width: 960, height: 20, text: "记录每个接入设备的 Windows DirectInput 实例 GUID 与 DEST 4 字节固定标识", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 80, y: 440, width: 940, height: 24, text: "• 0x14..0x27 (Device[0]) : 设备实例 GUID (16 字节) + DEST 固定标签 (4 字节) ── 主力手柄 / 方向盘", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 468, width: 940, height: 24, text: "• 0x28..0x3B (Device[1]) : 设备实例 GUID (16 字节) + DEST 固定标签 (4 字节) ── 第二手柄 / 虚拟手柄", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 496, width: 940, height: 24, text: "• 0x3C..0x4F (Device[2]) : 设备实例 GUID (16 字节) + DEST 固定标签 (4 字节) ── 系统键盘 GUID", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });

  // 连线 2 -> 3
  addArrow({ startX: 550, startY: 555, points: [[0, 0], [0, 45]], strokeColor: "#495057", strokeWidth: 2 });
  addText({ x: 560, y: 566, width: 220, height: 20, text: "0x0000 填充对齐 ──► 绑定三元组数组", fontSize: 12.5, strokeColor: "#495057", textAlign: "left" });

  // 区块 3: 绑定数据块 Bindings Block (x: 50, y: 605, w: 1000, h: 235)
  addRect({ x: 50, y: 605, width: 1000, height: 235, backgroundColor: "#f3f0ff", strokeColor: "#7048e8", strokeWidth: 2 });
  addText({ x: 70, y: 620, width: 960, height: 28, text: "3. 绑定记录三元组数组 Bindings Block (Binding Count × 12 字节)", fontSize: 20, strokeColor: "#7048e8" });
  addText({ x: 70, y: 648, width: 960, height: 20, text: "每条绑定记录均为定长 12 字节结构体：struct { uint32_t device_id; uint32_t input_code; uint32_t function_id; }", fontSize: 13, strokeColor: "#868e96" });
  addText({ x: 80, y: 680, width: 940, height: 24, text: "• Device ID (4 字节)   : 引用设备描述表的索引 (0=第一设备, 1=第二设备, 2=键盘)", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 708, width: 940, height: 24, text: "• Input Code (4 字节)  : 输入编码 (高 16 位为类型: 0x02=模拟轴 Axis, 0x04=按键 Key; 低 16 位为扫描码)", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 736, width: 940, height: 24, text: "• Function ID (4 字节) : 目标功能定义 (0=Steering转向, 3=Throttle独立油门, 4=合并轴, 5=Footbrake独立刹车, 6=Handbrake手刹)", fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left" });
  addText({ x: 80, y: 768, width: 940, height: 22, text: "提示：游戏退出时强制按 pfsam.acm 结构写盘；设备顺序或 GUID 错位会导致整个键位映射错乱", fontSize: 13, strokeColor: "#c92a2a", textAlign: "left" });

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
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1100 870" width="100%" height="100%">
  <defs>
    <filter id="shadow" x="-3%" y="-3%" width="106%" height="106%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.06"/>
    </filter>
    <marker id="arrow-gray" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#495057" />
    </marker>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif; font-weight: 800; fill: #0f172a; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 500; fill: #475569; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 19px; }
      .card-sub { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12.5px; fill: #64748b; }
      .card-body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 14.5px; fill: #1e293b; }
      .tag-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 12px; font-weight: 600; fill: #475569; }
      .warn-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13px; font-weight: 600; fill: #dc2626; }
    </style>
  </defs>

  <rect width="100%" height="100%" fill="#f8fafc" rx="16"/>

  <!-- 顶部主副标题 -->
  <g transform="translate(550, 48)" text-anchor="middle">
    <text class="title" font-size="28" y="0">RBR 控制器配置文件 (pfsam.acm) 二进制结构</text>
    <text class="subtitle" font-size="15" y="30">Richard Burns Rally 经典 DirectInput 设备识别与定长 12 字节绑定三元组布局</text>
  </g>

  <!-- ================= 1. 文件头部 Header ================= -->
  <g transform="translate(50, 130)" filter="url(#shadow)">
    <rect width="1000" height="180" rx="14" fill="#ffffff" stroke="#2563eb" stroke-width="2"/>
    <text class="card-title" x="30" y="38" fill="#1d4ed8">1. 文件头部 Header (20 字节, 0x00..0x13)</text>
    <text class="card-sub" x="30" y="60">固定魔数识别与元数据总量定义 (全部采用 32 位小端无符号整数 uint32_t)</text>
    <line x1="30" y1="74" x2="970" y2="74" stroke="#e2e8f0" stroke-width="1.5" stroke-dasharray="4,4"/>
    
    <g class="card-body" transform="translate(30, 102)">
      <text y="0">• <tspan font-family="ui-monospace, monospace" font-weight="700">0x00..0x03</tspan> : PAMA 魔数标识 (char[4], ASCII "PAMA", 0x50414D41)</text>
      <text y="28">• <tspan font-family="ui-monospace, monospace" font-weight="700">0x04..0x07</tspan> : 引擎格式版本号 (uint32_t, 默认 0x00000300)</text>
      <text y="56">• <tspan font-family="ui-monospace, monospace" font-weight="700">0x08..0x0B</tspan> : 配置套数 Config Count (uint32_t = 1)   |   <tspan font-family="ui-monospace, monospace" font-weight="700">0x0C..0x0F</tspan> : 设备总数 Device Count (uint32_t = 3)   |   <tspan font-family="ui-monospace, monospace" font-weight="700">0x10..0x13</tspan> : 绑定项总数 (uint32_t = N)</text>
    </g>
  </g>

  <!-- 连线 1 -> 2 -->
  <line x1="550" y1="310" x2="550" y2="355" stroke="#64748b" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="560" y="322" width="180" height="22" rx="4" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="tag-text" x="570" y="337">依据 Device Count 展开设备表</text>

  <!-- ================= 2. 设备描述表 Device Table ================= -->
  <g transform="translate(50, 360)" filter="url(#shadow)">
    <rect width="1000" height="190" rx="14" fill="#ffffff" stroke="#16a34a" stroke-width="2"/>
    <text class="card-title" x="30" y="38" fill="#15803d">2. 设备描述表 Device Table (Device Count × 20 字节, 0x14 起)</text>
    <text class="card-sub" x="30" y="60">记录每个接入设备的 Windows DirectInput 实例 GUID 与 DEST 4 字节固定标识</text>
    <line x1="30" y1="74" x2="970" y2="74" stroke="#e2e8f0" stroke-width="1.5" stroke-dasharray="4,4"/>
    
    <g class="card-body" transform="translate(30, 102)">
      <text y="0">• <tspan font-family="ui-monospace, monospace" font-weight="700">0x14..0x27 (Device[0])</tspan> : 设备实例 GUID (16 字节) + DEST 固定标签 (4 字节) ── 主力手柄 / 方向盘</text>
      <text y="28">• <tspan font-family="ui-monospace, monospace" font-weight="700">0x28..0x3B (Device[1])</tspan> : 设备实例 GUID (16 字节) + DEST 固定标签 (4 字节) ── 第二手柄 / 虚拟手柄</text>
      <text y="56">• <tspan font-family="ui-monospace, monospace" font-weight="700">0x3C..0x4F (Device[2])</tspan> : 设备实例 GUID (16 字节) + DEST 固定标签 (4 字节) ── 系统键盘 GUID</text>
    </g>
  </g>

  <!-- 连线 2 -> 3 -->
  <line x1="550" y1="550" x2="550" y2="595" stroke="#64748b" stroke-width="2.2" marker-end="url(#arrow-gray)"/>
  <rect x="560" y="562" width="230" height="22" rx="4" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="tag-text" x="570" y="577">0x0000 填充对齐 ──► 绑定三元组数组</text>

  <!-- ================= 3. 绑定数据块 Bindings Block ================= -->
  <g transform="translate(50, 600)" filter="url(#shadow)">
    <rect width="1000" height="235" rx="14" fill="#ffffff" stroke="#7c3aed" stroke-width="2"/>
    <text class="card-title" x="30" y="38" fill="#6d28d9">3. 绑定记录三元组数组 Bindings Block (Binding Count × 12 字节)</text>
    <text class="card-sub" x="30" y="60">每条绑定记录均为定长 12 字节结构体：struct { uint32_t device_id; uint32_t input_code; uint32_t function_id; }</text>
    <line x1="30" y1="74" x2="970" y2="74" stroke="#e2e8f0" stroke-width="1.5" stroke-dasharray="4,4"/>
    
    <g class="card-body" transform="translate(30, 102)">
      <text y="0">• <tspan font-family="ui-monospace, monospace" font-weight="700">Device ID (4 字节)</tspan>   : 引用设备描述表的索引 (0=第一设备, 1=第二设备, 2=键盘)</text>
      <text y="28">• <tspan font-family="ui-monospace, monospace" font-weight="700">Input Code (4 字节)</tspan>  : 输入编码 (高 16 位为类型: 0x02=模拟轴 Axis, 0x04=数字按键 Key; 低 16 位为扫描码)</text>
      <text y="56">• <tspan font-family="ui-monospace, monospace" font-weight="700">Function ID (4 字节)</tspan> : 目标功能定义 (0=Steering转向, 3=Throttle独立油门, 4=合并轴, 5=Footbrake独立刹车, 6=Handbrake手刹)</text>
      <text y="86" class="warn-text">• 提示：游戏退出时强制按 pfsam.acm 结构写盘；设备顺序或 GUID 错位会导致整个键位映射错乱</text>
    </g>
  </g>

</svg>
`;
}

function main() {
  const docsDir = path.resolve(__dirname, '../docs');
  const wikiDir = path.resolve(__dirname, '../wiki');

  const excalidrawContent = JSON.stringify(buildExcalidrawJSON(), null, 2);
  const excalidrawPath = path.join(docsDir, 'rbr-acm-format.excalidraw');
  fs.writeFileSync(excalidrawPath, excalidrawContent, 'utf-8');
  console.log(`[generate] Excalidraw generated: ${excalidrawPath}`);

  const svgContent = buildSVG();
  const svgPath = path.join(docsDir, 'rbr-acm-format.svg');
  fs.writeFileSync(svgPath, svgContent, 'utf-8');
  console.log(`[generate] SVG generated: ${svgPath}`);

  // 同时同步一份到 wiki 目录供 wiki 发布
  fs.writeFileSync(path.join(wikiDir, 'rbr-acm-format.svg'), svgContent, 'utf-8');

  const pngPath = path.join(docsDir, 'rbr-acm-format.png');
  try {
    execSync(`rsvg-convert -w 2400 "${svgPath}" -o "${pngPath}"`);
    console.log(`[generate] PNG generated: ${pngPath}`);
  } catch (err) {
    console.warn(`[warn] rsvg-convert failed: ${err.message}`);
  }
}

main();
