import { useQuery } from '@tanstack/react-query'

/**
 * 守护进程状态指示（底部导航点）：纯探活（主进程 IPC，不拉起），5s 轮询。
 * Web / 测试环境（无 window.tangoforge）→ 恒 false（不显示为运行）。
 */
export function useDaemonStatus(): boolean {
  const { data } = useQuery({
    queryKey: ['daemon-status'],
    queryFn: async () => {
      const status = window.tangoforge?.daemon.status
      if (!status) return false
      return status()
    },
    refetchInterval: 5000,
  })
  return data ?? false
}
