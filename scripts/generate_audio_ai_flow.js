#!/usr/bin/env node
// Generates the diagrams used by docs/audio-ai-haptics.md.
//
//   node scripts/generate_audio_ai_flow.js
//
// Produces:
//   docs/audio-ai-flow.svg    inference pipeline (PCM -> features -> YAMNet -> rules)
//   docs/audio-ai-hd.svg      the three paths that drive the voice coils
//
// The SVGs are hand-laid-out (no layout engine), so coordinates are explicit and
// easy to nudge when labels change.

const fs = require('fs');
const path = require('path');

const OUT = path.join(__dirname, '..', 'docs');

const C = {
  bg: '#ffffff',
  box: '#f8f9fa',
  line: '#868e96',
  text: '#212529',
  muted: '#6c757d',
  audio: '#1971c2',
  ai: '#7048e8',
  coil: '#0ca678',
  motor: '#e8590c',
  trigger: '#c2255c',
};

function header(w, h) {
  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="100%" height="100%" font-family="Segoe UI, Noto Sans CJK SC, sans-serif">
  <defs>
    <marker id="ar" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">
      <path d="M0,0 L10,4 L0,8 z" fill="${C.line}"/>
    </marker>
    <marker id="ar-audio" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">
      <path d="M0,0 L10,4 L0,8 z" fill="${C.audio}"/>
    </marker>
    <marker id="ar-ai" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">
      <path d="M0,0 L10,4 L0,8 z" fill="${C.ai}"/>
    </marker>
    <marker id="ar-coil" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">
      <path d="M0,0 L10,4 L0,8 z" fill="${C.coil}"/>
    </marker>
  </defs>
  <rect width="100%" height="100%" fill="${C.bg}"/>`;
}

const box = (x, y, w, h, fill, stroke) =>
  `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="8" fill="${fill}" stroke="${stroke}" stroke-width="1.4"/>`;

// XML needs &, < and > escaped in text nodes; several labels contain && and <.
const esc = (s) =>
  String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

const text = (x, y, s, size = 13, color = C.text, weight = 400, anchor = 'start') =>
  `<text x="${x}" y="${y}" font-size="${size}" fill="${color}" font-weight="${weight}" text-anchor="${anchor}">${esc(s)}</text>`;

const arrow = (x1, y1, x2, y2, color = C.line, marker = 'ar') =>
  `<line x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}" stroke="${color}" stroke-width="1.8" marker-end="url(#${marker})"/>`;

const elbow = (x1, y1, x2, y2, color = C.line, marker = 'ar') =>
  `<path d="M${x1},${y1} L${(x1 + x2) / 2},${y1} L${(x1 + x2) / 2},${y2} L${x2},${y2}" fill="none" stroke="${color}" stroke-width="1.8" marker-end="url(#${marker})"/>`;

// ---------------------------------------------------------------- flow -----

function flow() {
  const W = 1000, H = 620;
  let s = header(W, H);

  s += text(28, 36, '音频 AI 推理流程（source/audio → YAMNet → rules → 音圈）', 17, C.text, 600);

  // capture
  s += box(28, 70, 200, 84, '#e7f5ff', C.audio);
  s += text(44, 96, 'parec 16 kHz', 13, C.audio, 600);
  s += text(44, 118, 'monitor 环回采集', 12, C.muted);
  s += text(44, 138, '--latency-msec=20', 11, C.muted);

  // block read
  s += box(28, 188, 200, 76, '#f1f3f5', C.line);
  s += text(44, 214, 'Read 256 帧 (16 ms)', 12, C.text, 600);
  s += text(44, 236, 'consume()', 11, C.muted);

  // features
  s += box(282, 188, 210, 108, '#e7f5ff', C.audio);
  s += text(298, 212, 'measure()', 13, C.audio, 600);
  s += text(298, 234, 'bass / mid / loudness', 11, C.muted);
  s += text(298, 254, 'onset = (fast−slow)/slow', 11, C.muted);
  s += text(298, 276, '3× biquad 带通', 11, C.muted);

  // ring
  s += box(282, 320, 210, 76, '#f1f3f5', C.line);
  s += text(298, 346, 'feedAI(): 16000 样本环', 12, C.text, 600);
  s += text(298, 368, '每 classify_ms(120) 取一窗', 11, C.muted);

  // engine
  s += box(546, 320, 250, 132, '#f3f0ff', C.ai);
  s += text(562, 346, 'classifyLoop() goroutine', 13, C.ai, 600);
  s += text(562, 368, 'ortEngine.Classify(win)', 12, C.text);
  s += text(562, 388, 'tensor [16000] f32 → Run()', 11, C.muted);
  s += text(562, 408, '输出 [frames, 521] → 求平均', 11, C.muted);
  s += text(562, 430, '类别名映射 → 每信号取最高分', 11, C.muted);

  // signals
  s += box(546, 486, 250, 96, '#f3f0ff', C.ai);
  s += text(562, 510, 'AI 信号 (aiCh, 只保留最新)', 12, C.ai, 600);
  s += text(562, 532, 'surface_water/metal/wood/gravel', 10.5, C.muted);
  s += text(562, 550, 'audio_gunshot/explosion/impact', 10.5, C.muted);
  s += text(562, 568, '每帧(60Hz)并入信号集', 10.5, C.muted);

  // rules
  s += box(846, 188, 130, 108, '#f8f9fa', C.line);
  s += text(860, 212, '规则引擎', 13, C.text, 600);
  s += text(860, 234, 'when 表达式', 11, C.muted);
  s += text(860, 254, 'crossed()/has()', 11, C.muted);
  s += text(860, 276, '优先级覆盖', 11, C.muted);

  // outputs
  s += box(846, 330, 130, 60, '#e6fcf5', C.coil);
  s += text(860, 354, '音圈 Ch3/4', 12, C.coil, 600);
  s += text(860, 374, 'haptic_*', 10.5, C.muted);

  s += box(846, 404, 130, 60, '#fff4e6', C.motor);
  s += text(860, 428, '马达', 12, C.motor, 600);
  s += text(860, 448, 'rumble_*', 10.5, C.muted);

  s += box(846, 480, 130, 60, '#fff0f6', C.trigger);
  s += text(860, 504, '扳机', 12, C.trigger, 600);
  s += text(860, 524, 'l2 / r2', 10.5, C.muted);

  // arrows
  s += arrow(228, 112, 274, 112, C.audio, 'ar-audio');
  s += arrow(274, 112, 274, 214, C.audio, 'ar-audio');
  s += arrow(228, 226, 274, 226, C.audio, 'ar-audio');
  s += arrow(492, 236, 538, 236, C.audio, 'ar-audio');
  s += arrow(538, 236, 538, 300, C.audio, 'ar-audio');
  s += arrow(492, 356, 538, 356, C.ai, 'ar-ai');
  s += arrow(796, 386, 838, 386, C.ai, 'ar-ai');
  s += arrow(796, 534, 838, 534, C.ai, 'ar-ai');
  s += arrow(492, 300, 520, 300, C.audio, 'ar-audio');
  s += `<path d="M520,300 L520,470 L838,470" fill="none" stroke="${C.line}" stroke-width="1.6" stroke-dasharray="5 4" marker-end="url(#ar)"/>`;
  s += text(530, 462, 'onset 直连（不等 AI）', 10.5, C.muted);

  s += text(28, 594, '延迟：采集 ≈37 ms · 瞬态路径 ≈44 ms · AI 路径 ≈150 ms（窗口末端在"现在"）', 12, C.muted);

  s += '</svg>\n';
  return s;
}

// ------------------------------------------------------------------- hd -----

function hd() {
  const W = 1000, H = 520;
  let s = header(W, H);

  s += text(28, 36, 'HD 触觉的三条触发通路', 17, C.text, 600);

  // game audio
  s += box(28, 80, 180, 70, '#e7f5ff', C.audio);
  s += text(44, 106, '游戏音频', 13, C.audio, 600);
  s += text(44, 128, 'PipeWire monitor', 11, C.muted);

  // path 1
  s += box(300, 70, 300, 92, '#e6fcf5', C.coil);
  s += text(316, 94, '① 音圈直通 passthrough', 13, C.coil, 600);
  s += text(316, 116, '25–200 Hz 带通 × 规则增益', 11, C.muted);
  s += text(316, 136, '音圈重放游戏自己的波形 →', 11, C.muted);
  s += text(316, 154, '材质质感天然不同（碎石/金属/水）', 11, C.muted);

  // path 2
  s += box(300, 196, 300, 84, '#fff4e6', C.motor);
  s += text(316, 220, '② 瞬态 onset（~20 ms）', 13, C.motor, 600);
  s += text(316, 242, 'fast/slow 包络差，对背景音量免疫', 11, C.muted);
  s += text(316, 262, 'onset > 0.55 && loudness > 0.35', 11, C.muted);

  // path 3
  s += box(300, 314, 300, 84, '#f3f0ff', C.ai);
  s += text(316, 338, '③ AI 事件确认（~150 ms）', 13, C.ai, 600);
  s += text(316, 360, 'crossed("audio_gunshot", 0.30)', 11, C.muted);
  s += text(316, 380, '边沿触发：一次事件只响一次', 11, C.muted);

  // coil
  s += box(700, 70, 260, 108, '#e6fcf5', C.coil);
  s += text(716, 96, '音圈 Ch3/4（HD）', 13, C.coil, 600);
  s += text(716, 118, 'mode: passthrough', 11, C.muted);
  s += text(716, 138, 'haptic_left / haptic_right = 增益', 11, C.muted);
  s += text(716, 158, 'gain 0.7 · low 55Hz · high 200Hz', 11, C.muted);

  // motor
  s += box(700, 200, 260, 80, '#fff4e6', C.motor);
  s += text(716, 224, '马达（体量）', 13, C.motor, 600);
  s += text(716, 246, 'rumble_left / rumble_right', 11, C.muted);
  s += text(716, 266, '枪声/爆炸的"咚"', 11, C.muted);

  // trigger
  s += box(700, 314, 260, 84, '#fff0f6', C.trigger);
  s += text(716, 338, '扳机（操作感）', 13, C.trigger, 600);
  s += text(716, 360, 'rigid 挡墙 + weapon "咔哒"', 11, C.muted);
  s += text(716, 380, 'L2 45% / R2 70%，按住 14Hz 连发', 11, C.muted);

  // arrows
  s += arrow(208, 108, 292, 108, C.audio, 'ar-audio');
  s += elbow(600, 116, 692, 116, C.coil, 'ar-coil');
  s += elbow(600, 238, 692, 238, C.coil, 'ar-coil');
  s += elbow(600, 356, 692, 348, C.coil, 'ar-coil');
  s += text(614, 232, '音圈', 10.5, C.coil);
  s += text(614, 350, '马达', 10.5, C.motor);

  s += text(28, 448, '互斥规则：单发路径要求 r2 < 0.80；按住时只保留一条 14 Hz 节奏（优先级 48），', 12, C.muted);
  s += text(28, 470, '否则瞬态 26 Hz、连发 14 Hz、AI 突发三种频率会互相打架（加特林"抖得没规律"）。', 12, C.muted);

  s += '</svg>\n';
  return s;
}

fs.writeFileSync(path.join(OUT, 'audio-ai-flow.svg'), flow());
fs.writeFileSync(path.join(OUT, 'audio-ai-hd.svg'), hd());
console.log('wrote docs/audio-ai-flow.svg, docs/audio-ai-hd.svg');
