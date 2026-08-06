# -*- coding: utf-8 -*-
import io

p = 'app/src/features/audit/AuditPage.tsx'
s = io.open(p, encoding='utf-8').read()
s = s.replace('!data?.entries || data.entries.length === 0', '!data?.items || data.items.length === 0')
s = s.replace('data.entries.map((e: AuditEntry, i: number) => (', 'data.items.map((e: AuditEntry, i: number) => (')
s = s.replace('<tr key={e.id ?? i} className="hover:bg-accent/40">', '<tr key={`${e.ts}-${i}`} className="hover:bg-accent/40">')
s = s.replace('{e.actor_name}', '{e.actor}')
io.open(p, 'w', encoding='utf-8', newline='\n').write(s)

p2 = 'app/src/hooks/useConfig.ts'
s2 = io.open(p2, encoding='utf-8').read()
s2 = s2.replace('export function useConfig() {\n  const qc = useQueryClient()\n  return useQuery({', 'export function useConfig() {\n  return useQuery({')
io.open(p2, 'w', encoding='utf-8', newline='\n').write(s2)

p3 = 'app/src/features/tasks/ImportExportPage.tsx'
s3 = io.open(p3, encoding='utf-8').read()
s3 = s3.replace("import { toast } from 'sonner'\n", '')
io.open(p3, 'w', encoding='utf-8', newline='\n').write(s3)

p4 = 'app/src/features/settings/SettingsPage.tsx'
s4 = io.open(p4, encoding='utf-8').read()
s4 = s4.replace("import { Button } from '@/components/ui/button'\n", '')
s4 = s4.replace('api_key: next.api_key || undefined } },', '...(next.api_key ? { api_key: next.api_key } : {}) } },')
io.open(p4, 'w', encoding='utf-8', newline='\n').write(s4)
print('fixed 4 files')
