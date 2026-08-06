# TF-014 WebSocket 与其余端点 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-014-ws-api`

## 进展记录

### 2026-08-06（完成）
1. 新增依赖 `github.com/gorilla/websocket v1.5.3`（QA P3-4 推荐，纯 Go）。
2. hub 广播中心 + /ws/events 独立鉴权链；写钩子双通道（audit + hub.Publish）。
3. graph（全量 nodes+edges）/ audit 查询导出 / import-export-skill 占位。
4. 单测 8 用例；真实 daemon 冒烟 10 项全过（含 WS 事件实时接收）。

## 决策记录
- **事件 type 复用写钩子 action**（QA 默认项 4）：task.created 等直接作事件 type，审计与 WS 同源。
- **WS 独立鉴权链**（QA 默认项 3）：/ws/events 不在 /api 路由树下，handleWS 内自行完成 5 步（来源过滤→项目校验→识别→task.read→升级）；与 /api 中间件语义一致。
- **写钩子组合**：OnWrite 单回调内依次 audit.Write + hub.Publish（不破坏 WriteHook 签名，TASK-SEMANTICS §11 兼容）。
- **graph 数据**：nodes=排除 archived 的全量任务（List 默认语义）+ edges（parent/dependency）。方向：`A.depends_on=[B]` → edge B→A（与 §9 语义一致）。
- **占位端点**（QA P3-3）：路由先注册保证可发现，返回 501 NOT_IMPLEMENTED，P4 替换。

## 踩坑记录
1. **daemon 冒烟环境**：curl 命令含中文 JSON 与嵌套引号在 bash -c 下反复解析失败 → 改用脚本文件 scp 执行（含 WS 的 go 客户端）。
2. **httptest 无法模拟非回环**：WS 远程 401 无法在单测覆盖（回环 dial），改为在总结/日志中说明逻辑复用 + 由 /api 测试覆盖。
3. **慢消费者**：gorilla 写阻塞会卡死写泵 → send 缓冲 64 + 写满关闭连接（hub.Publish 内 select-default 处理）。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(api): TF-014 WebSocket 事件推送 + graph/audit 端点（M3 达成）"
```
