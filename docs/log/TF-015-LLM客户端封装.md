# TF-015 LLM 客户端封装 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-015-llm-client`

## 进展记录

### 2026-08-06（完成）
1. **范围扩展（QA P4-1）**：任务清单原为「OpenAI 兼容 HTTP 客户端」，用户确认扩展为**三协议**（openai / anthropic / responses），DeepSeek（base https://api.deepseek.com，model deepseek-v4-flash）为实测目标，API key 走环境变量 `DEEPSEEK_API_KEY`。
2. `internal/config`：`LLMConfig` 新增 `api_kind` 字段（默认 openai），`WithDefaults`/`DefaultLLMConfig` 同步。
3. `internal/llm/client.go`：`New`（base/model 非空校验 + APIKey 空回退环境变量）、`Complete`（重试：网络/超时/5xx/429 重试，4xx 不重试，线性退避 100ms×i）、`CompleteJSON`（openai response_format + prompt Schema；anthropic/responses 后处理提取首个平衡 JSON 块）、三协议 buildRequest/parseResponse 分派。
4. `internal/llm/client_test.go`：14 用例（三协议构造/解析、JSON 围栏/数组/非法、500→OK 重试、429 重试、400 不重试、超时、未配置、env 回退、并发峰值 ≤ Concurrency、FromConfig 默认）。

## 决策记录
- **APIKey 回退位置**：统一放 `New`（`FromConfig` 纯转换），避免两处逻辑漂移；测试 `TestEnvKeyFallback` 验证。
- **协议路径拼接**：openai `{base}/chat/completions`（DeepSeek 无 /v1 前缀亦可）、anthropic `{base}/v1/messages`、responses `{base}/v1/responses`；base_url 尾部 `/` 自动去除，用户按厂商端点填 base（DeepSeek anthropic 兼容 = https://api.deepseek.com/anthropic）。
- **重试策略**：4xx（除 429）不重试——配置/鉴权错误重试无意义；登记 TASK-SEMANTICS §14.3。
- **LLM 未配置**：`ErrNotConfigured` → 规划 HTTP 422 `LLM_NOT_CONFIGURED`（TF-018/019 统一映射）；导出 default 模式不依赖 LLM。

## 踩坑记录
1. `gofmt -l` 首次列出 client.go/client_test.go（表字段对齐），`gofmt -w` 修复。
2. `TestEnvKeyFallback` 首跑失败（Authorization "Bearer" 空）：env 回退原本只放 `FromConfig`，`New(Config{})` 直连不回退 → 统一移至 `New`。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(llm): TF-015 LLM 多协议客户端（openai/anthropic/responses + 结构化输出）"
```
