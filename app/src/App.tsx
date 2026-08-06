import { useEffect } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router'
import { AppLayout } from '@/components/layout/app-layout'
import { Toaster } from '@/components/ui/sonner'
import { WorkspacePage } from '@/features/projects/WorkspacePage'
import { KanbanPage } from '@/features/tasks/KanbanPage'
import { TaskDetailPage } from '@/features/tasks/TaskDetail'
import { NavPage } from '@/features/tasks/NavViews'
import { GraphPage } from '@/features/tasks/GraphPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { bootstrapDaemon } from '@/lib/bootstrap'

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

/**
 * 路由骨架（React Router v7，docs/TECHNICAL.md §4.1）：
 * - /                      工作区（TF-024 项目列表）
 * - /project/:projectId/* 项目内（看板 TF-025、任务详情 TF-026）
 * - /settings              设置（TF-028）
 */
export default function App() {
  // 启动引导：拉起 daemon + 注入 UI token（Electron 环境；Web 模式静默跳过）
  useEffect(() => {
    void bootstrapDaemon()
  }, [])

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route element={<AppLayout />}>
            <Route index element={<WorkspacePage />} />
            <Route path="project/:projectId" element={<Navigate to="kanban" replace />} />
            <Route path="project/:projectId/kanban" element={<KanbanPage />} />
            <Route path="project/:projectId/nav" element={<NavPage />} />
            <Route path="project/:projectId/graph" element={<GraphPage />} />
            <Route path="project/:projectId/tasks/:taskId" element={<TaskDetailPage />} />
            <Route path="settings" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
      <Toaster />
    </QueryClientProvider>
  )
}
