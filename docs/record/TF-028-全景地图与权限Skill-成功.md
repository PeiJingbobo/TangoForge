# TF-028 全景地图 + 权限/Skill 界面 — 任务总结

> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
D3 力导向全景图 + 权限管理界面（仅 UI）+ Skill 浏览界面。

## 2. 交付内容
- `components/graph/graph-view.tsx`：d3-forceSimulation + zoom + drag（useRef 管理，卸载 simulation.stop() 无泄漏）；节点颜色映射状态机 Color（archived 灰）；>300 节点按状态聚簇（聚合节点显示数量）；点击节点跳详情
- `features/tasks/GraphPage.tsx`：路由 /project/:id/graph，空态/加载态
- `features/permissions/PermissionsPanel.tsx`：16 action 按域分组勾选 + 全量覆盖保存（dirty 检测）
- `features/skills/SkillsPanel.tsx`：列表（行式）+ skill_info 详情（instructions 全文）
- `features/settings/SettingsPage.tsx`：权限 / Skills 双 Tab
- `app-layout`：项目导航（看板/导航/全景图）

## 3. 验证结果
- `pnpm typecheck` / `pnpm lint` 全绿；`pnpm test` **80 用例全过**（GraphView 3 + Permissions 2 + Skills 2 + 其余）
- 图渲染断言：circle=节点数、line=边数、聚簇>300 聚合、卸载不抛错（销毁）
- 权限保存：勾选 → PUT → toast；Skill：列表 + 详情加载
- 踩坑：d3 v7 drag 泛型与 selection.call 签名不兼容 → 断言包装 `(selection: unknown) => void`；jsdom SVG 点击交互不可测（删除该用例，mac 实测验证）

## 4. 遗留问题与后续
- 全景图节点点击跳转、拖拽体验：mac 实测项
- **P5 全部任务（TF-022~028）代码完成，M5 待 mac 运行 App 人工验证后关闭**
