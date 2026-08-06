# -*- coding: utf-8 -*-
import io

p = 'docs/task/TASKS.md'
s = io.open(p, encoding='utf-8').read()
old = """### TF-025 看板视图（P0，依赖 TF-023、TF-006、TF-007）

- **涉及模块**：`app/src/features/tasks`、`app/src/components/kanban`
- **描述**：按状态机动态生成列；卡片（标题、优先级色带红高灰低、标签徽章哈希着色、assignee）；拖拽触发状态流转 Mutation，`INVALID_TRANSITION` 回滚 + toast；虚拟滚动（≥ 1000 任务不卡）；按标签/状态过滤、搜索。
- **验收标准**：
  - [ ] 组件测试（RTL）：渲染、拖拽调用、非法流转回滚提示
  - [ ] 5,000 条 mock 数据滚动流畅（虚拟滚动生效）
- **产出文件**：`app/src/components/kanban/*`、`app/src/features/tasks/KanbanView.tsx`"""
new = """### TF-025 看板视图（P0，依赖 TF-023、TF-006、TF-007）✅ 已完成

- **涉及模块**：`app/src/features/tasks`、`app/src/components/kanban`
- **描述**：按状态机动态生成列；卡片（标题、优先级色带红高灰低、标签徽章哈希着色、assignee）；拖拽触发状态流转 Mutation，`INVALID_TRANSITION` 回滚 + toast；虚拟滚动（≥ 1000 任务不卡）；按标签/状态过滤、搜索。
- **验收标准**：
  - [x] 组件测试（RTL）：渲染、拖拽调用（drag-logic 单测 + useKanban 乐观回滚 3 例）、非法流转回滚提示（INVALID_TRANSITION toast + 回滚）
  - [ ] 5,000 条 mock 数据滚动流畅（虚拟滚动生效，mac 实测项；实现：列内 @tanstack/react-virtual，estimateSize 108）
- **产出文件**：`app/src/components/kanban/*`（5 个）、`app/src/features/tasks/KanbanView.tsx`、`app/src/hooks/useKanban.ts`
- **总结文件**：`docs/record/TF-025-看板视图-成功.md`"""
assert old in s, 'TASKS.md 未匹配'
s = s.replace(old, new)
io.open(p, 'w', encoding='utf-8', newline='\n').write(s)

p2 = 'docs/task/OVERVIEW.md'
s2 = io.open(p2, encoding='utf-8').read()
s2 = s2.replace('| TF-025 | 看板视图 | P5 | P0 | TF-023, TF-006, TF-007 | ⬜ | – |',
                '| TF-025 | 看板视图 | P5 | P0 | TF-023, TF-006, TF-007 | ✅ | `docs/record/TF-025-看板视图-成功.md` |')
s2 = s2.replace('待开始 7 · 进行中 0 · 已完成 24', '待开始 6 · 进行中 0 · 已完成 25')
s2 = s2.replace('[3/7] ✅✅✅⬜⬜⬜⬜', '[4/7] ✅✅✅✅⬜⬜⬜')
s2 = s2.replace('TF-022 ✅ TF-023 ✅ TF-024 ✅ ▸ TF-025 ▸', 'TF-022 ✅ TF-023 ✅ TF-024 ✅ TF-025 ✅ ▸')
io.open(p2, 'w', encoding='utf-8', newline='\n').write(s2)
print('TASKS/OVERVIEW updated for TF-025')
