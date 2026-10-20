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
    x: 200, y: 35, width: 800, height: 45,
    text: "赛车直驱模拟器 FFB 力矩降维与抓地力极限曲线",
    fontSize: 28, strokeColor: "#1e1e1e", textAlign: "center"
  });
  addText({
    x: 200, y: 85, width: 800, height: 26,
    text: "将专业直驱对齐力矩 (Aligning Torque) 与滑移极限降维映射至自适应扳机与音圈触感",
    fontSize: 15.5, strokeColor: "#495057", textAlign: "center"
  });

  // 四块曲线卡片 (2x2 网格)
  // 卡片 1: 曲线 A (左上)
  addRect({ x: 50, y: 135, width: 535, height: 250, backgroundColor: "#ffffff", strokeColor: "#2563eb", strokeWidth: 2 });
  addText({ x: 70, y: 150, width: 495, height: 26, text: "曲线 A: S 形软饱和力矩 (Soft-Clipping S-Curve)", fontSize: 17, strokeColor: "#1d4ed8", textAlign: "left" });
  addText({ x: 70, y: 180, width: 495, height: 20, text: "映射: 横向载荷 lat_g ──► 转向扳机自适应阻力", fontSize: 13, strokeColor: "#64748b", textAlign: "left" });
  addText({ x: 70, y: 320, width: 495, height: 22, text: "特点: 小转角轻柔精准，中段线性饱满，极限大转角平滑渐近饱和防打手", fontSize: 12.5, strokeColor: "#1e293b", textAlign: "left" });
  addText({ x: 70, y: 345, width: 495, height: 22, text: "公式: force = clamp(255 * smoothstep(0, 1.2, abs(lat_g)), 0, 255)", fontSize: 12, strokeColor: "#2563eb", textAlign: "left" });

  // 卡片 2: 曲线 B (右上)
  addRect({ x: 615, y: 135, width: 535, height: 250, backgroundColor: "#ffffff", strokeColor: "#dc2626", strokeWidth: 2 });
  addText({ x: 635, y: 150, width: 495, height: 26, text: "曲线 B: 推头失速力矩骤降 (Understeer Slip-Drop)", fontSize: 17, strokeColor: "#b91c1c", textAlign: "left" });
  addText({ x: 635, y: 180, width: 495, height: 20, text: "映射: 前胎滑移率 slip_front ──► 抓地力丧失力矩瞬间落空", fontSize: 13, strokeColor: "#64748b", textAlign: "left" });
  addText({ x: 635, y: 320, width: 495, height: 22, text: "特点: 滑移率在临界点内阻力扎实，一旦推头失抓，阻力断崖式释放 50%", fontSize: 12.5, strokeColor: "#1e293b", textAlign: "left" });
  addText({ x: 635, y: 345, width: 495, height: 22, text: "公式: drop = slip > 0.16 ? lerp(1.0, 0.45, (slip - 0.16)/0.2) : 1.0", fontSize: 12, strokeColor: "#dc2626", textAlign: "left" });

  // 卡片 3: 曲线 C (左下)
  addRect({ x: 50, y: 410, width: 535, height: 250, backgroundColor: "#ffffff", strokeColor: "#7c3aed", strokeWidth: 2 });
  addText({ x: 70, y: 425, width: 495, height: 26, text: "曲线 C: 轮胎抓地力临界高频啸叫 (Grip Boundary Vibration)", fontSize: 17, strokeColor: "#6d28d9", textAlign: "left" });
  addText({ x: 70, y: 455, width: 495, height: 20, text: "映射: 极限临界摩擦过渡区 ──► 音圈马达 120~180Hz 物理微颤", fontSize: 13, strokeColor: "#64748b", textAlign: "left" });
  addText({ x: 70, y: 595, width: 495, height: 22, text: "特点: 车辆逼近抓地力极限边缘时，手心产生细密高频嘶鸣，提示极限临界", fontSize: 12.5, strokeColor: "#1e293b", textAlign: "left" });
  addText({ x: 70, y: 620, width: 495, height: 22, text: "公式: haptic = clamp(200 * smoothstep(0.12, 0.20, slip), 0, 180)", fontSize: 12, strokeColor: "#7c3aed", textAlign: "left" });

  // 卡片 4: 曲线 D (右下)
  addRect({ x: 615, y: 410, width: 535, height: 250, backgroundColor: "#ffffff", strokeColor: "#16a34a", strokeWidth: 2 });
  addText({ x: 635, y: 425, width: 495, height: 26, text: "曲线 D: 车速感应阻尼增益 (Speed-Sensitive Damping)", fontSize: 17, strokeColor: "#15803d", textAlign: "left" });
  addText({ x: 635, y: 455, width: 495, height: 20, text: "映射: 车速 speed (km/h) ──► 转向中心阻尼与路感基底", fontSize: 13, strokeColor: "#64748b", textAlign: "left" });
  addText({ x: 635, y: 595, width: 495, height: 22, text: "特点: 低速倒车移库轻柔灵敏，高速直线巡航自动沉稳下沉，杜绝抖动", fontSize: 12.5, strokeColor: "#1e293b", textAlign: "left" });
  addText({ x: 635, y: 620, width: 495, height: 22, text: "公式: speed_gain = lerp(0.3, 1.0, smoothstep(20, 120, speed))", fontSize: 12, strokeColor: "#16a34a", textAlign: "left" });

  // 底部 YAML 配置指导说明框
  addRect({ x: 50, y: 685, width: 1100, height: 160, backgroundColor: "#ffffff", strokeColor: "#0f172a", strokeWidth: 2 });
  addText({ x: 70, y: 702, width: 1060, height: 26, text: "Profile YAML 曲线自定义语法 (Assetto Corsa / ACR 实机支持)", fontSize: 18, strokeColor: "#0f172a", textAlign: "left" });
  addText({ x: 70, y: 735, width: 1060, height: 24, text: "• 基础力矩: force: 'clamp(255 * smoothstep(0, 1.5, abs(lat_g)) * (slip > 0.18 ? 0.5 : 1.0), 0, 255)'", fontSize: 13.5, strokeColor: "#2563eb", textAlign: "left" });
  addText({ x: 70, y: 765, width: 1060, height: 24, text: "• 极限抓地音圈: target: haptic_left, value: 'clamp(220 * smoothstep(0.12, 0.22, slip), 0, 200)', modulate: { type: sine, hz: 160 }", fontSize: 13.5, strokeColor: "#7c3aed", textAlign: "left" });
  addText({ x: 70, y: 795, width: 1060, height: 24, text: "• 车手可自由修改 smoothstep 的阈值区间与衰减比例，调校专属软硬手感与推头阻力落空点", fontSize: 13, strokeColor: "#475569", textAlign: "left" });

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
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 890" width="100%" height="100%">
  <defs>
    <filter id="shadow" x="-3%" y="-3%" width="106%" height="106%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.06"/>
    </filter>
    <marker id="arrow" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
      <path d="M 0 1.5 L 6.5 4 L 0 6.5 Z" fill="#64748b" />
    </marker>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif; font-weight: 800; fill: #0f172a; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 500; fill: #475569; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 16px; }
      .card-sub { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; fill: #64748b; }
      .desc-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13px; fill: #1e293b; }
      .code-text { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; font-weight: 600; }
      .axis-label { font-family: ui-monospace, monospace; font-size: 11px; fill: #64748b; }
    </style>
  </defs>

  <rect width="100%" height="100%" fill="#f8fafc" rx="16"/>

  <!-- 顶部主副标题 -->
  <g transform="translate(600, 48)" text-anchor="middle">
    <text class="title" font-size="28" y="0">赛车直驱模拟器 FFB 力矩降维与抓地力极限曲线</text>
    <text class="subtitle" font-size="15" y="30">将专业直驱对齐力矩 (Aligning Torque) 与滑移极限降维映射至自适应扳机与音圈触感</text>
  </g>

  <!-- ================= 曲线 A: S 形软饱和曲线 (左上) ================= -->
  <g transform="translate(50, 130)" filter="url(#shadow)">
    <rect width="535" height="255" rx="12" fill="#ffffff" stroke="#2563eb" stroke-width="2"/>
    <text class="card-title" x="24" y="32" fill="#1d4ed8">曲线 A: S 形软饱和力矩 (Soft-Clipping S-Curve)</text>
    <text class="card-sub" x="24" y="52">横向载荷 lat_g / 转向角 ──► 扳机阻力</text>

    <!-- 曲线图表坐标系 -->
    <g transform="translate(45, 68)">
      <!-- 背景网格 -->
      <line x1="0" y1="110" x2="440" y2="110" stroke="#cbd5e1" stroke-width="1.5"/>
      <line x1="0" y1="10" x2="0" y2="110" stroke="#cbd5e1" stroke-width="1.5"/>
      <line x1="0" y1="60" x2="440" y2="60" stroke="#f1f5f9" stroke-width="1" stroke-dasharray="3,3"/>
      <line x1="220" y1="10" x2="220" y2="110" stroke="#f1f5f9" stroke-width="1" stroke-dasharray="3,3"/>

      <!-- S-Curve 真实曲线 -->
      <path d="M 0 110 C 140 105, 200 45, 440 22" fill="none" stroke="#2563eb" stroke-width="3"/>
      
      <!-- 坐标文字 -->
      <text class="axis-label" x="430" y="125" text-anchor="end">载荷 lat_g ──►</text>
      <text class="axis-label" x="8" y="18">阻力 255</text>
      <text class="axis-label" x="8" y="105">0</text>
    </g>

    <text class="desc-text" x="24" y="218">• 特点：小转角轻柔精准，中段线性饱满，极限大转角平滑渐近饱和防打手</text>
    <text class="code-text" x="24" y="238" fill="#1d4ed8">公式：force = clamp(255 * smoothstep(0, 1.2, abs(lat_g)), 0, 255)</text>
  </g>

  <!-- ================= 曲线 B: 推头失速力矩骤降 (右上) ================= -->
  <g transform="translate(615, 130)" filter="url(#shadow)">
    <rect width="535" height="255" rx="12" fill="#ffffff" stroke="#dc2626" stroke-width="2"/>
    <text class="card-title" x="24" y="32" fill="#b91c1c">曲线 B: 推头失速力矩骤降 (Understeer Slip-Drop)</text>
    <text class="card-sub" x="24" y="52">前胎滑移率 slip_front ──► 抓地力丧失力矩瞬间落空</text>

    <!-- 曲线图表坐标系 -->
    <g transform="translate(45, 68)">
      <line x1="0" y1="110" x2="440" y2="110" stroke="#cbd5e1" stroke-width="1.5"/>
      <line x1="0" y1="10" x2="0" y2="110" stroke="#cbd5e1" stroke-width="1.5"/>
      <line x1="200" y1="10" x2="200" y2="110" stroke="#fee2e2" stroke-width="1.5" stroke-dasharray="4,4"/>

      <!-- 抓地力达到极限后突降曲线 -->
      <path d="M 0 100 Q 120 40 190 20 L 220 70 Q 320 75 440 75" fill="none" stroke="#dc2626" stroke-width="3"/>
      
      <circle cx="190" cy="20" r="4" fill="#dc2626"/>
      <text class="axis-label" x="195" y="15" fill="#dc2626" font-weight="700">极限抓地临界点</text>

      <text class="axis-label" x="430" y="125" text-anchor="end">滑移率 slip ──►</text>
      <text class="axis-label" x="8" y="18">对齐力矩</text>
      <text class="axis-label" x="8" y="105">0</text>
    </g>

    <text class="desc-text" x="24" y="218">• 特点：滑移率在极限内阻力扎实，一旦推头失抓，阻力断崖式释放 50% 变轻</text>
    <text class="code-text" x="24" y="238" fill="#b91c1c">公式：drop = slip > 0.16 ? lerp(1.0, 0.45, (slip - 0.16)/0.2) : 1.0</text>
  </g>

  <!-- ================= 曲线 C: 轮胎抓地力临界高频啸叫 (左下) ================= -->
  <g transform="translate(50, 410)" filter="url(#shadow)">
    <rect width="535" height="255" rx="12" fill="#ffffff" stroke="#7c3aed" stroke-width="2"/>
    <text class="card-title" x="24" y="32" fill="#6d28d9">曲线 C: 轮胎抓地力临界微颤 (Grip Boundary Vibration)</text>
    <text class="card-sub" x="24" y="52">临界摩擦过渡区 ──► 音圈马达 120~180Hz 物理微颤</text>

    <!-- 曲线图表坐标系 -->
    <g transform="translate(45, 68)">
      <line x1="0" y1="110" x2="440" y2="110" stroke="#cbd5e1" stroke-width="1.5"/>
      <line x1="0" y1="10" x2="0" y2="110" stroke="#cbd5e1" stroke-width="1.5"/>
      <rect x="150" y="10" width="120" height="100" fill="#f5f3ff" opacity="0.6"/>

      <!-- 临界钟形/阶梯微颤波形 -->
      <path d="M 0 110 L 140 110 Q 170 110 190 20 Q 230 20 260 110 L 440 110" fill="none" stroke="#7c3aed" stroke-width="3"/>
      
      <text class="axis-label" x="210" y="15" text-anchor="middle" fill="#6d28d9" font-weight="700">抓地极限啸叫区间</text>
      <text class="axis-label" x="430" y="125" text-anchor="end">滑移率 slip ──►</text>
      <text class="axis-label" x="8" y="18">微颤强度</text>
      <text class="axis-label" x="8" y="105">0</text>
    </g>

    <text class="desc-text" x="24" y="218">• 特点：车辆逼近抓地力极限边缘时，手心产生细密高频嘶鸣，提示极限临界</text>
    <text class="code-text" x="24" y="238" fill="#6d28d9">公式：haptic = clamp(200 * smoothstep(0.12, 0.20, slip), 0, 180)</text>
  </g>

  <!-- ================= 曲线 D: 车速感应阻尼增益 (右下) ================= -->
  <g transform="translate(615, 410)" filter="url(#shadow)">
    <rect width="535" height="255" rx="12" fill="#ffffff" stroke="#16a34a" stroke-width="2"/>
    <text class="card-title" x="24" y="32" fill="#15803d">曲线 D: 车速感应阻尼增益 (Speed-Sensitive Damping)</text>
    <text class="card-sub" x="24" y="52">车速 speed (km/h) ──► 转向中心阻尼与路感基底</text>

    <!-- 曲线图表坐标系 -->
    <g transform="translate(45, 68)">
      <line x1="0" y1="110" x2="440" y2="110" stroke="#cbd5e1" stroke-width="1.5"/>
      <line x1="0" y1="10" x2="0" y2="110" stroke="#cbd5e1" stroke-width="1.5"/>

      <!-- 对数/平滑阶梯曲线 -->
      <path d="M 0 95 Q 120 90 200 45 Q 320 25 440 20" fill="none" stroke="#16a34a" stroke-width="3"/>

      <text class="axis-label" x="430" y="125" text-anchor="end">车速 speed ──►</text>
      <text class="axis-label" x="8" y="18">阻尼倍率 1.0</text>
      <text class="axis-label" x="8" y="95">0.3</text>
    </g>

    <text class="desc-text" x="24" y="218">• 特点：低速倒车移库轻柔灵敏，高速直线巡航自动沉稳下沉，杜绝虚位抖动</text>
    <text class="code-text" x="24" y="238" fill="#15803d">公式：speed_gain = lerp(0.3, 1.0, smoothstep(20, 120, speed))</text>
  </g>

  <!-- ================= 底部配置说明框 ================= -->
  <g transform="translate(50, 690)" filter="url(#shadow)">
    <rect width="1100" height="165" rx="12" fill="#ffffff" stroke="#0f172a" stroke-width="2"/>
    <text class="card-title" x="24" y="32" fill="#0f172a">Profile YAML 曲线自定义语法 (Assetto Corsa / ACR 实机支持)</text>
    <line x1="24" y1="46" x2="1076" y2="46" stroke="#e2e8f0" stroke-width="1.5"/>
    
    <g class="code-text" transform="translate(24, 72)">
      <text y="0" fill="#2563eb">• 基础回正力矩: force: 'clamp(255 * smoothstep(0, 1.2, abs(lat_g)) * (slip > 0.16 ? lerp(1.0, 0.45, (slip - 0.16)/0.2) : 1.0), 0, 255)'</text>
      <text y="28" fill="#7c3aed">• 极限抓地音圈: target: haptic_left, value: 'clamp(200 * smoothstep(0.12, 0.20, slip), 0, 180)', modulate: { type: sine, hz: 160 }</text>
      <text y="56" fill="#475569" font-family="-apple-system, sans-serif" font-weight="500">• 调优指导：车手可在 YAML 中修改 smoothstep 临界点与 lerp 衰减幅度，随心定制手感软硬与推头落空时机</text>
    </g>
  </g>

</svg>
`;
}

function main() {
  const docsDir = path.resolve(__dirname, '../docs');
  if (!fs.existsSync(docsDir)) {
    fs.mkdirSync(docsDir, { recursive: true });
  }

  const excalidrawContent = JSON.stringify(buildExcalidrawJSON(), null, 2);
  const excalidrawPath = path.join(docsDir, 'direct-drive-curves.excalidraw');
  fs.writeFileSync(excalidrawPath, excalidrawContent, 'utf-8');
  console.log(`[generate] Excalidraw generated: ${excalidrawPath}`);

  const svgContent = buildSVG();
  const svgPath = path.join(docsDir, 'direct-drive-curves.svg');
  fs.writeFileSync(svgPath, svgContent, 'utf-8');
  console.log(`[generate] SVG generated: ${svgPath}`);

  const pngPath = path.join(docsDir, 'direct-drive-curves.png');
  try {
    execSync(`rsvg-convert -w 2400 "${svgPath}" -o "${pngPath}"`);
    console.log(`[generate] PNG generated: ${pngPath}`);
  } catch (err) {
    console.warn(`[warn] rsvg-convert failed: ${err.message}`);
  }
}

main();
