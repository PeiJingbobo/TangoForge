# TF-027 导入导出 UI — 任务总结

> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
Markdown 导入草稿流 UI（提交→预览→确认/丢弃）+ 导出（模板模式/目标/预览/执行）+ LLM 生成模板入口。

## 2. 交付内容
- `features/imports/ImportDialog.tsx`：文件/目录/多文件/粘贴内容四形态（ParseInput），LLM 解析中 loading
- `features/imports/DraftsPanel.tsx`：pending 草稿列表（source_file/数量/时间）+ 确认（反馈 created/archived 数）/丢弃；无草稿不渲染
- `features/tasks/ExportDialog.tsx`：模板模式（default/llm）+ 目标（copy/overwrite 需路径）+ 渲染预览（content/path）+ 执行；LLM 生成模板（示例→生成→自动切 llm 模式）
- 挂载：KanbanView 工具栏（导入/导出按钮 + 草稿面板）

## 3. 验证结果
- `pnpm typecheck` / `pnpm lint` 全绿；`pnpm test` **73 用例全过**（DraftsPanel 4 + ExportDialog 3 + 其余）
- 联调三端点：POST /import、GET /import/drafts、POST confirm、DELETE discard、POST /export、POST /export/template/generate 全部经 hooks 对齐

## 4. 遗留问题与后续
- 与真实 daemon 联调冒烟：mac 实测项（P5 关闭前）
