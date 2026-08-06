# TF-019 Markdown 导出与模板 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
实现从结构化任务数据重建 Markdown：默认模板（升级为可往返格式）、自定义模板覆盖、LLM 生成模板（校验后落盘 + 更新项目配置）、overwrite/copy 写盘目标；替换 TF-014 的 export 占位端点。

## 2. 交付内容
- **新增/修改文件**：
  - `internal/exporter/templates/default.tmpl` — **升级**：Front Matter（title/generated_at）+ `##/###` 层级标题 + `- 状态: <key>`/优先级/标签/负责人元数据行（往返一致）
  - `internal/exporter/exporter.go` — Service：`Render`（任务树展平 → 模板渲染 → 写盘 → export.complete 事件）、`GenerateTemplate`（LLM → Parse 校验 → generated-template.tmpl → config.template_path 更新）、loadTemplate/flattenTree/resolveTargetPath；错误集 `EXPORT_FAILED`/`TEMPLATE_INVALID`
  - `internal/exporter/exporter_test.go` — 10 用例（默认渲染断言、自定义模板、overwrite 缺 path、llm 未生成、项目未导入、LLM 模板生成/非法/未配置、flatten 层级）
  - `internal/api/handlers_export.go` — `POST /api/export`、`POST /api/export/template/generate` 替换占位
  - `internal/api/handlers_export_test.go` — 5 用例（copy 默认写盘、overwrite 缺 path 422、agent 403、template generate + llm 模式渲染、非法模板 422）
  - `internal/api/server.go` — exporterSvc 组装（OnExport 双通道 audit/hub）
  - `internal/api/errors.go` — 映射 `EXPORT_FAILED`/`TEMPLATE_INVALID`(422)
  - `docs/TASK-SEMANTICS.md` — 新增 §18；`docs/task/TASKS.md`/`OVERVIEW.md` 状态同步
- **关键实现点**：
  1. 往返格式：`- 状态: <key>` 元数据行 + 标题层级（LLM 可读、任意状态机通用）
  2. text/template 递归替代：深度优先展平 + `header` 函数（2+level 个 #）
  3. LLM 生成模板必须通过 template.Parse 校验才写盘（非法拒绝）
  4. overwrite（path 必填）/ copy（缺省 export.md）均写盘 + 响应 content

## 3. 验证结果
- `go vet ./...` → 干净
- `CGO_ENABLED=0 go test ./internal/exporter/... ./internal/api/...` → ok
- `CGO_ENABLED=0 go test ./...` → **全仓全绿**
- `bash ./scripts/check_coverage.sh` → **91.7% ≥ 90%** 通过
- 全端点 501 占位已清零（import/export/skill 全部替换）

## 4. 遗留问题与后续
- 真实 LLM 往返验证（导出 → 重新导入）留待 M4 人工验证（配置真实 DeepSeek 后执行）。
- TF-017 `export_markdown` MCP 工具与 TF-021 CLI export 子命令将复用本 Service。
- 全仓首跑曾出现 TestExport_TemplateGenerateInvalid 偶发失败（缓存），单跑通过，未复现，持续观察。
