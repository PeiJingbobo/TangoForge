#!/bin/bash
set -e
cd ~/HD-DATA/Coding/TangoForge
git add -A
export PATH=$HOME/node-sdk/bin:$HOME/go-sdk/go/bin:$HOME/HD-DATA/Coding/TangoForge/node_modules/.bin:$PATH
git commit -m "fix(ci): 修复 GitHub Actions——golangci-lint v2 + go matrix 升级 + 全量 lint 问题清零

backend-ci 失败根因（lint 从未真正执行过）：
- golangci-lint-action version latest 解析至 v1.64.8（go1.24 构建），无法处理
  go.mod 的 go 1.25.5 目标 → 显式 v2.12.2（go1.25/1.26 支持）
- matrix go 1.22/1.23 远低于 go.mod 要求 → 升级 ['1.25','1.26']（cross-build 同步）
- .golangci.yml 迁移 v2 格式（linters.exclusions 替代 issues.exclude-*，
  formatters 独立段）；排除 node_modules 与既有 stutter 命名

golangci-lint v2 首跑暴露 36 个既有问题（v1 从未成功加载配置），全部修复：
- 真问题：errcheck 4（Close 检查）、unused 1（nowRFC3339 死代码）、
  staticcheck 6（Body.Bytes/fmt.Fprintf/嵌入式选择器）、revive builtin 遮蔽
  5（close/max 重命名）、error-strings 3、unused-parameter 10、包注释、
  indent-error-flow 1
- 格式：golangci-lint fmt（gofumpt）63 文件纯格式

验证：golangci-lint run 0 issues + go test ./... 13 包全绿"
echo COMMIT_DONE
git log --oneline -1
git push origin main 2>&1 | tail -1
