// Package audit 负责审计日志的异步写入与导出。
//
// 约束（docs/TECHNICAL.md §3.6「审计日志（不可篡改）」）：
//   - 所有写操作（Create / Update / Archive / Restore / StatusChange / Import /
//     Export / 权限与状态机修改）必须异步写入 audit_log 表（数据本体）；
//     audit.log 文件仅为按需导出物；
//   - 审计字段：id, ts, actor, actor_class, action, target, result, detail；
//     仅写操作记录，读取不记录。
package audit
