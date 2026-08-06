# -*- coding: utf-8 -*-
import io

p = 'docs/task/TASKS.md'
s = io.open(p, encoding='utf-8').read()
old = """### TF-026 任务详情与导航视图（P1，依赖 TF-023、TF-008）

- **涉及模块**：`app/src/features/tasks`
- **描述**：任务详情/编辑（字段编辑、归档/还原、依赖编辑 + 无环提示）；导航三视图：树形列表（折叠/搜索）、时间线、状态分类。
- **验收标准**：
  - [ ] 组件测试：详情编辑、依赖环错误展示
  - [ ] 树形视图与后端树结构一致
- **产出文件**：`app/src/features/tasks/TaskDetail.tsx`、`app/src/features/tasks/TaskForm.tsx`、`app/src/components/common/TreeNav.tsx` 等"""
new = """### TF-026 任务详情与导航视图（P1，依赖 TF-023、TF-008）✅ 已完成

- **涉及模块**：`app/src/features/tasks`
- **描述**：任务详情/编辑（字段编辑、归档/还原、依赖编辑 + 无环提示）；导航三视图：树形列表（折叠/搜索）、时间线、状态分类。
- **验收标准**：
  - [x] 组件测试：详情编辑（TaskForm 5 例 + TaskDetail 3 例）、依赖环错误展示（CIRCULAR_DEPENDENCY → toast 无环提示）
  - [x] 树形视图与后端树结构一致（TreeNav 直接消费后端 tree，4 例）
- **产出文件**：`app/src/features/tasks/TaskDetail.tsx`、`app/src/features/tasks/TaskForm.tsx`、`app/src/features/tasks/NavViews.tsx`、`app/src/components/common/TreeNav.tsx`、`app/src/features/tasks/constants.ts`
- **总结文件**：`docs/record/TF-026-任务详情与导航-成功.md`"""
assert old in s, 'TF-026 未匹配'
s = s.replace(old, new)
io.open(p, 'w', encoding='utf-8', newline='\n').write(s)

p2 = 'docs/task/OVERVIEW.md'
s2 = io.open(p2, encoding='utf-8').read()
s2 = s2.replace('| TF-026 | 任务详情与导航视图 | P5 | P1 | TF-023, TF-008 | ⬜ | – |',
                '| TF-026 | 任务详情与导航视图 | P5 | P1 | TF-023, TF-008 | ✅ | `docs/record/TF-026-任务详情与导航-成功.md` |')
s2 = s2.replace('待开始 6 · 进行中 0 · 已完成 25', '待开始 5 · 进行中 0 · 已完成 26')
s2 = s2.replace('[4/7] ✅✅✅✅⬜⬜⬜', '[5/7] ✅✅✅✅✅⬜⬜')
s2 = s2.replace('TF-025 ✅ ▸ TF-026 ▸', 'TF-025 ✅ TF-026 ✅ ▸')
io.open(p2, 'w', encoding='utf-8', newline='\n').write(s2)
print('TF-026 docs updated')
