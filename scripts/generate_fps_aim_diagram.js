const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const docsDir = path.resolve(__dirname, '../docs');
const wikiDir = path.resolve(__dirname, '../wiki');

// ==========================================
// 1. 架构流程图 (Pipeline Architecture SVG)
// ==========================================
function buildPipelineSVG() {
  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1160 860" width="100%" height="100%">
  <defs>
    <filter id="shadow" x="-3%" y="-3%" width="106%" height="106%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.06"/>
    </filter>
    <marker id="arrow" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#495057" />
    </marker>
    <marker id="arrow-blue" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#1971c2" />
    </marker>
    <marker id="arrow-purple" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#7048e8" />
    </marker>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 800; fill: #0f172a; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 500; fill: #475569; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 18px; }
      .card-sub { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12.5px; fill: #64748b; }
      .card-body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 14px; fill: #1e293b; line-height: 1.5; }
      .pill { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 600; fill: #334155; }
      .code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-weight: 600; fill: #099268; }
      .highlight { font-weight: 700; fill: #c92a2a; }
    </style>
  </defs>

  <rect width="100%" height="100%" fill="#f8fafc" rx="16"/>

  <!-- 顶部主副标题 -->
  <g transform="translate(580, 45)" text-anchor="middle">
    <text class="title" font-size="28" y="0">FPS 辅助瞄准与手柄增强流水线架构</text>
    <text class="subtitle" font-size="15" y="28">DualSense 摇杆曲线塑造 · One Euro 滤波 · 陀螺仪体感融合 · 弹道反冲抑制</text>
  </g>

  <!-- ================= 第一行：输入与滤波与曲线 ================= -->

  <!-- 节点 1: 物理硬件采集 -->
  <g transform="translate(40, 110)" filter="url(#shadow)">
    <rect width="320" height="210" rx="12" fill="#ebfbee" stroke="#0ca678" stroke-width="2"/>
    <text class="card-title" x="20" y="34" fill="#0ca678">1. 物理设备采集层</text>
    <text class="card-sub" x="20" y="54">DualSense HID (250Hz / 500Hz)</text>
    <line x1="20" y1="66" x2="300" y2="66" stroke="#b2f2bb" stroke-width="1.5"/>
    <g class="card-body" transform="translate(20, 92)">
      <text y="0">• <tspan class="code">Right Stick (RX, RY)</tspan> : 16 位大范围粗瞄</text>
      <text y="26">• <tspan class="code">IMU 6-Axis (Gyro)</tspan> : 偏航/俯仰高频角速度</text>
      <text y="52">• <tspan class="code">Triggers (L2 / R2)</tspan> : 举镜瞄准态 / 开火连击</text>
      <text y="78">• 物理零位死区消除与中心偏移自校准</text>
    </g>
  </g>

  <!-- 连线 1 -> 2 -->
  <line x1="360" y1="215" x2="420" y2="215" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow)"/>
  <rect x="368" y="196" width="46" height="20" rx="4" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="pill" x="375" y="210">RAW</text>

  <!-- 节点 2: 滤波与抗抖 -->
  <g transform="translate(420, 110)" filter="url(#shadow)">
    <rect width="320" height="210" rx="12" fill="#e7f5ff" stroke="#1971c2" stroke-width="2"/>
    <text class="card-title" x="20" y="34" fill="#1971c2">2. 动态自适应滤波层</text>
    <text class="card-sub" x="20" y="54">1€ Filter (One Euro) / EMA 降噪</text>
    <line x1="20" y1="66" x2="300" y2="66" stroke="#a5d8ff" stroke-width="1.5"/>
    <g class="card-body" transform="translate(20, 92)">
      <text y="0">• <tspan class="highlight">低速手颤消除</tspan> : 极小 cutoff 过滤心跳微抖</text>
      <text y="26">• <tspan class="highlight">高速零延迟直通</tspan> : 动态加大截断频率</text>
      <text y="52">• 解决手柄轻微推拉时“准心飘浮”现象</text>
      <text y="78">• 陀螺仪角速度传感器零漂实时跟踪</text>
    </g>
  </g>

  <!-- 连线 2 -> 3 -->
  <line x1="740" y1="215" x2="800" y2="215" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow)"/>
  <rect x="746" y="196" width="50" height="20" rx="4" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="pill" x="752" y="210">Clean</text>

  <!-- 节点 3: 响应曲线塑造 -->
  <g transform="translate(800, 110)" filter="url(#shadow)">
    <rect width="320" height="210" rx="12" fill="#f3f0ff" stroke="#7048e8" stroke-width="2"/>
    <text class="card-title" x="20" y="34" fill="#7048e8">3. 摇杆响应曲线塑形</text>
    <text class="card-sub" x="20" y="54">Sigmoid / 指数 / 双区曲线</text>
    <line x1="20" y1="66" x2="300" y2="66" stroke="#d0bfff" stroke-width="1.5"/>
    <g class="card-body" transform="translate(20, 92)">
      <text y="0">• <tspan class="code">微动爆头区 (0~35%)</tspan> : 扁平极低灵敏度</text>
      <text y="26">• <tspan class="code">过渡巡航区 (35~75%)</tspan> : 线性顺滑跟枪</text>
      <text y="52">• <tspan class="code">大范围拉枪 (75~100%)</tspan> : 指数级极速转身</text>
      <text y="78">• 曲线可在 profile yaml 动态更换</text>
    </g>
  </g>

  <!-- ================= 第一行折返到第二行 ================= -->
  <path d="M 960 320 L 960 370 L 200 370 L 200 420" fill="none" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow)"/>
  <rect x="540" y="358" width="80" height="24" rx="4" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="pill" x="548" y="374">曲线化摇杆</text>

  <!-- ================= 第二行：核心融合与控制 ================= -->

  <!-- 节点 4: 陀螺仪微调融合 -->
  <g transform="translate(40, 430)" filter="url(#shadow)">
    <rect width="320" height="210" rx="12" fill="#fff4e6" stroke="#e8590c" stroke-width="2"/>
    <text class="card-title" x="20" y="34" fill="#e8590c">4. 陀螺仪精瞄融合器</text>
    <text class="card-sub" x="20" y="54">L2 举镜门控 / 姿态投影映射</text>
    <line x1="20" y1="66" x2="300" y2="66" stroke="#ffd8a8" stroke-width="1.5"/>
    <g class="card-body" transform="translate(20, 92)">
      <text y="0">• <tspan class="highlight">L2 举镜激活 (Hold)</tspan> : 平时休眠不晕晃</text>
      <text y="26">• 角速度转视口偏移: <tspan class="code">ΔRX += Gyro_Yaw</tspan></text>
      <text y="52">• 手腕微晃 1 毫米 = 像素级爆头准心移动</text>
      <text y="78">• 具备死区紧绷抑制（Tightening Deadzone）</text>
    </g>
  </g>

  <!-- 连线 4 -> 5 -->
  <line x1="360" y1="535" x2="420" y2="535" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow)"/>
  <rect x="368" y="516" width="46" height="20" rx="4" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="pill" x="375" y="530">融合态</text>

  <!-- 节点 5: 弹道反冲抑制 -->
  <g transform="translate(420, 430)" filter="url(#shadow)">
    <rect width="320" height="210" rx="12" fill="#fff5f5" stroke="#e03131" stroke-width="2"/>
    <text class="card-title" x="20" y="34" fill="#e03131">5. 射击反冲自适应抑制</text>
    <text class="card-sub" x="20" y="54">R2 连发时序衰减器 (Anti-Recoil)</text>
    <line x1="20" y1="66" x2="300" y2="66" stroke="#ffc9c9" stroke-width="1.5"/>
    <g class="card-body" transform="translate(20, 92)">
      <text y="0">• <tspan class="code">R2 连发状态监听</tspan> : 开火持续时间 t 累加</text>
      <text y="26">• <tspan class="code">向下压枪补偿</tspan> : <tspan class="highlight">RY += R(t) + 噪声扰动</tspan></text>
      <text y="52">• 初始枪口上跳突补偿 + 持续后坐力平稳补偿</text>
      <text y="78">• 叠加高斯伪随机散布，杜绝死板机械手感</text>
    </g>
  </g>

  <!-- 连线 5 -> 6 -->
  <line x1="740" y1="535" x2="800" y2="535" stroke="#495057" stroke-width="2.2" marker-end="url(#arrow)"/>
  <rect x="748" y="516" width="48" height="20" rx="4" fill="#ffffff" stroke="#cbd5e1"/>
  <text class="pill" x="754" y="530">Final</text>

  <!-- 节点 6: 虚拟手柄驱动与游戏接入 -->
  <g transform="translate(800, 430)" filter="url(#shadow)">
    <rect width="320" height="210" rx="12" fill="#f8f9fa" stroke="#343a40" stroke-width="2"/>
    <text class="card-title" x="20" y="34" fill="#212529">6. 游戏端与虚拟驱动</text>
    <text class="card-sub" x="20" y="54">uinput (Linux) / ViGEm (Windows)</text>
    <line x1="20" y1="66" x2="300" y2="66" stroke="#ced4da" stroke-width="1.5"/>
    <g class="card-body" transform="translate(20, 92)">
      <text y="0">• <tspan class="code">Xbox 360 / Dual Action</tspan> 虚拟手柄输出</text>
      <text y="26">• 任何单机 FPS 即插即用（Steam / Proton）</text>
      <text y="52">• 零内存篡改、零外部依赖、极致纯粹</text>
      <text y="78">• 配合自适应扳机阻尼，实现硬核击发手感</text>
    </g>
  </g>

  <!-- ================= 底部：Profile 热更新与配置中枢 ================= -->
  <g transform="translate(40, 680)" filter="url(#shadow)">
    <rect width="1080" height="145" rx="12" fill="#ffffff" stroke="#495057" stroke-width="1.5"/>
    <text class="card-title" x="30" y="34" fill="#1e293b">⚙️ Profile 声明式配置中心 (profiles/fps-aim.yaml)</text>
    <text class="card-sub" x="30" y="54">所有算法参数、曲线形态、滤波系数与触发热键统一声明化，支持运行时热重载</text>
    <line x1="30" y1="64" x2="1050" y2="64" stroke="#e2e8f0" stroke-width="1.2"/>
    <g class="card-body" transform="translate(30, 88)">
      <text y="0">• <tspan font-family="ui-monospace, monospace" font-weight="700">aim_curve</tspan> : { type: sigmoid, exponent: 2.4, inner_deadzone: 0.04, outer_boost: 1.6 }</text>
      <text y="24">• <tspan font-family="ui-monospace, monospace" font-weight="700">aim_filter</tspan> : { type: one_euro, min_cutoff: 0.8, beta: 0.04, d_cutoff: 1.0 }</text>
      <text y="48">• <tspan font-family="ui-monospace, monospace" font-weight="700">gyro_aim</tspan> : { trigger: l2_hold, sens_yaw: 1.85, sens_pitch: 1.4, deadzone_deg: 0.5 }</text>
    </g>
  </g>

</svg>
`;
}

// ==========================================
// 2. 曲线算法对比图 (Curves Diagram SVG)
// ==========================================
function buildCurvesSVG() {
  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 890" width="100%" height="100%">
  <defs>
    <filter id="shadow" x="-3%" y="-3%" width="106%" height="106%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.06"/>
    </filter>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 800; fill: #0f172a; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 500; fill: #475569; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 16.5px; }
      .axis-label { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; fill: #64748b; font-weight: 600; }
      .curve-desc { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13px; fill: #334155; }
      .formula { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12.5px; font-weight: 700; fill: #2b8a3e; }
    </style>
  </defs>

  <rect width="100%" height="100%" fill="#f8fafc" rx="16"/>

  <!-- 顶部主副标题 -->
  <g transform="translate(600, 45)" text-anchor="middle">
    <text class="title" font-size="28" y="0">四大主流 FPS 摇杆响应算法与效果深度对比</text>
    <text class="subtitle" font-size="15" y="28">横轴：物理摇杆偏离量 [0% .. 100%]  ——  纵轴：游戏内视口旋转角速度 [0% .. 100%]</text>
  </g>

  <!-- 坐标系与网格主体区域 -->
  <!-- 坐标系左下角：(100, 560)，右上角：(580, 120)，宽480，高440 -->
  <g transform="translate(0, 0)">
    <!-- 背景框 -->
    <rect x="70" y="100" width="510" height="490" rx="12" fill="#ffffff" stroke="#cbd5e1" stroke-width="1.5" filter="url(#shadow)"/>
    
    <!-- 网格线 -->
    <path d="
      M 120 540 L 550 540
      M 120 435 L 550 435
      M 120 330 L 550 330
      M 120 225 L 550 225
      M 120 120 L 550 120
      M 120 540 L 120 120
      M 227.5 540 L 227.5 120
      M 335 540 L 335 120
      M 442.5 540 L 442.5 120
      M 550 540 L 550 120
    " fill="none" stroke="#f1f3f5" stroke-width="1.5"/>

    <!-- 坐标轴刻度标签 -->
    <text class="axis-label" x="120" y="562" text-anchor="middle">0%</text>
    <text class="axis-label" x="227.5" y="562" text-anchor="middle">25%</text>
    <text class="axis-label" x="335" y="562" text-anchor="middle">50%</text>
    <text class="axis-label" x="442.5" y="562" text-anchor="middle">75%</text>
    <text class="axis-label" x="550" y="562" text-anchor="middle">100%</text>
    
    <text class="axis-label" x="105" y="544" text-anchor="end">0%</text>
    <text class="axis-label" x="105" y="439" text-anchor="end">25%</text>
    <text class="axis-label" x="105" y="334" text-anchor="end">50%</text>
    <text class="axis-label" x="105" y="229" text-anchor="end">75%</text>
    <text class="axis-label" x="105" y="124" text-anchor="end">100%</text>

    <!-- 爆头微调区阴影提示 -->
    <rect x="120" y="120" width="150" height="420" fill="#e7f5ff" opacity="0.45"/>
    <text x="195" y="145" text-anchor="middle" font-family="sans-serif" font-size="12" font-weight="700" fill="#1971c2">微调爆头区 (0~35%)</text>

    <!-- 极速转身区阴影提示 -->
    <rect x="442.5" y="120" width="107.5" height="420" fill="#fff5f5" opacity="0.45"/>
    <text x="496" y="145" text-anchor="middle" font-family="sans-serif" font-size="12" font-weight="700" fill="#e03131">极速转身区 (75~100%)</text>

    <!-- 曲线 1: 线性 Linear (灰色虚线) -->
    <line x1="120" y1="540" x2="550" y2="120" stroke="#868e96" stroke-width="2.5" stroke-dasharray="6,4"/>

    <!-- 曲线 2: 指数幂次 Exponential y = x^2.2 (蓝色) -->
    <path d="M 120 540 Q 370 520 550 120" fill="none" stroke="#1971c2" stroke-width="3.5"/>

    <!-- 曲线 3: S形 Sigmoid (紫色，最推荐) -->
    <path d="M 120 540 C 270 535 330 370 550 120" fill="none" stroke="#7048e8" stroke-width="4"/>

    <!-- 曲线 4: 双区阶跃 Dual-Zone (橙色，守望先锋风格) -->
    <path d="M 120 540 L 442.5 350 L 550 120" fill="none" stroke="#e8590c" stroke-width="3"/>
  </g>

  <!-- 右侧：四大曲线详述卡片列表 (宽度 570px，充足且多行排版) -->
  <g transform="translate(605, 100)">

    <!-- 卡片 1: 经典 S 型 Sigmoid (紫色) -->
    <g transform="translate(0, 0)" filter="url(#shadow)">
      <rect width="525" height="114" rx="10" fill="#f3f0ff" stroke="#7048e8" stroke-width="1.8"/>
      <circle cx="20" cy="22" r="6" fill="#7048e8"/>
      <text class="card-title" x="35" y="27" fill="#7048e8">1. 经典 S 型曲线 (Sigmoid / 智能自适应) ⭐ 最推荐</text>
      <text class="formula" x="35" y="49">y = 1 / (1 + e^(-k * (x - x0))) 归一化</text>
      <text class="curve-desc" x="35" y="72">• 前段极其平缓，手腕与手指轻抖时准心稳定不飘；</text>
      <text class="curve-desc" x="35" y="93">• 中段斜率平稳跟枪，推到底部陡峭爆发 180° 闪身大拉枪。</text>
    </g>

    <!-- 卡片 2: 指数幂次曲线 Power / Exp (蓝色) -->
    <g transform="translate(0, 126)" filter="url(#shadow)">
      <rect width="525" height="114" rx="10" fill="#e7f5ff" stroke="#1971c2" stroke-width="1.8"/>
      <circle cx="20" cy="22" r="6" fill="#1971c2"/>
      <text class="card-title" x="35" y="27" fill="#1971c2">2. 指数幂次曲线 (Power Curve, γ = 2.0 ~ 2.5)</text>
      <text class="formula" x="35" y="49">y = x^γ (γ 取 2.2)</text>
      <text class="curve-desc" x="35" y="72">• COD / 战地传统手感，低推量段被持续压制；</text>
      <text class="curve-desc" x="35" y="93">• 中远距离点射爆头率高，但后段大范围提速相对平缓。</text>
    </g>

    <!-- 卡片 3: 双区曲线 Dual-Zone (橙色) -->
    <g transform="translate(0, 252)" filter="url(#shadow)">
      <rect width="525" height="114" rx="10" fill="#fff4e6" stroke="#e8590c" stroke-width="1.8"/>
      <circle cx="20" cy="22" r="6" fill="#e8590c"/>
      <text class="card-title" x="35" y="27" fill="#e8590c">3. 双区阶跃曲线 (Dual-Zone / 守望先锋风格)</text>
      <text class="formula" x="35" y="49">x &lt; 85%: y = 0.4*x/0.85  |  x ≥ 85%: 突增至 100%</text>
      <text class="curve-desc" x="35" y="72">• 内圈 85% 行程全程保持低速匀速微调，跟枪极稳；</text>
      <text class="curve-desc" x="35" y="93">• 推满外圈 15% 瞬间爆发最高灵敏度，适合习惯恒速的玩家。</text>
    </g>

    <!-- 卡片 4: 线性输入 Linear (灰色) -->
    <g transform="translate(0, 378)" filter="url(#shadow)">
      <rect width="525" height="112" rx="10" fill="#f8f9fa" stroke="#868e96" stroke-width="1.8"/>
      <circle cx="20" cy="22" r="6" fill="#868e96"/>
      <text class="card-title" x="35" y="27" fill="#495057">4. 原始线性输入 (Linear / 物理直出)</text>
      <text class="formula" x="35" y="49">y = x (无任何算法塑形)</text>
      <text class="curve-desc" x="35" y="72">• 手柄默认出厂状态，轻碰摇杆即容易滑出弱点；</text>
      <text class="curve-desc" x="35" y="93">• 调低灵敏度导致转身慢如蜗牛，单机 FPS 体验最差。</text>
    </g>

  </g>

  <!-- 底部对比结论总结条 -->
  <g transform="translate(70, 615)" filter="url(#shadow)">
    <rect width="1060" height="235" rx="12" fill="#ffffff" stroke="#2563eb" stroke-width="1.8"/>
    <text class="card-title" x="30" y="34" fill="#1d4ed8">💡 单机 FPS 实战选择建议与核心调优策略</text>
    <line x1="30" y1="48" x2="1030" y2="48" stroke="#e2e8f0" stroke-width="1.2"/>
    <g class="curve-desc" transform="translate(30, 72)">
      <text y="0">• <tspan font-weight="700" fill="#0f172a">慢节奏战术/狙击（潜行者、辐射、生化危机）</tspan>：</text>
      <text y="22" x="16" fill="#64748b">首选 Power 指数曲线 (γ=2.4)，把微动死区彻底压住，开镜锁头纹丝不动。</text>

      <text y="54">• <tspan font-weight="700" fill="#0f172a">高速爽快移动射击（赛博朋克2077、DOOM、光环）</tspan>：</text>
      <text y="76" x="16" fill="#64748b">首选 S 型 Sigmoid 曲线，兼顾近身大范围滑铲转身与中距离精准锁弱点。</text>

      <text y="108">• <tspan font-weight="700" fill="#0f172a">配合 1€ (One Euro) 动态自适应滤波</tspan>：</text>
      <text y="130" x="16" fill="#64748b">消除手指生理微颤，低速极度平滑，高速拉枪瞬时透明无延迟。</text>

      <text y="152">• <tspan font-weight="700" fill="#0f172a">结合 L2 举镜陀螺仪体感微调</tspan>：</text>
      <text y="174" x="16" fill="#64748b">摇杆只负责粗瞄至目标附近，最后爆头由手腕微晃 0 延迟命中，体验人枪合一！</text>
    </g>
  </g>

</svg>
`;
}

function main() {
  if (!fs.existsSync(docsDir)) fs.mkdirSync(docsDir, { recursive: true });
  if (!fs.existsSync(wikiDir)) fs.mkdirSync(wikiDir, { recursive: true });

  const pipelineSvg = buildPipelineSVG();
  const pipelinePath = path.join(docsDir, 'fps-aim-assist-pipeline.svg');
  fs.writeFileSync(pipelinePath, pipelineSvg, 'utf-8');
  fs.writeFileSync(path.join(wikiDir, 'fps-aim-assist-pipeline.svg'), pipelineSvg, 'utf-8');
  console.log(`[OK] Generated: ${pipelinePath}`);

  const curvesSvg = buildCurvesSVG();
  const curvesPath = path.join(docsDir, 'fps-aim-curves.svg');
  fs.writeFileSync(curvesPath, curvesSvg, 'utf-8');
  fs.writeFileSync(path.join(wikiDir, 'fps-aim-curves.svg'), curvesPath, 'utf-8');
  console.log(`[OK] Generated: ${curvesPath}`);

  try {
    execSync(`rsvg-convert -w 2400 "${pipelinePath}" -o "${path.join(docsDir, 'fps-aim-assist-pipeline.png')}"`);
    execSync(`rsvg-convert -w 2400 "${curvesPath}" -o "${path.join(docsDir, 'fps-aim-curves.png')}"`);
    console.log(`[OK] Converted to PNG successfully.`);
  } catch (e) {
    console.warn(`[Warn] rsvg-convert not available or failed: ${e.message}`);
  }
}

main();
