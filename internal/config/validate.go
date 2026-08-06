package config

import (
	"fmt"
	"net/url"
	"strings"
)

// validAPIKinds 支持的 LLM 协议（QA P4-1：openai / anthropic / responses）。
var validAPIKinds = map[string]bool{
	"openai":    true,
	"anthropic": true,
	"responses": true,
}

// Validate 校验全局配置「可写性」（前端首选项实时保存前调用，QA：校验通过才真正保存）。
// 错误信息为可读中文描述，前端直接展示并回滚输入。
func Validate(cfg GlobalConfig) error {
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("监听端口必须在 1-65535 之间")
	}

	llm := cfg.LLM
	if strings.TrimSpace(llm.BaseURL) == "" {
		return fmt.Errorf("LLM 接口地址不能为空")
	}
	u, err := url.Parse(llm.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("LLM 接口地址必须是合法的 http/https URL")
	}

	kind := llm.APIKind
	if kind == "" {
		kind = "openai"
	}
	if !validAPIKinds[kind] {
		return fmt.Errorf("LLM 协议类型必须是 openai / anthropic / responses")
	}

	if strings.TrimSpace(llm.Model) == "" {
		return fmt.Errorf("LLM 模型名不能为空")
	}
	if llm.TimeoutSec < 0 {
		return fmt.Errorf("请求超时不能为负数")
	}
	if llm.Retries < 0 {
		return fmt.Errorf("重试次数不能为负数")
	}
	if llm.MaxTokens < 0 {
		return fmt.Errorf("max_tokens 不能为负数")
	}
	if llm.Concurrency < 1 {
		return fmt.Errorf("并发数至少为 1")
	}
	return nil
}
