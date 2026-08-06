import re

# 命令文件调用点批量替换（用 lambda 避免 repl 转义问题）
pat = re.compile(r'project := opts\["project"\]\n(\s*)if err := requireProjectFlag\(project\); err != nil \{\n\1\s*return err\n\1\}')
def make_repl(m):
    ind = m.group(1)
    return ('project := opts["project"]\n' + ind + 'var err error\n'
            + ind + 'if project, err = requireProject(project); err != nil {\n'
            + ind + '\treturn err\n' + ind + '}')
for p in ['cmd/cli/cmd_tasks.go', 'cmd/cli/cmd_import.go', 'cmd/cli/cmd_other.go']:
    s = open(p, encoding='utf-8').read()
    n = len(pat.findall(s))
    s2 = pat.sub(make_repl, s)
    open(p, 'w', encoding='utf-8').write(s2)
    print('%s: 替换 %d 处' % (p, n))
