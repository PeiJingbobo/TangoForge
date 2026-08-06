import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// 服务端状态统一走 TanStack Query（docs/TECHNICAL.md §4.3）：
// 组件不直接持有服务端数据，一律经 Query hook 读取、Mutation 写入。
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
})

export default function App() {
  // 占位骨架：路由（React Router v7）与业务页面按 docs/TECHNICAL.md §4.1 逐步实现。
  return (
    <QueryClientProvider client={queryClient}>
      <div className="flex min-h-screen items-center justify-center">
        <h1 className="text-lg font-semibold">TangoForge</h1>
      </div>
    </QueryClientProvider>
  )
}
