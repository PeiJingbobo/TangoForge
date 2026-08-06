#!/bin/bash
set -e
python3 - << 'PYEOF'
import json
for name in ['rlow', 'chat']:
    p = '/tmp/tf_resp_%s.json' % name
    try:
        d = json.load(open(p))
    except Exception as e:
        print(name, '读取失败:', e)
        continue
    if 'choices' not in d:
        print(name, '异常:', json.dumps(d, ensure_ascii=False)[:300])
        continue
    c = d['choices'][0]
    m = c.get('message', {})
    content = m.get('content') or ''
    print('%s: finish=%s content=%d reasoning=%d usage=%s' % (
        name, c.get('finish_reason'), len(content),
        len(m.get('reasoning_content') or ''), d.get('usage')))
    # JSON 有效性
    try:
        obj = json.loads(content)
        print('  JSON 有效，tasks 顶层数:', len(obj.get('tasks', [])))
    except Exception as e:
        print('  JSON 无效:', e)
PYEOF
