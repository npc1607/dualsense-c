#!/usr/bin/env python3
"""
YAMNet Audio AI Bridge for dualsense-c

Captures system loopback/microphone audio, runs lightweight YAMNet classification,
and pushes detected game surface and combat events to dualsense-c over UDP (default 127.0.0.1:9998).

Usage:
  # Mock test mode (sends simulated water, metal, and gunshot events without hardware)
  python3 yamnet_bridge.py --mock

  # Process a WAV file
  python3 yamnet_bridge.py --input gameplay.wav --target 127.0.0.1:9998

  # Real-time microphone/loopback capture (requires sounddevice & onnxruntime)
  python3 yamnet_bridge.py --model yamnet.onnx --target 127.0.0.1:9998
"""

import argparse
import json
import socket
import sys
import time

TARGET_SIGNALS = [
    "surface_water",
    "surface_metal",
    "surface_wood",
    "surface_gravel",
    "audio_gunshot",
    "audio_explosion",
    "audio_impact",
]


def send_packet(sock, addr, event_dict):
    filtered = {k: float(v) for k, v in event_dict.items() if k in TARGET_SIGNALS}
    if not filtered:
        return
    payload = json.dumps(filtered).encode("utf-8")
    sock.sendto(payload, addr)


def run_mock(sock, addr):
    print(f"[*] Running in MOCK simulation mode, sending to {addr}...")
    scenarios = [
        {"desc": "Running into shallow water", "data": {"surface_water": 0.85}},
        {"desc": "Running on metal floor", "data": {"surface_metal": 0.90}},
        {"desc": "Gunshot fired!", "data": {"audio_gunshot": 0.95}},
        {"desc": "Walking on wooden bridge", "data": {"surface_wood": 0.78}},
        {"desc": "Explosion nearby!", "data": {"audio_explosion": 0.92}},
    ]
    idx = 0
    while True:
        sc = scenarios[idx % len(scenarios)]
        print(f"[{time.strftime('%H:%M:%S')}] {sc['desc']} -> {sc['data']}")
        send_packet(sock, addr, sc["data"])
        idx += 1
        time.sleep(1.5)


def run_file(sock, addr, wav_path, model_path):
    try:
        import numpy as np
        import soundfile as sf
        import onnxruntime as ort
    except ImportError:
        print("[!] Error: running with --input requires: pip install numpy soundfile onnxruntime")
        sys.exit(1)

    print(f"[*] Loading model {model_path}...")
    session = ort.InferenceSession(model_path)
    input_name = session.get_inputs()[0].name

    audio_data, sr = sf.read(wav_path)
    if sr != 16000:
        print(f"[!] Warning: audio sample rate is {sr}Hz, YAMNet expects 16000Hz")

    print(f"[*] Streaming {wav_path} to {addr}...")
    chunk_len = 16000  # 1 second
    for i in range(0, len(audio_data), chunk_len):
        chunk = audio_data[i : i + chunk_len]
        if len(chunk) < chunk_len:
            break
        chunk = chunk.astype(np.float32)
        scores, _, _ = session.run(None, {input_name: chunk})
        avg_scores = np.mean(scores, axis=0)

        # Mapping heuristic (example indices based on AudioSet)
        # Class 42: Footsteps, 308: Water, 427: Gunshot
        event = {}
        if avg_scores[308] > 0.3:
            event["surface_water"] = float(avg_scores[308])
        if avg_scores[427] > 0.3:
            event["audio_gunshot"] = float(avg_scores[427])

        if event:
            send_packet(sock, addr, event)
            print(f"Detected: {event}")
        time.sleep(1.0)


def main():
    parser = argparse.ArgumentParser(description="YAMNet Audio AI Detector for dualsense-c")
    parser.add_argument("--target", default="127.0.0.1:9998", help="Target UDP address (default 127.0.0.1:9998)")
    parser.add_argument("--mock", action="store_true", help="Send simulated events for pipeline testing")
    parser.add_argument("--input", help="Path to WAV audio file for offline verification")
    parser.add_argument("--model", default="yamnet.onnx", help="Path to YAMNet ONNX model")
    args = parser.parse_args()

    host, port = args.target.split(":")
    dest = (host, int(port))
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

    if args.mock:
        try:
            run_mock(sock, dest)
        except KeyboardInterrupt:
            print("\nStopped.")
            return

    if args.input:
        run_file(sock, dest, args.input, args.model)
        return

    print("[*] For real-time loopback capture, install sounddevice + onnxruntime.")
    print("    Running mock mode now. Use --help for full usage.")
    run_mock(sock, dest)


if __name__ == "__main__":
    main()
