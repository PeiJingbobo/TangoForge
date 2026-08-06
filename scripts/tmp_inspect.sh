#!/bin/bash
set -e
python3 - << 'PYEOF'
import json
d = json.load(open('/tmp/tf_llm_resp_8k.json'))
if 'choices' not in d:
    print('异常响应:', json.dumps(d, ensure_ascii=False)[:400])
    raise SystemExit
c = d['choices'][0]
m = c.get('message', {})
print('finish_reason:', c.get('finish_reason'))
print('usage:', d.get('usage'))
print('reasoning 长度:', len(m.get('reasoning_content') or ''))
print('content 长度:', len(m.get('content') or ''))
PYEOF
