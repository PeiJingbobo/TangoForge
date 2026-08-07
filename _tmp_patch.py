# -*- coding: utf-8 -*-
import io

# KanbanView: onOpenTask + 新建跳转 -> 打开抽屉
p = 'app/src/features/tasks/KanbanView.tsx'
s = io.open(p, encoding='utf-8').read()
s = s.replace(
    "import { useProjectId } from '@/hooks/useProject'",
    "import { useProjectId } from '@/hooks/useProject'\nimport { useTaskDrawerStore } from '@/stores/task-drawer'",
)
s = s.replace(
    '  const { getEffectiveStatus, moveTask } = useKanbanMutations(pid)',
    '  const { getEffectiveStatus, moveTask } = useKanbanMutations(pid)\n  const openTaskDrawer = useTaskDrawerStore((st) => st.openDrawer)',
)
s = s.replace(
    'onOpenTask={(id) => navigate(`/project/${encodeURIComponent(pid ?? \'\')}/tasks/${id}`)}',
    'onOpenTask={(id) => openTaskDrawer({ taskId: id })}',
)
s = s.replace(
    "navigate(`/project/${encodeURIComponent(pid ?? '')}/tasks/${t.id}`)",
    'openTaskDrawer({ taskId: t.id })',
)
io.open(p, 'w', encoding='utf-8', newline='\n').write(s)

# NavViews
p2 = 'app/src/features/tasks/NavViews.tsx'
s2 = io.open(p2, encoding='utf-8').read()
s2 = s2.replace(
    "import { useProjectId } from '@/hooks/useProject'",
    "import { useProjectId } from '@/hooks/useProject'\nimport { useTaskDrawerStore } from '@/stores/task-drawer'",
)
s2 = s2.replace(
    "const openTask = (id: string) => navigate(`/project/${encodeURIComponent(pid ?? '')}/tasks/${id}`)",
    'const openTaskDrawer = useTaskDrawerStore((st) => st.openDrawer)\n  const openTask = (id: string) => openTaskDrawer({ taskId: id })',
)
io.open(p2, 'w', encoding='utf-8', newline='\n').write(s2)

# GraphPage
p3 = 'app/src/features/tasks/GraphPage.tsx'
s3 = io.open(p3, encoding='utf-8').read()
s3 = s3.replace(
    "import { useProjectId } from '@/hooks/useProject'",
    "import { useProjectId } from '@/hooks/useProject'\nimport { useTaskDrawerStore } from '@/stores/task-drawer'",
)
s3 = s3.replace(
    '  const { data: sm } = useStateMachine(pid)',
    '  const { data: sm } = useStateMachine(pid)\n  const openTaskDrawer = useTaskDrawerStore((st) => st.openDrawer)',
)
s3 = s3.replace(
    'onSelect={(id) => navigate(`/project/${encodeURIComponent(pid ?? \'\')}/tasks/${id}`)}',
    'onSelect={(id) => openTaskDrawer({ taskId: id })}',
)
io.open(p3, 'w', encoding='utf-8', newline='\n').write(s3)
print('entries patched')
