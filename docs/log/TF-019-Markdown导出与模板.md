# TF-019 Markdown 导出与模板 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-019-exporter`

## 进展记录

### 2026-08-06（完成）
1. 升级 `internal/exporter/templates/default.tmpl`：Front Matter + `##/###` 层级标题 + `- 状态: <key>` 元数据行（可被 parser 往返导入）+ 优先级/标签/负责人。
2. `internal/exporter/exporter.go`：Service（Render / GenerateTemplate）、模板函数（join/header）、flattenTree（Level）、loadTemplate（default 配置覆盖/内置嵌入、llm 已生成）、resolveTargetPath、错误集（EXPORT_FAILED/TEMPLATE_INVALID/ErrProjectNotFound）。
3. `internal/api/handlers_export.go`：替换占位；api.Server 组装 exporterSvc（OnExport 双通道 audit + hub，事件 export.complete）。
4. 测试：exporter_test.go 10 用例 + handlers_export_test.go 5 用例；删除过时 TestPlaceholders_NOT_IMPLEMENTED（全端点已落地）。

## 决策记录
- **往返格式**（QA P4-1）：用 `- 状态: <key>` 元数据行而非 checkbox（状态机可自定义，key 对 LLM 更明确）；标题层级 `##`（顶层）/`###`（子任务）表达父子。
- **模板递归**：text/template 不支持直接递归 → 深度优先展平为 FlatTask{Level}，`header` 函数输出 2+level 个 `#`。
- **LLM 模板校验**：template.Parse（与渲染同 funcs）通过才写盘 + 更新 config.template_path；非法不落盘。
- **overwrite/copy**：overwrite path 必填；copy 缺省 {workdir}/.taskboard/export.md；均写盘 + 响应 content（QA P4-1 Q11-A）。
- **事件**：export.complete（target=path），审计一条；LLM provider 每次取最新（热重载）。

## 踩坑记录
1. TestGenerateTemplate_OK 首跑失败：llm 渲染无任务行（未 seed）→ seedTasks 后通过。
2. handlers_placeholder.go 残留 `net/http` import 未用 → 重写为空说明文件。
3. 全仓首跑 TestExport_TemplateGenerateInvalid 偶发失败（缓存），单跑/重跑通过——疑似并行测试下 httptest 端口时序，未复现，继续观察。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(exporter): TF-019 Markdown 导出与模板（默认/自定义/LLM 模板 + overwrite/copy）"
```
