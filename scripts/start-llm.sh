#!/usr/bin/env bash
# start-llm.sh - 启动 DualSense 本地伴生端侧小模型 (Qwen2.5-0.5B on port 9995)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${ROOT_DIR}/models/bin"
MODEL_FILE="${ROOT_DIR}/models/qwen/qwen2.5-0.5b-instruct-q4_k_m.gguf"
PORT="${DS5_LLM_PORT:-9995}"
PID_FILE="/tmp/ds5-llm-server.pid"

action="${1:-start}"

check_prereqs() {
    if [ ! -f "$MODEL_FILE" ]; then
        echo "错误: 未找到模型文件: $MODEL_FILE"
        echo "请先下载 Qwen2.5-0.5B-Instruct GGUF 模型到 models/qwen/ 目录下。"
        exit 1
    fi
    if [ ! -x "${BIN_DIR}/llama-server" ]; then
        echo "错误: 未找到推理程序: ${BIN_DIR}/llama-server"
        exit 1
    fi
}

case "$action" in
    start)
        check_prereqs
        if ss -tuln | grep -q ":${PORT} "; then
            echo "DualSense LLM 服务已在端口 ${PORT} 上运行。"
            exit 0
        fi
        echo "正在启动 DualSense 本地伴生小模型服务 (Qwen2.5-0.5B on 127.0.0.1:${PORT})..."
        LD_LIBRARY_PATH="${BIN_DIR}:${LD_LIBRARY_PATH:-}" setsid nohup "${BIN_DIR}/llama-server" \
            -m "$MODEL_FILE" \
            --host 127.0.0.1 --port "$PORT" \
            -c 1024 -t 4 --log-disable \
            > /tmp/ds5-llm-server.log 2>&1 < /dev/null &
        echo $! > "$PID_FILE"
        sleep 1
        if ss -tuln | grep -q ":${PORT} "; then
            echo "启动成功！服务监听中: http://127.0.0.1:${PORT}/completion"
        else
            echo "启动失败，请检查日志: /tmp/ds5-llm-server.log"
            exit 1
        fi
        ;;
    stop)
        echo "正在停止 DualSense 本地 LLM 服务..."
        if [ -f "$PID_FILE" ]; then
            p="$(cat "$PID_FILE" 2>/dev/null)"
            if [ -n "$p" ]; then
                kill "$p" 2>/dev/null || true
            fi
            rm -f "$PID_FILE"
        else
            killall -q llama-server 2>/dev/null || true
        fi
        echo "已停止。"
        ;;
    status)
        if ss -tuln | grep -q ":${PORT} "; then
            echo "状态: 正在运行 (端口 ${PORT})"
            curl -s "http://127.0.0.1:${PORT}/health" 2>/dev/null || echo ""
        else
            echo "状态: 未运行"
        fi
        ;;
    test)
        check_prereqs
        echo "正在测试本地小模型推理 (127.0.0.1:${PORT})..."
        curl --noproxy "*" -s "http://127.0.0.1:${PORT}/completion" \
            -H "Content-Type: application/json" \
            -d '{"prompt": "<|im_start|>system\n你是赛车领航员，用10字以内简短给出指令。<|im_end|>\n<|im_start|>user\n前方盲峰接急弯！<|im_end|>\n<|im_start|>assistant\n", "n_predict": 20}' \
            | python3 -c 'import sys, json; d=json.load(sys.stdin); print("回复:", d.get("content","").strip()); print("速度:", d.get("timings",{}).get("predicted_per_second"), "t/s")'
        ;;
    *)
        echo "用法: $0 {start|stop|status|test}"
        exit 1
        ;;
esac
