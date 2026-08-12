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
}

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
