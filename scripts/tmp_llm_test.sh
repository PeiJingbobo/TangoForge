#!/bin/bash
set -e
DOC="/Users/peijingbo/HD-DATA/Coding/TangoForge/test-markdown/task/01-executable-backlog.md"
python3 - << 'PYEOF'
import json

doc = open("/Users/peijingbo/HD-DATA/Coding/TangoForge/test-markdown/task/01-executable-backlog.md", encoding="utf-8").read()
sm_text = ("项目状态机状态列表（仅可使用这些状态）：\n- todo（label: 待办）\n- doing（label: 进行中）\n- done（label: 已完成）\n\n"
           "请解析以下 Markdown 任务文档：\n\n" + doc)
system = ("你是 TangoForge 的任务解析器。用户会提供一份 Markdown 任务文档，你需要将其中的任务语义化解析为结构化 JSON。"
          "规则：1. 只输出 JSON 本身；2. 每个任务必须有 title 与 status；3. 层级用嵌套 children；"
          "4. status 只能输出给定状态 key 或 label；5. priority 0-5 整数或别名；6. depends_on 输出被依赖任务标题；7. 保持顺序。")
req = {
    "model": "deepseek-v4-flash",
    "messages": [{"role": "system", "content": system}, {"role": "user", "content": sm_text}],
    "max_tokens": 4096,
    "response_format": {"type": "json_object"},
    "stream": False,
}
with open("/tmp/tf_llm_req.json", "w", encoding="utf-8") as f:
    json.dump(req, f, ensure_ascii=False)
print("请求已写，user 内容长度:", len(sm_text))
PYEOF

KEY=$(grep "^llm:" -A6 ~/.taskboard-app/config.yaml | grep api_key | awk '{print $2}' | tr -d '"')
echo "=== 测试 2：完整文档 ==="
time curl -s -m 180 https://api.deepseek.com/chat/completions \
  -H "Content-Type: application/json" -H "Authorization: Bearer $KEY" \
  -d @/tmp/tf_llm_req.json > /tmp/tf_llm_resp.json 2>&1
python3 - << 'PYEOF'
import json
d = json.load(open('/tmp/tf_llm_resp.json'))
print('顶层 keys:', list(d.keys()))
if d.get('choices'):
    m = d['choices'][0].get('message', {})
    print('content 长度:', len(m.get('content') or ''))
    print('finish_reason:', d['choices'][0].get('finish_reason'))
    print('content 前 200:', (m.get('content') or '')[:200])
else:
    print('完整响应:', json.dumps(d, ensure_ascii=False)[:800])
PYEOF
