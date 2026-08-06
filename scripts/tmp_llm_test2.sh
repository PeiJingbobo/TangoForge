#!/bin/bash
set -e
KEY=$(grep "^llm:" -A6 ~/.taskboard-app/config.yaml | grep api_key | awk '{print $2}' | tr -d '"')

echo "=== 测试 3: reasoning_effort=low（降低推理量） ==="
python3 - << 'PYEOF'
import json
d = json.load(open('/tmp/tf_llm_req.json', encoding='utf-8'))
d['max_tokens'] = 8192
d['reasoning_effort'] = 'low'
json.dump(d, open('/tmp/tf_llm_req_rlow.json', 'w', encoding='utf-8'), ensure_ascii=False)
PYEOF
curl -s -m 180 https://api.deepseek.com/chat/completions -H "Content-Type: application/json" -H "Authorization: Bearer $KEY" -d @/tmp/tf_llm_req_rlow.json > /tmp/tf_resp_rlow.json 2>&1
python3 - << 'PYEOF'
import json
d = json.load(open('/tmp/tf_resp_rlow.json'))
if 'choices' not in d:
    print('API 拒绝 reasoning_effort:', json.dumps(d, ensure_ascii=False)[:300])
else:
    c = d['choices'][0]; m = c.get('message', {})
    print('finish:', c.get('finish_reason'), '| content 长度:', len(m.get('content') or ''), '| usage:', d.get('usage'))
    print('content 前 200:', (m.get('content') or '')[:200])
PYEOF

echo "=== 测试 4: deepseek-chat（非推理模型） ==="
python3 - << 'PYEOF'
import json
d = json.load(open('/tmp/tf_llm_req.json', encoding='utf-8'))
d['model'] = 'deepseek-chat'
d['max_tokens'] = 4096
if 'reasoning_effort' in d: del d['reasoning_effort']
json.dump(d, open('/tmp/tf_llm_req_chat.json', 'w', encoding='utf-8'), ensure_ascii=False)
PYEOF
curl -s -m 180 https://api.deepseek.com/chat/completions -H "Content-Type: application/json" -H "Authorization: Bearer $KEY" -d @/tmp/tf_llm_req_chat.json > /tmp/tf_resp_chat.json 2>&1
python3 - << 'PYEOF'
import json
d = json.load(open('/tmp/tf_resp_chat.json'))
if 'choices' not in d:
    print('API 拒绝 deepseek-chat:', json.dumps(d, ensure_ascii=False)[:300])
else:
    c = d['choices'][0]; m = c.get('message', {})
    print('finish:', c.get('finish_reason'), '| content 长度:', len(m.get('content') or ''), '| usage:', d.get('usage'))
    print('content 前 200:', (m.get('content') or '')[:200])
PYEOF
