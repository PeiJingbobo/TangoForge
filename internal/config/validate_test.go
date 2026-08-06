package config

import "testing"

func TestValidate(t *testing.T) {
	valid := DefaultGlobalConfig()
	valid.LLM.BaseURL = "https://api.deepseek.com"
	valid.LLM.Model = "deepseek-chat"

	cases := []struct {
		name    string
		mutate  func(*GlobalConfig)
		wantErr bool
	}{
		{"默认配置合法", func(c *GlobalConfig) { *c = valid }, false},
		{"端口越界", func(c *GlobalConfig) { c.Port = 70000 }, true},
		{"端口为 0", func(c *GlobalConfig) { c.Port = 0 }, true},
		{"BaseURL 为空", func(c *GlobalConfig) { c.LLM.BaseURL = "  " }, true},
		{"BaseURL 非法协议", func(c *GlobalConfig) { c.LLM.BaseURL = "ftp://x/y" }, true},
		{"BaseURL 无 host", func(c *GlobalConfig) { c.LLM.BaseURL = "https://" }, true},
		{"BaseURL 合法本地", func(c *GlobalConfig) { c.LLM.BaseURL = "http://127.0.0.1:11434/v1" }, false},
		{"api_kind 非法", func(c *GlobalConfig) { c.LLM.APIKind = "gemini" }, true},
		{"api_kind 空回退 openai", func(c *GlobalConfig) { c.LLM.APIKind = "" }, false},
		{"model 为空", func(c *GlobalConfig) { c.LLM.Model = "" }, true},
		{"timeout 负数", func(c *GlobalConfig) { c.LLM.TimeoutSec = -1 }, true},
		{"retries 负数", func(c *GlobalConfig) { c.LLM.Retries = -1 }, true},
		{"max_tokens 负数", func(c *GlobalConfig) { c.LLM.MaxTokens = -5 }, true},
		{"concurrency 0", func(c *GlobalConfig) { c.LLM.Concurrency = 0 }, true},
		{"concurrency 正常", func(c *GlobalConfig) { c.LLM.Concurrency = 4 }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)
			err := Validate(cfg)
			if tc.wantErr && err == nil {
				t.Fatalf("Validate(%+v) = nil, want error", cfg)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%+v) = %v, want nil", cfg, err)
			}
		})
	}
}
