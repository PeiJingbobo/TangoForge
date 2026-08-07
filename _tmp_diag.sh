#!/usr/bin/env bash
# 诊断 ui_token 匹配链路（主进程 readUiToken 同款逻辑）
set -e
CFG="$HOME/.taskboard-app/config.yaml"
echo "=== config.yaml ui_token/api_token 行 ==="
grep -nE "^(ui_token|api_token):" "$CFG" || echo "(not found)"

echo "=== 正则提取（主进程同款）==="
TOKEN=$(python3 -c "
import re, sys
s = open(sys.argv[1]).read()
m = re.search(r'^ui_token:\s*(.+?)\s*$', s, re.M)
print(m.group(1) if m else '')
" "$CFG")
echo "len=${#TOKEN} token=${TOKEN:0:6}..."

echo "=== 带正则 token 请求 /api/config ==="
curl -s -m 2 -H "X-UI-Token: $TOKEN" http://127.0.0.1:19810/api/config | head -c 100
echo

echo "=== 不带 token 请求（对照）==="
curl -s -m 2 http://127.0.0.1:19810/api/config | head -c 100
echo

echo "=== daemon 日志（最近 403/denied）==="
grep -iE "403|denied" /tmp/tangoforge-daemon.log 2>/dev/null | tail -3 || echo "(no log)"
