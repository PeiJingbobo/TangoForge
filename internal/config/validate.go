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

	// TF-046：embedding 配置校验（model 空 = 未配置，仅校验已配置部分）。
	emb := llm.Embedding
	if strings.TrimSpace(emb.Model) != "" {
		kind := emb.APIKind
		if kind == "" {
			kind = "openai"
		}
		if kind != "openai" && kind != "ollama" {
			return fmt.Errorf("embedding 协议类型必须是 openai / ollama")
		}
		// base_url 为空时复用 chat base_url（EmbeddingFromConfig 处理）；此处校验显式配置的 URL。
		if strings.TrimSpace(emb.BaseURL) != "" {
			eu, err := url.Parse(emb.BaseURL)
			if err != nil || (eu.Scheme != "http" && eu.Scheme != "https") || eu.Host == "" {
				return fmt.Errorf("embedding 接口地址必须是合法的 http/https URL")
			}
		}
		if emb.TimeoutSec < 0 {
			return fmt.Errorf("embedding 请求超时不能为负数")
		}
		if emb.MaxTokens < 0 {
			return fmt.Errorf("embedding max_tokens 不能为负数")
		}
	}

	// TF-052：知识库全局配置数值校验（0 = 未设置，由 WithDefaults 补默认；仅拒绝负数）。
	k := cfg.Knowledge
	if k.DebounceMS < 0 {
		return fmt.Errorf("知识库防抖窗口不能为负数")
	}
	if k.EmbedConcurrency < 0 {
		return fmt.Errorf("知识库嵌入并发不能为负数")
	}
	if k.MaxIndexSize < 0 {
		return fmt.Errorf("知识库索引大小上限不能为负数")
	}
	if k.SearchTopK < 0 {
		return fmt.Errorf("知识库检索 top_k 不能为负数")
	}
	if k.SearchThreshold < 0 || k.SearchThreshold > 1 {
		return fmt.Errorf("知识库检索阈值必须在 0-1 之间")
	}
	return nil
}
