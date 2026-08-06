# TF-005 任务基础 CRUD — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
交付任务核心域基础 CRUD（`internal/task`）：Create / Get / List（树形、过滤、搜索、分页）/ Update（详情字段）+ 独立 ChangeStatus 接口；定义三端共享 Interface 与 SQLite Repo；项目识别以 `.taskboard/meta.db` 元数据为准（QA Q2-B，不依赖全局注册表）；并按要求建立语义文档 `docs/TASK-SEMANTICS.md` 且在 AGENTS.md 引用（QA Q7 增强）。

## 2. 交付内容
- **新增文件**：
  - `internal/task/service.go` — Service 接口（Create/Get/List/Update/ChangeStatus/Close）+ 实现（workdir 连接缓存、状态机存在性校验、树形组装、祖先保留过滤、分页、排序、parent 环校验、WriteHook 预留）
  - `internal/task/repo.go` — TaskRepo 接口 + SQLite 实现（JSON 列先扫 string 再解析，兼容 modernc 驱动）
  - `internal/task/errors.go` — 哨兵错误 + 业务码（PROJECT_NOT_FOUND / TASK_NOT_FOUND / TASK_INVALID / PARENT_NOT_FOUND / PARENT_CYCLE / STATUS_NOT_FOUND）
  - `internal/task/priority.go` — priority 别名归一化（lowest/none=0、low=1、normal/default=3、high=4、highest/critical/urgent=5）
  - `internal/task/service_test.go` — 27 个测试用例
  - `app/src/types/task.ts` — 前端 DTO（契约同步）
  - `docs/TASK-SEMANTICS.md` — 任务域语义统一文档（v1.0）
- **修改文件**：`AGENTS.md` / `docs/AGENTS.md`（v1.4，引用 TASK-SEMANTICS.md）、`go.mod`（uuid 提升为直接依赖）、`go.sum`（修正 modernc.org/gc/v3 校验和）、`docs/task/TASKS.md`、`docs/task/OVERVIEW.md`
- **关键实现点**：
  1. 项目识别不依赖全局注册表：`projectDB()` 校验 `{workdir}/.taskboard/meta.db` 存在即视为项目，连接按 workdir 缓存；`tasks.project_id` 固定写 1（与 permissions 一致）
  2. Q8 职责边界：Update 禁止 status（UpdateInput 无该字段），状态更新走独立 ChangeStatus（TF-005 校验状态存在性，transitions 校验 TF-006 追加）
  3. List 语义：非分页返回全量树（每层 priority DESC, created_at ASC, id ASC）；分页时扁平分页（page 从 1、size 默认 100 上限 500）；q/filter 走"祖先保留、后代过滤"；默认排除 archived
  4. UpdateInput 全指针部分更新语义 + parent_id 三重态（**string：nil 不改 / &nil 置顶 / &str 改父），parent 链环校验（PARENT_CYCLE）
  5. WriteHook 写钩子（task.created/updated/status_changed）预留 TF-012 审计与 TF-014 WS 接入点

## 3. 验证结果
- `go test -count=1 -v ./internal/task/` → **27/27 PASS**（覆盖 Create/Get/List 树形/过滤/搜索/分页/排序/项目隔离/Update 部分更新/parent 环/ChangeStatus/写钩子/错误路径）
- `internal/task` 覆盖率 **91.3%**（超过 TF-009 的 90% 门槛）
- 全仓：`CGO_ENABLED=0 go build ./...` 通过；`go vet ./...` 通过；`gofmt -l` 无输出；`go test ./...` 全绿（api/config/db/project/task）
- `go mod tidy -diff` 零差异；`go mod verify` 通过

## 4. 遗留问题与后续
- **git 提交未完成**：`.git/logs/HEAD` 与 `.git/objects` 被系统进程锁定（Permission denied / Bad file descriptor），`git commit` 无法执行；代码与文档已全部就绪于工作区，需在 IDE git 集成释放后手动提交（建议命令见 `docs/log/TF-005-任务基础CRUD.md`）。
- `depends_on` 仅存储不校验（TF-008 落地 DEPENDENCY_NOT_FOUND / CIRCULAR_DEPENDENCY）。
- ChangeStatus 的 transitions 流转校验与 STATUS_IN_USE 归 TF-006。
- 前端 `app/src/types/task.ts` 语法已人工核对，typecheck 需待 TF-022 前端环境就绪后执行。
- 语义文档 `docs/TASK-SEMANTICS.md` 为长期约定起点：后续任务新增"定义语义"须在此登记（已在 AGENTS.md v1.4 固化）。
