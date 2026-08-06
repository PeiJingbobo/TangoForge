# TF-013 HTTP API 核心端点 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-013-http-api`

## 进展记录

### 2026-08-06（完成）
1. Server 自组装依赖 + 审计接线；projects 组豁免 X-Project（QA P3-2）。
2. handlers：projects/tasks/state-machine/permissions；mapError 错误映射（QA P3-7）。
3. IdentifyMiddleware 改配置提供者模式（热重载 ui_token/api_token 即时生效）。
4. 集成冒烟 11 用例；全链路 3 次稳定通过；覆盖率 92.3%。

## 决策记录
- **审计接线**（QA P3-1）：写钩子 OnWrite→audit.Write（ok）；OnDenied→audit.Write（denied）；actor 经 ctx。
- **PATCH 动态权限**：路由挂 task.update，body 含 status 时 handler 内 ensureAction 二次校验 task.update_status（动态 action 无法静态挂中间件）。
- **PUT /api/permissions**：不挂权限中间件，handler 内 actor==ui 校验（识别层已保证回环+Token）。
- **projects 组**：GET/POST 放行 agent（project.read 默认授予、无项目上下文不逐项查表，注释说明）；DELETE 仅 UI。
- **错误映射**：422 用于业务规则类冲突（INVALID_TRANSITION 等），400 用于参数非法，404 用于不存在，401/403 用于安全。

## 踩坑记录（重要）
1. **WAL 写升级 SQLITE_BUSY（根因定位）**：archive 的 DEFERRED 事务先 GetByID（读）后 Update（写）→ WAL 下读快照已建，遇 audit 并发写锁时**写升级立即 BUSY 且 busy handler 不适用**。实验验证：单条 Exec 并发写会等待 5.1s（busy_timeout 生效）；事务内先读后写则立即 BUSY。
2. **修复过程**：先尝试 `BeginTx(Isolation: sql.LevelSerializable)`（期望 BEGIN IMMEDIATE）——实验证明 modernc 会持写锁，但并发场景下与 database/sql 连接池互斥存在死锁风险（fatal deadlock），且测试仍偶发 BUSY。**最终方案：事务外读快照（幂等短路保留）+ 事务内首条语句即写**（DEFERRED 首个写语句获取写锁时应用 busy_timeout）——稳定通过。
3. **chi 路由类型**：`RequirePermission` 返回 http.Handler 接口，chi 的 r.Get 需要 http.HandlerFunc → 加 `perm(action, h)` 包装。
4. **测试基建**：TempDir 清理需在 Server.Close 之后（defer 而非 t.Cleanup，LIFO 顺序坑）；远程测试需 RemoteAccess=true（否则 403 优先于 401）；旧断言（路由未定义 404）随 TF-013 更新。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(api): TF-013 HTTP API 核心端点（权限/审计接线 + 全链路冒烟）"
```
