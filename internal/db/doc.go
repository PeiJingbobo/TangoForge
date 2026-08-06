// Package db 负责 SQLite 初始化、迁移（migrate up/down）与连接池管理。
//
// 约束（docs/TECHNICAL.md §2.2 / §3.1）：
//   - 仅使用 modernc.org/sqlite（纯 Go 实现，CGO_ENABLED=0 可编译），禁止 CGO / 外部动态链接依赖；
//   - 仅提供原生 SQL 或 Query Builder，数据库事务边界由业务层（task/parser/exporter）控制；
//   - 核心表：projects / tasks / permissions / import_drafts / skills / audit_log
//     （DDL 以 docs/REQUIREMENTS.md §四.4 为准，meta.db 位于 {workdir}/.taskboard/meta.db）。
package db
