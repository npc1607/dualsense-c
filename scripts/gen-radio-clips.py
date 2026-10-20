#!/usr/bin/env python3
"""
gen-radio-clips.py - 程序化生成 DualSense 赛车电台对讲机音效包
包含：
  1. 双音频对讲机开启提示音 (PTT Beep In) + 40ms 静电底噪
  2. 对讲机关闭提示音 (PTT Beep Out)
  3. 经过军用/电台带通滤波与饱和压缩处理的 Edge-TTS 拟真赛车工程师电台语音
"""

import os
import struct
import math
import random
import subprocess
import sys

OUT_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "assets", "radio")

def save_wav(filename, samples, sample_rate=48000):
    with open(filename, "wb") as f:
        data_len = len(samples) * 2
        f.write(b"RIFF")
        f.write(struct.pack("<I", 36 + data_len))
        f.write(b"WAVEfmt ")
        f.write(struct.pack("<IHHIIHH", 16, 1, 1, sample_rate, sample_rate * 2, 2, 16))
        f.write(b"data")
        f.write(struct.pack("<I", data_len))
        for s in samples:
            val = max(-32767, min(32767, int(s)))
            f.write(struct.pack("<h", val))

def gen_beep_in(path, sr=48000):
    samples = []
    dur = 0.075
    n = int(sr * dur)
    for i in range(n):
        t = i / sr
        env = min(1.0, i / (sr * 0.005)) * min(1.0, (n - i) / (sr * 0.005))
        tone = 0.5 * math.sin(2 * math.pi * 1200 * t) + 0.5 * math.sin(2 * math.pi * 1800 * t)
        samples.append(tone * env * 22000)
    n_sq = int(sr * 0.035)
    for i in range(n_sq):
        env = (1.0 - i / n_sq)
        noise = (random.random() * 2 - 1) * 8000 * env
        samples.append(noise)
    save_wav(path, samples, sr)

def gen_beep_out(path, sr=48000):
    samples = []
    n_sq = int(sr * 0.03)
    for i in range(n_sq):
        env = (1.0 - i / n_sq)
        noise = (random.random() * 2 - 1) * 9000 * env
        samples.append(noise)
    dur = 0.06
    n = int(sr * dur)
    for i in range(n):
        t = i / sr
        env = min(1.0, i / (sr * 0.005)) * min(1.0, (n - i) / (sr * 0.005))
        tone = math.sin(2 * math.pi * 900 * t)
        samples.append(tone * env * 18000)
    save_wav(path, samples, sr)

CLIPS = [
    ("tire_slip", "zh-CN-YunxiNeural", "注意，后轮严重打滑，稳住方向！"),
    ("engine_redline", "zh-CN-YunxiNeural", "转速逼近红线，立即升挡！"),
    ("fastest_pace", "zh-CN-YunxiNeural", "漂亮！节奏非常好，继续保持！"),
    ("heavy_crash", "zh-CN-YunxiNeural", "收到撞击，检查车身受损！"),
    ("f1_box", "en-GB-RyanNeural", "Box this lap, box box.")
]

def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    beep_in = os.path.join(OUT_DIR, "beep_in.wav")
    beep_out = os.path.join(OUT_DIR, "beep_out.wav")
    gen_beep_in(beep_in)
    gen_beep_out(beep_out)
    print("PTT beeps ready.")

    tts_bin = "/tmp/tts-env/bin/edge-tts"
    if not os.path.exists(tts_bin):
        tts_bin = "edge-tts"

    for name, voice, text in CLIPS:
        raw_mp3 = f"/tmp/{name}_raw.mp3"
        voice_filt = f"/tmp/{name}_filt.wav"
        final_wav = os.path.join(OUT_DIR, f"{name}.wav")

        print(f"Generating [{name}]: {text}")
        subprocess.run([
            tts_bin,
            "--voice", voice,
            "--text", text,
            "--write-media", raw_mp3
        ], check=True)

        filter_chain = "highpass=f=350,lowpass=f=3200,volume=2.2,acompressor=threshold=-12dB:ratio=4:attack=5:release=50"
        subprocess.run([
            "ffmpeg", "-y", "-i", raw_mp3,
            "-af", filter_chain,
            "-ar", "48000", "-ac", "1",
            voice_filt
        ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)

        subprocess.run([
            "ffmpeg", "-y",
            "-i", beep_in,
            "-i", voice_filt,
            "-i", beep_out,
            "-filter_complex", "[0:a][1:a][2:a]concat=n=3:v=0:a=1[outa]",
            "-map", "[outa]",
            "-ar", "48000", "-ac", "1",
            final_wav
        ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)

    print("All radio assets successfully compiled into", OUT_DIR)

if __name__ == "__main__":
    main()
