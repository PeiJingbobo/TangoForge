#!/usr/bin/env bash
# 将 TangoForge CLI 注册到全局（macOS / Linux，幂等）
# 用法：./register-cli.sh（随 App 分发于 resources/bin/）
# 创建 ~/bin/tangoforge 符号链接并把 ~/bin 注入 shell PATH。
set -e

BIN_DIR="$(cd "$(dirname "$0")" && pwd)"
CLI="$BIN_DIR/tangoforge"
if [ ! -f "$CLI" ]; then
  echo "!! 未找到 CLI：$CLI"
  exit 1
fi

TARGET_DIR="$HOME/bin"
mkdir -p "$TARGET_DIR"
ln -sf "$CLI" "$TARGET_DIR/tangoforge"

# 选择 shell 配置文件（mac 默认 zsh；存在 bash_profile 则用 bash）。
RC_FILE="$HOME/.zshrc"
if [ -f "$HOME/.bash_profile" ]; then
  RC_FILE="$HOME/.bash_profile"
fi
if [ -f "$HOME/.bashrc" ] && [ ! -f "$HOME/.bash_profile" ]; then
  RC_FILE="$HOME/.bashrc"
fi

if ! grep -qF "export PATH=\"$TARGET_DIR" "$RC_FILE" 2>/dev/null; then
  printf '\n# TangoForge CLI\nexport PATH="%s:$PATH"\n' "$TARGET_DIR" >> "$RC_FILE"
fi

echo "已注册 CLI：$TARGET_DIR/tangoforge -> $CLI（PATH 已写入 $RC_FILE）"
echo "验证：新开终端执行 tangoforge --help（需守护进程运行；App 启动会自动拉起）"
