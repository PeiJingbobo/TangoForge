# TF-015 LLM 客户端封装 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
实现多协议 LLM HTTP 客户端（QA P4-1 扩展：OpenAI Chat / Anthropic Messages / Responses 三协议），供 P4 后续 parser（TF-018）/ exporter（TF-019）复用；配置层新增 `llm.api_kind`。

## 2. 交付内容
- **新增文件**：
  - `internal/llm/client.go` — 三协议客户端：`New`（base/model 校验 + APIKey 空回退环境变量 `DEEPSEEK_API_KEY`）、`Complete`（重试：网络/超时/5xx/429 重试、4xx 不重试、线性退避）、`CompleteJSON`（openai `response_format=json_object` + prompt Schema；anthropic/responses 后处理提取首个平衡 JSON 块）、`FromConfig`（config.LLMConfig → 运行配置）；错误集 `ErrNotConfigured / ErrTimeout / ErrAPIStatus(携带状态码) / ErrInvalidResponse`
  - `internal/llm/client_test.go` — 14 用例：三协议请求构造（路径/头/body 字段）与响应解析、JSON 提取（围栏/数组/非法）、500→OK 与 429 重试、400 不重试、超时、未配置、env 回退、并发峰值 ≤ Concurrency
- **修改文件**：
  - `internal/config/config.go` — `LLMConfig` 新增 `APIKind`（`api_kind`）
  - `internal/config/global.go` — `DefaultLLMConfig`/`WithDefaults` 补 `api_kind` 默认（openai）
  - `docs/TASK-SEMANTICS.md` — 新增 §14（LLM 客户端语义：协议路径、结构化输出、重试与错误集、边界）
  - `docs/task/TASKS.md` / `OVERVIEW.md` — TF-015 状态 ✅、统计同步
- **关键实现点**：
  1. 三协议差异集中在 `buildRequest`（请求体/头/路径）与 `parseResponse`（内容提取），上层 `Complete/CompleteJSON` 协议无关
  2. JSON 提取器支持对象/数组、字符串内 `{}`/`[]` 与转义、围栏与前后缀文本容错
  3. 重试语义：仅网络错误/超时/5xx/429 重试（共 `Retries+1` 次），4xx 立即失败（配置/鉴权错误重试无意义）
  4. APIKey 空 → 环境变量回退（`New` 统一处理），实测目标 DeepSeek（https://api.deepseek.com，deepseek-v4-flash）

## 3. 验证结果
- `gofmt -l internal/llm internal/config` → 干净（gofmt -w 已修复）
- `CGO_ENABLED=0 go test ./internal/llm/... ./internal/config/...` → **ok**（14 用例全过）
- `CGO_ENABLED=0 go vet ./internal/llm/... ./internal/config/...` → 干净
- `CGO_ENABLED=0 go test ./...` → **全仓全绿**（无回归）

## 4. 遗留问题与后续
- 真实 DeepSeek 冒烟未执行：两侧环境变量 `DEEPSEEK_API_KEY` 均未生效（用户侧未配置）；M4 人工验证时由用户 export 或写入 `~/.taskboard-app/config.yaml` 的 `llm.api_key` 后跑通导入草稿流。
- LLM 错误 → HTTP 映射（`LLM_NOT_CONFIGURED` 等 → 422）在 TF-018/019 落地时统一接线。
- `docs/AGENTS.md` 为仓库根 `AGENTS.md` 同步副本，未随本次变更（llm 模块描述无需修订）。
