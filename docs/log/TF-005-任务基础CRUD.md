# TF-005 任务基础 CRUD — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-005-task-crud`

## 进展记录

### 2026-08-06（完成）
1. **QA 确认（17 项）**：Q1-A（Service 内部缓存项目库连接）、Q2-B（**业务层不依赖全局注册表，以 `.taskboard/meta.db` 元数据识别项目，project_id 固定 1**）、Q3-A（Service/Repo 双接口）、Q4 全采用（创建校验集）、Q5-A（low=1）、Q6-A（tags 去重去空保序）、Q7-A（**指针语义 + 语义文档沉淀 + AGENTS.md 引用**）、Q8（**Update 禁 status，独立 ChangeStatus**）、Q9-A（parent 环校验本轮做）、Q10~Q13-A（树形/分页/过滤/排序/详情语义）、Q14-A（WriteHook 预留）、Q15-A（google/uuid 直接依赖）、Q16-A（前端 types 同步）、Q17-A（哨兵错误 + 业务码）。
2. **新建 `docs/TASK-SEMANTICS.md`**（v1.0）：项目识别、字段语义、Create/Update/ChangeStatus/List 语义、错误码、钩子约定、任务边界；AGENTS.md 升 v1.4 并引用（docs/AGENTS.md 同步）。
3. **实现 `internal/task`**：errors.go / priority.go / repo.go / service.go（见总结 §2）。
4. **测试**：27 用例全 PASS，覆盖率 91.3%；全仓 gofmt/vet/CGO 构建/测试全绿。
5. **文档同步**：TASKS.md（P2 1/5）、OVERVIEW.md（统计 31·26·0·5）更新。

## 决策记录
- **Q2-B 语义**：任务域校验项目存在 = 检查 `{workdir}/.taskboard/meta.db`；删除全局注册表记录后目录内容仍完整保留、重新导入即复用（与 TF-004 Import 幂等闭环）。
- **Q8 接口拆分**：`Update` 只动详情字段；`ChangeStatus(ctx, workdir, id, status)` 独立；TF-006 在 ChangeStatus 内追加 transitions 校验，签名不变。
- **时间戳精度**：DB 存 RFC3339 秒精度，测试断言对同秒排序采用 id ASC 兜底，避免随机 UUID 顺序依赖。

## 踩坑记录
1. **modernc.org/sqlite 不支持 Scan 到 []string**：tags/depends_on 列需先扫 string 再 `json.Unmarshal`，否则 `Scan error on column index 7`。
2. **测试 TempDir 清理失败**：service 缓存连接未释放导致 meta.db 被占用（Windows 文件锁）→ Service 增加 `Close()`，测试 `t.Cleanup` 统一关闭。
3. **环境故障（重要）**：
   - Go 不在 PATH：使用 WorkBuddy 管理工具链 `C:\Users\PeiJingbo\.workbuddy\binaries\go\go\bin\go.exe`（go1.26.5）。
   - **`.git` 目录被系统级锁定**：`.git/logs/HEAD` append 报 Bad file descriptor、`.git/objects` 新建对象 Permission denied（沙箱外亦如此，疑似 IDE git 集成/杀软占用）；`git commit` 无法执行。已完成的基线提交与 TF-005 提交均**留在工作区未提交**。
   - bash 子进程（cp/mv/git/go mod tidy 写文件）对部分路径写入被拒；用 Write/Edit 工具（原生通道）可正常写文件。
4. **go.sum 校验和冲突**：`modernc.org/gc/v3 v3.1.4` 的 go.sum 条目与 sumdb 权威值不符（`...XeItAw...` vs `...XeITAw...`），已按 sumdb 值修正，`go mod verify` 通过。

## 建议提交命令（待 .git 锁释放后执行）
```bash
git add -A
git commit -m "feat(task): TF-005 任务基础 CRUD"

# 附：P1 基线（若未提交）
# git commit -m "chore: P1 基础设施基线提交（TF-001~TF-004，M1 数据层可用）"
```
