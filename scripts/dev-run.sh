#!/usr/bin/env bash
# TangoForge 桌面端开发启动（macOS）
# 用法：在 macOS 上执行 scripts/dev-run.sh
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="$HOME/node-sdk/bin:$ROOT/node_modules/.bin:$PATH"
export COREPACK_NPM_REGISTRY=https://registry.npmmirror.com
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/

echo "==> 1/3 守护进程探活（127.0.0.1:19810）"
if curl -sf -m 2 http://127.0.0.1:19810/ping >/dev/null 2>&1; then
  echo "    daemon 已在运行"
else
  echo "    拉起 daemon（退出 App 后仍常驻）"
  "$ROOT/bin/tangoforge-daemon" >/tmp/tangoforge-daemon.log 2>&1 &
  disown
  sleep 1
fi

echo "==> 2/3 UI 凭据（~/.taskboard-app/config.yaml ui_token）"
if grep -q '^ui_token:' "$HOME/.taskboard-app/config.yaml" 2>/dev/null; then
  echo "    ui_token 已配置"
else
  echo "    !! 未找到 ui_token，UI 将以受限身份运行"
fi

echo "==> 3/3 启动 Electron（electron-vite dev）"
cd "$ROOT/app" && corepack pnpm dev
