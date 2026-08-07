/**
 * 导出记录（TF-039）：localStorage 按项目隔离，最多保留 50 条。
 * 每次导出成功（ExportDialog onSuccess）追加；导入导出页记录列表展示。
 */

const KEY = 'tangoforge.export-records'
const MAX = 50

export interface ExportRecord {
  id: string
  /** 导出完成时间（ISO 8601） */
  exportedAt: string
  /** 输出文件绝对路径 */
  path: string
  /** 模板模式（default | llm） */
  mode: 'default' | 'llm'
  /** 导出任务数 */
  taskCount?: number
}

function loadAll(): Record<string, ExportRecord[]> {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? (JSON.parse(raw) as Record<string, ExportRecord[]>) : {}
  } catch {
    return {}
  }
}

function saveAll(map: Record<string, ExportRecord[]>): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(map))
  } catch {
    // 存储不可用（隐私模式/配额）时静默失败：记录仅内存存在，不影响导出。
  }
}

/** 追加一条导出记录（新纪录在前，超出 50 条裁剪）。 */
export function addExportRecord(
  workdir: string,
  rec: Omit<ExportRecord, 'id' | 'exportedAt'>,
): ExportRecord {
  const all = loadAll()
  const list = all[workdir] ?? []
  const entry: ExportRecord = {
    ...rec,
    id: crypto.randomUUID(),
    exportedAt: new Date().toISOString(),
  }
  list.unshift(entry)
  all[workdir] = list.slice(0, MAX)
  saveAll(all)
  return entry
}

/** 按项目取导出记录（新纪录在前）。 */
export function getExportRecords(workdir: string): ExportRecord[] {
  return loadAll()[workdir] ?? []
}
