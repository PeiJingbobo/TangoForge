---
name: taskboard-basic
description: 使用 TangoForge 管理项目任务（创建/查询/更新/流转/归档）
version: "1.0.0"
hosts: [.claude/skills, .cursor/skills, .github/skills, user-claude, user-codebuddy]
when_to_use: 需要创建、查询、更新、流转、归档或还原项目任务时激活本技能
---

# TaskBoard Basic — 任务管理操作指南

TangoForge 是本地优先的人机协作任务中间件。守护进程常驻本机（默认端口 19810），
所有操作必须**显式携带项目工作目录**（HTTP `X-Project` 头 / MCP `project` 参数 / CLI `--project`）。

## 核心概念

- **项目** = 一个工作目录（含 `.taskboard/` 元数据）。操作前先用 `project_list` 找到目标项目。
- **任务字段**：`id`(UUID)、`parent_id`(子任务)、`title`、`description`、`status`、`priority`(0-5)、
  `tags`、`assignee`、`depends_on`(依赖，必须无环)。
- **状态机**：默认 `todo → doing → done → archived`；每项目可自定义（`state_machine_get` 查看）。
- **删除 = 归档**：`task_archive` 将任务置为 archived（记录归档前状态），`task_restore` 还原。
- **父任务归档/删除时**：所有子任务自动变为顶层任务（parent_id 置空）。

## 操作流程

### 1. 定位项目

```
project_list                                  # 列出全部已注册项目
```

### 2. 查询任务

```
task_list  project=/path/to/project           # 全部任务（树形）
task_read  project=/path/to/project id=<task-id>
```

### 3. 创建 / 更新任务

```
task_create project=/path/to/project title="编写需求文档" \
            description="..." status=todo priority=2 tags=["docs"]
task_update project=/path/to/project id=<task-id> status=doing
```

### 4. 归档 / 还原

```
task_archive project=/path/to/project id=<task-id>
task_restore project=/path/to/project id=<task-id>
```

## 字段语义速查

- `status` 必须是项目状态机中的合法 key；非法流转返回 `INVALID_TRANSITION`。
- `depends_on` 是任务 ID 数组；引入循环依赖时写操作被拒绝（`CIRCULAR_DEPENDENCY`）。
- `priority` 0 最低、5 最高；列表默认按 priority 降序 + created_at 排序。

## 三端调用方式（等价）

| 端 | 入口 | 说明 |
|---|---|---|
| MCP | `tangoforge mcp`（stdio，推荐 AI 使用） | 本技能对应工具见上文 |
| HTTP | `http://127.0.0.1:19810/api/*` | 请求头 `X-Project: <workdir>` |
| CLI | `tangoforge tasks ... --project <workdir>` | 自动拉起守护进程 |

> 更完整的端点/工具清单见 `GET http://127.0.0.1:19810/api/guide`（免鉴权说明书端点）。
