#!/usr/bin/env bash
# 将 TangoForge CLI 注册到全局（macOS / Linux，幂等）
# 用法：./register-cli.sh（随 App 分发于 resources/bin/）
# 创建 ~/bin/tangoforge 符号链接并把 ~/bin 注入 shell PATH。
set -e

BIN_DIR="$(cd "$(dirname "$0")" && pwd)"
CLI="$BIN_DIR/tangoforge"
if [ ! -f "$CLI" ]; then
  # 仓库场景：脚本在 scripts/，CLI 在仓库根 bin/。
  REPO_BIN="$(cd "$BIN_DIR/../bin" 2>/dev/null && pwd)"
  if [ -n "$REPO_BIN" ] && [ -f "$REPO_BIN/tangoforge" ]; then
    BIN_DIR="$REPO_BIN"
    CLI="$BIN_DIR/tangoforge"
  fi
fi
if [ ! -f "$CLI" ]; then
  echo "!! 未找到 CLI：$CLI"
  exit 1
fi

TARGET_DIR="$HOME/bin"
mkdir -p "$TARGET_DIR"
ln -sf "$CLI" "$TARGET_DIR/tangoforge"

# PATH 注入所有 shell profile（macOS 默认登录 shell 为 zsh，zsh 不加载
# bash_profile）：.zshrc 总是写入；.bash_profile/.bashrc 存在时一并写入。
INJECTED=""
for rc in "$HOME/.zshrc" "$HOME/.bash_profile" "$HOME/.bashrc"; do
  [ -f "$rc" ] || [ "$(basename "$rc")" = ".zshrc" ] || continue
  if ! grep -qF "export PATH=\"$TARGET_DIR" "$rc" 2>/dev/null; then
    printf '\n# TangoForge CLI\nexport PATH="%s:$PATH"\n' "$TARGET_DIR" >> "$rc"
  fi
  INJECTED="$INJECTED $rc"
done

echo "已注册 CLI：$TARGET_DIR/tangoforge -> $CLI（PATH 已写入：$INJECTED）"
echo "验证：新开终端执行 tangoforge --help（需守护进程运行；App 启动会自动拉起）"
