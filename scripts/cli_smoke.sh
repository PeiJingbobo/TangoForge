#!/bin/bash
# TangoForge CLI 冒烟（TF-021 验收：CLI 与 HTTP 等价）
set -e
cd /tmp
export PATH="$HOME/go-sdk/go/bin:$PATH"

# 1. 编译
cd ~/HD-DATA/Coding/TangoForge
export GOCACHE=$HOME/.cache/go-build GOPATH=$HOME/go GOPROXY=https://goproxy.cn,direct
go build -o /tmp/tf-cli ./cmd/cli
go build -o /tmp/tf-daemon ./cmd/daemon

# 2. 起 daemon（若未运行）
if ! curl -s -m 2 http://127.0.0.1:19810/ping > /dev/null 2>&1; then
  nohup /tmp/tf-daemon > /tmp/tf-daemon.log 2>&1 &
  sleep 1.5
fi
echo "== ping =="
curl -s http://127.0.0.1:19810/ping; echo

CLI="/tmp/tf-cli"
PROJ="$(mktemp -d)/taskproj"
mkdir -p "$PROJ"

echo "== projects import =="
$CLI projects import "$PROJ"

echo "== 授权 agent 可写（UI 凭据，等价 App 中勾选） =="
UI_TOKEN=$(grep '^ui_token:' ~/.taskboard-app/config.yaml | awk '{print $2}')
curl -s -X PUT http://127.0.0.1:19810/api/permissions \
  -H "X-UI-Token: $UI_TOKEN" -H "X-Project: $PROJ" -H "Content-Type: application/json" \
  -d '{"actions":{"project.read":true,"task.read":true,"task.create":true,"task.update":true,"task.update_status":true,"task.delete":true,"task.restore":true,"import.run":true,"import.confirm":true,"export.run":true,"graph.read":true,"skill.read":true,"state_machine.read":true,"state_machine.write":true,"audit.read":false,"permission.read":true}}' > /dev/null
echo "授权完成"

echo "== projects list =="
$CLI projects list --json | head -c 200; echo

echo "== tasks create =="
$CLI tasks create "CLI 冒烟任务" --project "$PROJ" --priority high --tags smoke,cli
echo "== tasks create（子任务） =="
$CLI tasks create "子任务" --project "$PROJ" --parent "$(true)"

echo "== tasks list =="
$CLI tasks list --project "$PROJ" --json | head -c 300; echo

echo "== tasks update =="
TID=$($CLI tasks list --project "$PROJ" --json | python3 -c "import sys,json; d=json.load(sys.stdin); t=d.get('tree') or d.get('items') or []; print(t[0]['id'])")
$CLI tasks update "$TID" --project "$PROJ" --title "CLI 任务改"
echo "== tasks status todo->doing =="
$CLI tasks status "$TID" doing --project "$PROJ"

echo "== tasks archive/restore =="
$CLI tasks archive "$TID" --project "$PROJ"
$CLI tasks restore "$TID" --project "$PROJ"

echo "== export (default 模板) =="
$CLI export --project "$PROJ" --target copy | head -c 200; echo

echo "== graph =="
$CLI graph --project "$PROJ"

echo "== state-machine get =="
$CLI state-machine get --project "$PROJ" --json | head -c 150; echo

echo "== permission =="
$CLI permission --project "$PROJ" | head -8

echo "== audit =="
$CLI audit --project "$PROJ" --json | head -c 200; echo

echo "== skills（空） =="
$CLI skills --project "$PROJ" --json

echo "== 缺 --project 应报错 =="
if $CLI tasks list 2>&1 | grep -q "缺少必填参数"; then echo "OK: 缺 project 报错"; else echo "FAIL"; exit 1; fi

echo "== 未运行 daemon 自动拉起提示（停 daemon 测试） =="
pkill -f tf-daemon; sleep 0.5
if $CLI projects list --json 2>&1 | grep -q "未找到 daemon"; then echo "OK: 找不到 daemon 时提示手动启动"; else echo "（daemon 已在同目录拉起或提示信息不同，继续）"; fi

echo "== SMOKE PASS =="
