import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'

/**
 * 守护进程状态指示（底部指示点）：5s 轮询。
 * 判断链（鲁棒）：
 *  1. 优先主进程 IPC `daemon.status`（纯探活，Electron 环境）；
 *  2. IPC 缺失/异常时（旧主进程、web 环境）回退 API 探活 `GET /ping`
 *     （走统一代理路径，与项目列表同一链路——项目列表能读即 daemon 存活）。
 */
export function useDaemonStatus(): boolean {
  const { data } = useQuery({
    queryKey: ['daemon-status'],
    queryFn: async () => {
      const status = window.tangoforge?.daemon.status
      if (status) {
        try {
          return await status()
        } catch {
          // IPC 不可用 → 走探活兜底
        }
      }
      try {
        await apiRequest('/ping')
        return true
      } catch {
        return false
      }
    },
    refetchInterval: 5000,
  })
  return data ?? false
}
