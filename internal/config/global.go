package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// DefaultLLMConfig 返回 LLM 默认配置（QA Q14 确认：60s / 重试 1 / 4096 / 并发 1；api_kind 默认 openai）。
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		APIKind:     "openai",
		TimeoutSec:  60,
		Retries:     1,
		MaxTokens:   4096,
		Concurrency: 1,
		Embedding:   DefaultEmbeddingConfig(),
	}
}

// DefaultEmbeddingConfig 返回 embedding 默认配置（QA-K5：model 空 = 未配置，向量功能受限）。
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		APIKind:    "openai",
		TimeoutSec: 60,
	}
}

// DefaultGlobalConfig 返回全局配置默认值：端口 19810、远程访问关闭、LLM 默认、知识库默认。
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Port:      DefaultPort,
		LLM:       DefaultLLMConfig(),
		Knowledge: DefaultKnowledgeGlobalConfig(),
	}
}

// DefaultKnowledgeGlobalConfig 返回知识库全局配置默认值（QA-K18：全开 + 30s 防抖 + 串行 + 512KB）。
func DefaultKnowledgeGlobalConfig() KnowledgeGlobalConfig {
	return KnowledgeGlobalConfig{
		Enabled:          boolPtr(true),
		FSNotify:         boolPtr(true),
		StartupScan:      boolPtr(true),
		DebounceMS:       30000,
		EmbedConcurrency: 1,
		MaxIndexSize:     524288,
		VectorSearch:     boolPtr(true),
		SearchTopK:       10,
		SearchThreshold:  0.3,
	}
}

// boolPtr 返回 bool 指针。
func boolPtr(v bool) *bool { return &v }

// WithDefaults 用默认值补齐缺失字段（零值字段视为未配置）。
//
// 知识库布尔开关语义（QA-K18 默认全开）：*bool 指针 nil = 未设置 → 补默认 true；
// 显式 false 保留关闭。数值字段零值补默认。
func (c GlobalConfig) WithDefaults() GlobalConfig {
	d := DefaultGlobalConfig()
	if c.Port <= 0 {
		c.Port = d.Port
	}
	if c.LLM.APIKind == "" {
		c.LLM.APIKind = d.LLM.APIKind
	}
	if c.LLM.TimeoutSec <= 0 {
		c.LLM.TimeoutSec = d.LLM.TimeoutSec
	}
	if c.LLM.Retries < 0 {
		c.LLM.Retries = d.LLM.Retries
	}
	if c.LLM.MaxTokens <= 0 {
		c.LLM.MaxTokens = d.LLM.MaxTokens
	}
	if c.LLM.Concurrency <= 0 {
		c.LLM.Concurrency = d.LLM.Concurrency
	}
	// embedding 默认值（模型空 = 未配置，其余给默认）。
	if c.LLM.Embedding.APIKind == "" {
		c.LLM.Embedding.APIKind = d.LLM.Embedding.APIKind
	}
	if c.LLM.Embedding.TimeoutSec <= 0 {
		c.LLM.Embedding.TimeoutSec = d.LLM.Embedding.TimeoutSec
	}
	// knowledge 全局配置默认值（QA-K18/K23）。
	k := c.Knowledge
	if k.DebounceMS <= 0 {
		k.DebounceMS = d.Knowledge.DebounceMS
	}
	if k.EmbedConcurrency <= 0 {
		k.EmbedConcurrency = d.Knowledge.EmbedConcurrency
	}
	if k.MaxIndexSize <= 0 {
		k.MaxIndexSize = d.Knowledge.MaxIndexSize
	}
	if k.SearchTopK <= 0 {
		k.SearchTopK = d.Knowledge.SearchTopK
	}
	if k.SearchThreshold <= 0 {
		k.SearchThreshold = d.Knowledge.SearchThreshold
	}
	// 布尔开关：nil → 默认 true。
	boolDefault := func(p *bool, def bool) *bool {
		if p == nil {
			v := def
			return &v
		}
		return p
	}
	k.Enabled = boolDefault(k.Enabled, true)
	k.FSNotify = boolDefault(k.FSNotify, true)
	k.StartupScan = boolDefault(k.StartupScan, true)
	k.VectorSearch = boolDefault(k.VectorSearch, true)
	c.Knowledge = k
	return c
}

// LoadGlobal 读取全局配置。
//
// 缺失文件容错：文件不存在时返回默认配置（nil error），由守护进程首次启动时 SaveGlobal 落盘；
// 文件存在但解析失败则返回错误（不静默降级，避免误读损坏配置）。
func LoadGlobal(path string) (GlobalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultGlobalConfig(), nil
		}
		return GlobalConfig{}, fmt.Errorf("config: read global %s: %w", path, err)
	}
	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return GlobalConfig{}, fmt.Errorf("config: parse global %s: %w", path, err)
	}
	return cfg.WithDefaults(), nil
}

// SaveGlobal 将全局配置序列化写入磁盘（自动创建父目录，权限 0600 保护凭据）。
func SaveGlobal(path string, cfg GlobalConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: marshal global: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("config: write global %s: %w", path, err)
	}
	return nil
}

// GenerateToken 生成 32 位十六进制随机凭据（用于 ui_token / api_token 初始化）。
func GenerateToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("config: generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// WatchGlobal 监听全局配置文件变化，变更后重新加载并回调最新配置。
//
// 特性（QA Q8 热重载）：
//   - 监听配置文件所在目录（兼容编辑器「临时文件 + rename」的原子保存方式）；
//   - 事件去抖（150ms 合并连续写入），避免一次保存触发多次回调；
//   - 重新加载失败时保留旧配置并记日志（不中断 watcher）；
//   - 返回 stop 函数，守护进程退出时调用以释放资源。
//
// 本函数不阻塞；调用方负责在回调中实现「端口重绑 / remote_access 切换」等副作用。
func WatchGlobal(path string, onUpdate func(GlobalConfig)) (stop func(), err error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("config: mkdir %s: %w", dir, err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("config: new watcher: %w", err)
	}
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("config: watch %s: %w", dir, err)
	}

	done := make(chan struct{})
	var timer *time.Timer

	go func() {
		defer func() { _ = watcher.Close() }()
		for {
			select {
			case <-done:
				return
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				// 只关心目标配置文件（含编辑器原子保存产生的 rename 事件）。
				if filepath.Base(ev.Name) != base {
					continue
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(150*time.Millisecond, func() {
					cfg, loadErr := LoadGlobal(path)
					if loadErr != nil {
						slog.Error("config: hot reload failed, keep old config", "path", path, "err", loadErr)
						return
					}
					slog.Info("config: hot reloaded", "path", path, "port", cfg.Port)
					onUpdate(cfg)
				})
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return func() { close(done) }, nil
}
