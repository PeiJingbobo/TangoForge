# TangoForge — docs 文档目录说明（README）

> **版本**：v1.0（2026-08-05）
> **定位**：本文档是 `docs/` 目录的**使用手册与文件索引**。新增/修改文档后必须同步更新本文件的「文件索引」。
> **权威约定**：文档优先级 = `docs/REQUIREMENTS.md`（需求基线）＞ `AGENTS.md`（开发约束，根目录权威版本）＞ `docs/TECHNICAL.md`（技术落地）；冲突时以需求文档为准。

---

## 1. 目录结构总览

```
docs/
├── README.md               # 本文件：使用说明 + 文件索引
├── AGENTS.md               # AGENTS.md 的同步副本（权威版本在仓库根目录）
├── REQUIREMENTS.md         # 需求基线 v1.1（最高优先级）
├── REQUIREMENTS-REVIEW.md  # 需求评审 35 项 QA 决策记录
├── TECHNICAL.md            # 技术落地说明 v1.0
├── task/                   # 📋 开发任务计划（三件套）
│   ├── MASTER-PLAN.md      #   主行动计划：阶段/里程碑/执行规则/完成定义
│   ├── TASKS.md            #   可执行任务清单：TF-001~TF-031 逐条细节
│   └── OVERVIEW.md         #   任务全景预览：总览表/泳道/依赖图/进度
├── record/                 # ✅ 任务总结归档（正式任务完成后必写）
│   └── README.md           #   命名规范与模板
└── log/                    # 📝 任务日志（工作过程记录，临时任务豁免）
    └── README.md           #   命名规范与模板
```

---

## 2. 各文件 / 目录使用方式

| 文件 / 目录 | 何时使用 | 如何维护 | 维护人 |
|-------------|----------|----------|--------|
| `README.md`（本文件） | 任何人想了解 docs 里有什么、该读哪个 | 新增文档时同步登记进 §4 索引 | 文档作者 |
| `AGENTS.md` | AI 助手 / 开发者开发前必读；开发时持续遵守 | **权威版本在仓库根目录**，本副本同步；改根目录后用脚本/工具复制保持一致 | 约束变更提出者 |
| `REQUIREMENTS.md` | 需求有疑问、范围变更评审时 | 走评审流程（参考 REVIEW 记录），改后通知 AGENTS/TECHNICAL 同步 | 需求负责人 |
| `REQUIREMENTS-REVIEW.md` | 追溯 35 项需求决策的来由 | 只追加新一期评审记录，不篡改历史 | 评审记录者 |
| `TECHNICAL.md` | 实现技术细节（Go 规范 / 前端规范 / 数据模型 / API） | 接口、模型、约束变化时同步更新 | 对应模块开发者 |
| `task/` | 领取/更新开发任务 | 见 `docs/task/MASTER-PLAN.md` §5、§6 | 任务执行者 |
| `record/` | 任务完成时归档总结 | 命名 `TF-XXX-标题-结果.md`，模板见目录内 README | 任务执行者 |
| `log/` | 任务进行中记录过程 | 命名 `TF-XXX-标题.md`，按日期追加；临时任务豁免（`AGENTS.md §13.4`） | 任务执行者 |

### 阅读路线建议

- **新人 / AI 助手第一次入场**：`AGENTS.md` → `docs/README.md` → `docs/task/MASTER-PLAN.md` → 领取 `docs/task/TASKS.md` 中第一个 P0 任务。
- **写代码前**：查 `docs/TECHNICAL.md`（技术细节）与 `AGENTS.md`（铁律），需求冲突以 `REQUIREMENTS.md` 为准。
- **开发过程中**：维护 `docs/task/OVERVIEW.md` 进度、`docs/log/` 日志、完成后写 `docs/record/` 总结。

---

## 3. 任务相关目录联动规则

```
docs/task/TASKS.md  （状态唯一事实源：待开始 → 进行中 → 已完成）
        │ 开始任务：状态置"进行中"，同步 OVERVIEW.md
        ▼
docs/log/TF-XXX-*.md （进行中记录工作过程；临时任务豁免）
        │ 完成任务：状态置"已完成"
        ▼
docs/record/TF-XXX-*-结果.md （必写总结：范围 / 交付 / 验证 / 遗留）
        ▼
docs/task/OVERVIEW.md （同步统计与泳道；里程碑达成后对照质量门禁验收）
```

---

## 4. 文件索引

### 4.1 需求与约束类（权威基线）

| 文件 | 版本 | 说明 | 变更触发 |
|------|------|------|----------|
| `REQUIREMENTS.md` | v1.1 | 需求基线（产品/功能/非功能/技术方案/API） | 需求评审通过后 |
| `REQUIREMENTS-REVIEW.md` | – | 35 项 QA 决策记录（Q1~Q35） | 新一期评审追加 |
| `TECHNICAL.md` | v1.0 | 技术落地规范（Go + 前端 shadcn-ui） | 技术决策变化 |
| `AGENTS.md`（docs 副本） | v1.3 | 开发约束（根目录为权威，本副本同步） | 根目录变更后同步 |

### 4.2 任务与过程类（本项目工作流）

| 文件 / 目录 | 说明 | 更新频率 |
|-------------|------|----------|
| `task/MASTER-PLAN.md` | 主行动计划：阶段 P1~P6、里程碑 M1~M6、执行策略、记录规范 | 阶段/里程碑调整时 |
| `task/TASKS.md` | 可执行任务清单：TF-001 ~ TF-031（状态唯一事实源） | 每任务完成时 |
| `task/OVERVIEW.md` | 任务全景预览：总览表 / 泳道 / 依赖图 / 里程碑卡片 | 每任务完成时 |
| `record/` | 任务总结归档（`TF-XXX-标题-结果.md`） | 每正式任务完成时 |
| `log/` | 任务日志（`TF-XXX-标题.md`） | 任务进行中 |

### 4.3 文件索引（按字母序）

- `AGENTS.md` — 开发约束与工作流（同步副本）
- `README.md` — 本文件
- `REQUIREMENTS-REVIEW.md` — 需求评审 QA 记录
- `REQUIREMENTS.md` — 需求基线
- `TECHNICAL.md` — 技术落地说明
- `log/README.md` — 日志目录规范
- `record/README.md` — 总结目录规范
- `task/MASTER-PLAN.md` — 主行动计划
- `task/OVERVIEW.md` — 任务全景预览
- `task/TASKS.md` — 可执行任务清单

---

## 5. 维护规则（新增 / 变更文档时）

1. **新增文档**：放入正确子目录（任务计划 → `task/`；总结 → `record/`；日志 → `log/`），并在本文件 §4 索引登记。
2. **改需求基线**：先评审（记录进 `REQUIREMENTS-REVIEW.md`），再更新 `REQUIREMENTS.md`，最后同步 `AGENTS.md` / `TECHNICAL.md` 受影响章节。
3. **改根目录 AGENTS.md**：必须同步 `docs/AGENTS.md` 副本（`cp AGENTS.md docs/AGENTS.md` 或等价操作）。
4. **任务状态**：以 `task/TASKS.md` 为唯一事实源，`task/OVERVIEW.md` 镜像同步，禁止两处不一致。

*（文档完）*
