#!/usr/bin/env bash
# 停止 TangoForge 守护进程（macOS / Linux）
# 用法：scripts/stop-daemon.sh
#
# 守护进程为 detached 常驻进程（dev-run.sh / App 启动拉起），退出 App 后仍在；
# 本脚本按端口 19810 结束其进程（与 dev-run.sh 的探活/杀进程逻辑一致）。
set -e

PORT="${TANGOFORGE_DAEMON_PORT:-19810}"
URL="http://127.0.0.1:$PORT"

# 探活：未运行则直接提示退出。
if ! curl -sf -m 1 "$URL/ping" >/dev/null 2>&1; then
  echo "守护进程未在运行（$URL）"
  exit 0
fi

echo "==> 停止守护进程（端口 $PORT）"
pids="$(lsof -ti:$PORT 2>/dev/null || true)"
if [ -z "$pids" ]; then
  echo "    !! 端口 $PORT 无进程（探活成功但 lsof 未找到，可能为其它监听）"
  exit 1
fi
for p in $pids; do
  kill -9 "$p" 2>/dev/null || true
done
sleep 1

# 确认停止。
if curl -sf -m 1 "$URL/ping" >/dev/null 2>&1; then
  echo "    !! 停止失败，端口 $PORT 仍响应"
  exit 1
fi
echo "    已停止（PID: $pids）"
