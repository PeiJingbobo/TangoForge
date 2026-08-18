# TF-055 PERT 任务图重构 — 任务总结

> 结果：成功　|　日期：2026-08-18　|　执行人：ai

## 1. 任务范围

重构全景图的 PERT 视图，使复杂依赖在圆形节点、共享端点、可选边路径高亮和可自由平移的工作流画布中清晰呈现。

## 2. 交付内容

- `app/src/lib/pert-layout.ts`：扩大层/行间距、节点排斥区、72 点碰撞采样、跨层安全走廊多段贝塞尔、共享起点/切线、根谱系继承与路径追踪。
- `app/src/lib/pert-layout.ts`：新增 18 轮父子中位线纵向松弛、层内动态重排与 PAVA 最小间距投影；真实 TF-050 → TF-051 纵向差由约 854px 降为 0px。
- `app/src/components/graph/pert-view.tsx`：谱系彩色加粗曲线、按 normal/active/dimmed 隔离透明度的箭头、无焦点框宽命中层与无限画布。
- `app/src/lib/pert-layout.ts`、`app/src/components/graph/pert-view.tsx`：搜索结果上游/下游路径追踪、旧选边清理，以及按全部命中节点联合边界自动缩放居中。
- `app/src/components/graph/pert-view.tsx`：搜索命中节点同心圆标记，以及背景点击只清除路径高亮、保留查询和结果标记的独立状态。
- `app/src/lib/pert-layout.ts`：重构为路径占用感知路由，按短边到长边布线，以空间哈希评分多曲率弧线/内部走廊/上下外侧通道，并缩短目标控制柄避免入边提前合并。
- `app/src/lib/pert-layout.ts`：节点净距扩大到 420×260px；单段宽缓贝塞尔成为主候选，多段通道降为避障兜底；以实际曲线采样计算画布边界。
- `app/src/components/graph/pert-view.tsx`：窗口自适应大画布、无平移边界、0.04x–3x 缩放、首次 0.58x 可读局部视图，以及随视口移动的点阵背景。
- `app/src/lib/pert-graphviz-layout.ts`：按需加载 Graphviz WASM，以 dot 完成交叉最小化、节点障碍感知 B 样条和稀疏分量布局；同步布局保留为失败兜底。
- `app/src/lib/pert-graphviz-layout.ts`：在 Graphviz 安全路径之上增加自然三次样条二次拟合、逐级简化与碰撞复核；共享源水平主干和目标水平切线，消除多段路径折角。
- `app/src/lib/pert-layout.ts`：层间净距扩大至 720px、行距扩大至 460px，并增加确定性横向散布，为长曲线预留转弯空间。
- `app/src/lib/pert-graphviz-layout.test.ts`、`app/src/lib/pert-layout.test.ts`、`app/src/components/graph/pert-view.test.tsx`：37 个定向测试。
- `app/package.json`、`pnpm-lock.yaml`：增加 `@viz-js/viz`，渲染构建为独立的异步布局 chunk。
- `docs/FEATURES/pert-graph.md`：v12 实现方案、自然样条平滑、非平面图边界与跨端边界。
- `docs/record/TF-055-PERT任务图重构-人工验证手册.md`：真实 App 人工验收步骤。

## 3. 验证结果

- PERT 定向测试：3 文件、37 测试通过。
- `pnpm typecheck`：通过。
- `pnpm lint`：通过，无本次 PERT 警告。
- `pnpm format:check`：通过。
- `pnpm test`：49 文件、325 测试全部通过。
- ARGUS 层级任务回归：141 节点、138 条父子边与 218 条依赖边均进入 PERT 布局；用户已在 macOS App 确认修复完成。
- 真实 `/api/graph`：47 节点/134 边，Graphviz 布局约 147ms；非端点节点穿越 0；独立路径交叉对由同步兜底 616 降为 424（约 31%）。
- `pnpm build`：Electron main/preload/renderer production build 通过。
- 后端/API/MCP/CLI 数据契约：未修改。

## 4. 遗留问题与后续

- 用户已确认本轮全景图优化完成，TF-028 可流转 `done`。
- 真实依赖图为非平面图；在不合并依赖且保持每条边独立可选的前提下，二维无法保证绝对零边交叉，当前实现采用节点避让硬约束和边交叉最小化。
- 全量测试仍输出工作区既有的 MSW handler、React `act` 和 Skills 嵌套 button 警告，但不影响本次门禁通过。
