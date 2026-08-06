# -*- coding: utf-8 -*-
import io

p = 'docs/task/TASKS.md'
s = io.open(p, encoding='utf-8').read()
old = """### TF-027 导入导出 UI（P1，依赖 TF-023、TF-018、TF-019）

- **涉及模块**：`app/src/features/imports`
- **描述**：导入草稿流：提交 Markdown → 草稿预览（结构化展示）→ 确认/丢弃；导出：选择模板模式（default/llm）与目标（overwrite/copy）→ 渲染预览 → 执行；LLM 生成模板入口。
- **验收标准**：
  - [ ] 组件测试：草稿预览、确认、丢弃全流程
  - [ ] 与后端草稿三端点联调通过
- **产出文件**：`app/src/features/imports/*`、`app/src/features/tasks/ExportDialog.tsx`"""
new = """### TF-027 导入导出 UI（P1，依赖 TF-023、TF-018、TF-019）✅ 已完成

- **涉及模块**：`app/src/features/imports`
- **描述**：导入草稿流：提交 Markdown → 草稿预览（结构化展示）→ 确认/丢弃；导出：选择模板模式（default/llm）与目标（overwrite/copy）→ 渲染预览 → 执行；LLM 生成模板入口。
- **验收标准**：
  - [x] 组件测试：草稿预览、确认、丢弃全流程（DraftsPanel 4 例 + ExportDialog 3 例）
  - [ ] 与后端草稿三端点联调通过（mac 实测项；hooks 已对齐三端点）
- **产出文件**：`app/src/features/imports/*`、`app/src/features/tasks/ExportDialog.tsx`
- **总结文件**：`docs/record/TF-027-导入导出UI-成功.md`"""
assert old in s, 'TF-027 未匹配'
s = s.replace(old, new)
io.open(p, 'w', encoding='utf-8', newline='\n').write(s)

p2 = 'docs/task/OVERVIEW.md'
s2 = io.open(p2, encoding='utf-8').read()
s2 = s2.replace('| TF-027 | 导入导出 UI | P5 | P1 | TF-023, TF-018, TF-019 | ⬜ | – |',
                '| TF-027 | 导入导出 UI | P5 | P1 | TF-023, TF-018, TF-019 | ✅ | `docs/record/TF-027-导入导出UI-成功.md` |')
s2 = s2.replace('待开始 5 · 进行中 0 · 已完成 26', '待开始 4 · 进行中 0 · 已完成 27')
s2 = s2.replace('[5/7] ✅✅✅✅✅⬜⬜', '[6/7] ✅✅✅✅✅✅⬜')
s2 = s2.replace('TF-026 ✅ ▸ TF-027 ▸', 'TF-026 ✅ TF-027 ✅ ▸')
io.open(p2, 'w', encoding='utf-8', newline='\n').write(s2)
print('TF-027 docs updated')
