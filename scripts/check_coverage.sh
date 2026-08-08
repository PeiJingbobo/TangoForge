#!/usr/bin/env bash
# 检查指定包的测试覆盖率是否达到阈值（默认 90%，可用 THRESHOLD 覆盖）。
# 用法：./scripts/check_coverage.sh [PKG]
#   THRESHOLD=95 PKG=./internal/task/... ./scripts/check_coverage.sh
set -euo pipefail

THRESHOLD="${THRESHOLD:-90.0}"
PKG="${1:-./internal/task/...}"

out="$(go test -cover "$PKG")"
echo "$out"

cov="$(echo "$out" | tail -1 | sed -n 's/.*coverage: \([0-9.]*\)% of statements.*/\1/p')"
if [[ -z "$cov" ]]; then
  echo "ERROR: 无法从 go test 输出解析覆盖率" >&2
  exit 1
fi

if awk "BEGIN{exit !($cov < $THRESHOLD)}"; then
  echo "ERROR: 覆盖率 ${cov}% 低于阈值 ${THRESHOLD}%（$PKG）" >&2
  exit 1
fi

echo "OK: 覆盖率 ${cov}% >= ${THRESHOLD}%（$PKG）"
