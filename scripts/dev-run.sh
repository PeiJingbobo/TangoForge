#!/usr/bin/env bash
# TangoForge 桌面端开发启动（macOS）
# 用法：在 macOS 上执行 scripts/dev-run.sh
#
# 守护进程策略：探活 + 版本过期自动重启——
#   1) 未运行            → 拉起最新二进制
#   2) 运行中但二进制已更新（构建 mtime 晚于进程启动 / 旧文件被替换为 deleted）→ kill 重启
#   3) 运行中且为最新    → 复用（不中断连接）
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="$HOME/node-sdk/bin:$ROOT/node_modules/.bin:$PATH"
export COREPACK_NPM_REGISTRY=https://registry.npmmirror.com
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/

DAEMON_BIN="$ROOT/bin/tangoforge-daemon"
DAEMON_PORT=19810
DAEMON_URL="http://127.0.0.1:$DAEMON_PORT"

# 探测运行中的 daemon 是否版本过期（需要重启）。
# 判据：二进制 mtime 晚于进程启动时间，或 lsof 显示进程持有的二进制已 deleted（go build 原子替换）。
daemon_is_stale() {
  local pid
  pid="$(lsof -ti:$DAEMON_PORT 2>/dev/null | head -1)"
  [ -z "$pid" ] && return 0 # 未运行 → 视为需要拉起

  local bin_mtime=0 start_epoch=0
  [ -f "$DAEMON_BIN" ] && bin_mtime="$(stat -f %m "$DAEMON_BIN" 2>/dev/null || echo 0)"

  local start
  start="$(ps -o lstart= -p "$pid" 2>/dev/null)"
  if [ -n "$start" ]; then
    start_epoch="$(date -j -f "%a %b %e %H:%M:%S %Y" "$start" +%s 2>/dev/null || echo 0)"
  fi

  # 二进制被替换（旧 inode 已 deleted）或构建时间晚于进程启动 → 过期
  if lsof -p "$pid" 2>/dev/null | grep -q "tangoforge-daemon.*deleted"; then
    return 0
  fi
  [ "$bin_mtime" -gt "$start_epoch" ] && return 0
  return 1
}

# 杀尽占用端口的所有进程（多 PID 场景，前两次踩坑：单 PID kill 静默失败）
kill_all_on_port() {
  local p
  for p in $(lsof -ti:$DAEMON_PORT 2>/dev/null); do
    kill -9 "$p" 2>/dev/null || true
  done
  sleep 1
}

start_daemon() {
  "$DAEMON_BIN" >/tmp/tangoforge-daemon.log 2>&1 &
  disown
  # 等待就绪（最多 5s）
  local i
  for i in $(seq 1 10); do
    curl -sf -m 1 "$DAEMON_URL/ping" >/dev/null 2>&1 && return 0
    sleep 0.5
  done
  return 1
}

echo "==> 1/3 守护进程检查（$DAEMON_URL）"
if [ ! -f "$DAEMON_BIN" ]; then
  echo "    !! 未找到 $DAEMON_BIN，请先在 macOS 构建（go build -o bin/tangoforge-daemon ./cmd/daemon）"
  exit 1
fi

if curl -sf -m 2 "$DAEMON_URL/ping" >/dev/null 2>&1; then
  if daemon_is_stale; then
    echo "    检测到二进制已更新，自动重启 daemon…"
    kill_all_on_port
    if start_daemon; then
      echo "    daemon 已重启并就绪"
    else
      echo "    !! daemon 重启失败，查看 /tmp/tangoforge-daemon.log"
      exit 1
    fi
  else
    echo "    daemon 已在运行（版本最新，复用）"
  fi
else
  echo "    daemon 未运行，拉起最新二进制（退出 App 后仍常驻）"
  kill_all_on_port
  if start_daemon; then
    echo "    daemon 已就绪"
  else
    echo "    !! daemon 启动失败，查看 /tmp/tangoforge-daemon.log"
    exit 1
  fi
fi

echo "==> 2/3 UI 凭据（~/.taskboard-app/config.yaml ui_token）"
if grep -q '^ui_token:' "$HOME/.taskboard-app/config.yaml" 2>/dev/null; then
  echo "    ui_token 已配置"
else
  echo "    !! 未找到 ui_token，UI 将以受限身份运行"
fi

echo "==> 3/3 启动 Electron（electron-vite dev）"
# 调试模式：./dev-run.sh debug（或 TF_DEBUG=1）→ 打开渲染进程 DevTools
if [ "${1:-}" = "debug" ] || [ "${TF_DEBUG:-}" = "1" ]; then
  echo "    调试模式：渲染进程 DevTools 将打开（查看 UI 层报错/白屏原因）"
  cd "$ROOT/app" && ELECTRON_DEBUG=1 corepack pnpm dev
else
  cd "$ROOT/app" && corepack pnpm dev
fi
