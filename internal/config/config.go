// Package config 负责 TangoForge 双层配置的加载、合并与热重载。
//
// 分层铁律（AGENTS.md §2.3）：本包只做「加载与合并」，不做任何业务判断；
// 状态机语义校验（TF-006）、权限语义（TF-011）等由对应业务层完成。
//
// 配置两层严禁混淆（AGENTS.md §2.3）：
//   - 全局配置（~/.taskboard-app/config.yaml）：端口、LLM、remote_access、api_token、ui_token；
//   - 项目配置（{workdir}/.taskboard/config.yaml）：state_machine、export 等业务配置。
//
// 时间约定（QA Q7）：时间戳统一 RFC3339 本地时区（业务层负责写入）。
package config

import (
	"path/filepath"
)

// DefaultPort 守护进程默认监听端口（与 cmd/daemon 保持一致）。
const DefaultPort = 19810

// GlobalConfig 全局配置（APP 级）：安全凭据、监听端口、LLM 服务。
// 支持 fsnotify 热重载（端口 / remote_access / LLM 即时生效，QA Q8）。
type GlobalConfig struct {
	// Port 守护进程监听端口，默认 19810。
	Port int `yaml:"port"`
	// RemoteAccess 远程访问开关；false 时非回环来源一律 403。
	RemoteAccess bool `yaml:"remote_access"`
	// APIToken 远程访问凭证（Authorization: Bearer <token>）。
	APIToken string `yaml:"api_token"`
	// UIToken UI 会话凭据，守护进程首次启动生成，仅回环来源有效。
	UIToken string `yaml:"ui_token"`
	// LLM LLM 服务配置（OpenAI 兼容，含 Ollama 等本地模型）。
	LLM LLMConfig `yaml:"llm"`
	// Knowledge 知识库全局配置（docs/KNOWLEDGE-BASE.md §4.1）。
	Knowledge KnowledgeGlobalConfig `yaml:"knowledge"`
}

// KnowledgeGlobalConfig 知识库全局配置（QA-K18 默认值 + QA-K23 向量搜索开关）。
//
// 布尔开关用 *bool：YAML 显式写 false 才关闭；未写（nil）→ WithDefaults 补默认 true
// （与「显式优于隐式」不冲突：关闭是显式行为，开启是默认值补齐）。
type KnowledgeGlobalConfig struct {
	// Enabled 总开关（关闭 = 不扫描/不监听/不索引，查询原样可用），默认 true。
	Enabled *bool `yaml:"enabled"`
	// FSNotify 实时监听开关，默认 true。
	FSNotify *bool `yaml:"fsnotify"`
	// StartupScan 启动扫描开关，默认 true。
	StartupScan *bool `yaml:"startup_scan"`
	// DebounceMS 防抖窗口（毫秒），默认 30000。
	DebounceMS int `yaml:"debounce_ms"`
	// EmbedConcurrency 摘要/嵌入 LLM 调用并发（默认 1 = 串行）。
	EmbedConcurrency int `yaml:"embed_concurrency"`
	// MaxIndexSize 超过该大小的文件不做向量嵌入（默认 512KB）。
	MaxIndexSize int `yaml:"max_index_size"`
	// VectorSearch 向量搜索开关（QA-K23）；未配置 Embedding 模型时强制禁用。
	VectorSearch *bool `yaml:"vector_search"`
	// SearchTopK 检索默认 top_k，默认 10。
	SearchTopK int `yaml:"search_top_k"`
	// SearchThreshold 检索相似度阈值，默认 0.3。
	SearchThreshold float64 `yaml:"search_threshold"`
	// DefaultDocDir 外部文件默认拷贝目录（空 = .taskboard/knowledge，QA-K13）。
	DefaultDocDir string `yaml:"default_doc_dir"`
}

// EnabledOn 返回知识库总开关（nil → 默认 true）。
func (k KnowledgeGlobalConfig) EnabledOn() bool { return k.Enabled == nil || *k.Enabled }

// FSNotifyOn 返回实时监听开关（nil → 默认 true）。
func (k KnowledgeGlobalConfig) FSNotifyOn() bool { return k.FSNotify == nil || *k.FSNotify }

// StartupScanOn 返回启动扫描开关（nil → 默认 true）。
func (k KnowledgeGlobalConfig) StartupScanOn() bool { return k.StartupScan == nil || *k.StartupScan }

// VectorSearchOn 返回向量搜索开关（nil → 默认 true）。
func (k KnowledgeGlobalConfig) VectorSearchOn() bool { return k.VectorSearch == nil || *k.VectorSearch }

// LLMConfig LLM 服务配置项集合（QA Q14 确认；QA P4-1 多协议扩展）。
type LLMConfig struct {
	// BaseURL 接口地址。openai：{base}/chat/completions；
	// anthropic：{base}/v1/messages（如 DeepSeek 兼容端点 https://api.deepseek.com/anthropic）；
	// responses：{base}/v1/responses。
	BaseURL string `yaml:"base_url"`
	// APIKey 认证密钥（本地模型可留空）；为空时客户端回退读取环境变量 DEEPSEEK_API_KEY（QA P4-1）。
	APIKey string `yaml:"api_key"`
	// Model 模型名（如 deepseek-v4-flash / claude-haiku-*）。
	Model string `yaml:"model"`
	// APIKind 协议类型：openai（默认）/ anthropic / responses（QA P4-1 多协议兼容）。
	APIKind string `yaml:"api_kind"`
	// TimeoutSec 请求超时（秒），默认 60。
	TimeoutSec int `yaml:"timeout_sec"`
	// Retries 重试次数，默认 1。
	Retries int `yaml:"retries"`
	// MaxTokens 单次响应最大 token 数，默认 4096。
	MaxTokens int `yaml:"max_tokens"`
	// Concurrency 请求并发数，默认 1。
	Concurrency int `yaml:"concurrency"`
	// Embedding 向量嵌入配置（QA-K5：独立于 chat 的 embedding 节）。
	Embedding EmbeddingConfig `yaml:"embedding"`
}

// EmbeddingConfig 向量嵌入配置（docs/KNOWLEDGE-BASE.md §4.1，QA-K5）。
type EmbeddingConfig struct {
	// BaseURL 为空 = 复用 llm.base_url。
	BaseURL string `yaml:"base_url"`
	// APIKey 为空 = 回退 llm.api_key / DEEPSEEK_API_KEY。
	APIKey string `yaml:"api_key"`
	// Model 如 nomic-embed-text (Ollama) / text-embedding-3-small；空 = embedding 未配置。
	Model string `yaml:"model"`
	// APIKind openai（POST {base}/embeddings）| ollama（POST {base}/api/embed）。
	APIKind string `yaml:"api_kind"`
	// TimeoutSec 请求超时（秒），默认 60。
	TimeoutSec int `yaml:"timeout_sec"`
	// MaxTokens 仅 openai 类生效，0 = 不限制。
	MaxTokens int `yaml:"max_tokens"`
}

// ProjectConfig 项目配置（{workdir}/.taskboard/config.yaml）：仅业务配置。
type ProjectConfig struct {
	// StateMachine 项目级状态机（每项目独立，REQUIREMENTS.md §2.2）。
	StateMachine StateMachine `yaml:"state_machine"`
	// Export 导出配置。
	Export ExportConfig `yaml:"export"`
	// Knowledge 知识库项目级配置（docs/KNOWLEDGE-BASE.md §4.2）。
	Knowledge KnowledgeProjectConfig `yaml:"knowledge"`
}

// KnowledgeProjectConfig 知识库项目级配置（QA-K13：项目级覆盖全局）。
type KnowledgeProjectConfig struct {
	// DefaultDocDir 外部文件默认拷贝目录；空 = 用全局逻辑（.taskboard/knowledge）。
	DefaultDocDir string `yaml:"default_doc_dir"`
}

// StateMachine 项目状态机定义。
type StateMachine struct {
	// States 状态列表（archived 为系统保留态，由归档/还原专用，不在此列）。
	States []State `yaml:"states"`
	// Transitions 合法流转规则（from → to[]）。
	Transitions []Transition `yaml:"transitions"`
}

// State 单个状态定义。
type State struct {
	Key   string `yaml:"key"`
	Label string `yaml:"label"`
	Color string `yaml:"color"`
}

// Transition 一条流转规则。
type Transition struct {
	From string   `yaml:"from"`
	To   []string `yaml:"to"`
}

// ExportConfig Markdown 导出配置。
type ExportConfig struct {
	// TemplatePath 自定义导出模板路径；空 = 默认模板。
	TemplatePath string `yaml:"template_path"`
}

// GlobalConfigPath 返回全局配置文件绝对路径（默认 ~/.taskboard-app/config.yaml）。
//
// homeDir 由调用方提供（os.UserHomeDir），本包不做环境探测（显式优于隐式）。
func GlobalConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".taskboard-app", "config.yaml")
}

// ProjectConfigPath 返回项目配置文件绝对路径（{workdir}/.taskboard/config.yaml）。
func ProjectConfigPath(workdir string) string {
	return filepath.Join(workdir, ".taskboard", "config.yaml")
}
