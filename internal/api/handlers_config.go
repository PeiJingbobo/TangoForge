package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"tangoforge/internal/auth"
	"tangoforge/internal/config"
	"tangoforge/internal/llm"
)

// 全局配置端点（GET/PUT /api/config）——首选项页数据源（TF-029 布局改造新增）。
//
// 语义：
//   - 仅 UI（回环 + X-UI-Token，识别层保证）；Agent / 远程一律 403（配置含密钥）；
//   - GET 返回脱敏视图（api_key / api_token 掩码，不暴露完整凭据）；
//   - PUT 全量覆盖：校验（config.Validate，失败 422 CONFIG_INVALID 不落盘）
//     → 写盘（SaveGlobal）→ 内存热更新（setConfig）→ 审计 config.updated；
//   - api_key / api_token 传空 = 保留原值（掩码不可逆，UI 以「留空不修改」交互）。
func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	if actor := auth.ActorFrom(r.Context()); actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"全局配置仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": s.configView()})
}

func (s *Server) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	actor := auth.ActorFrom(r.Context())
	if actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"全局配置仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	var req configPutReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}

	next := s.currentConfig()
	if req.Port != nil {
		next.Port = *req.Port
	}
	if req.RemoteAccess != nil {
		next.RemoteAccess = *req.RemoteAccess
	}
	if req.APIToken != nil && *req.APIToken != "" {
		next.APIToken = *req.APIToken
	}
	if req.LLM != nil {
		if req.LLM.BaseURL != nil {
			next.LLM.BaseURL = *req.LLM.BaseURL
		}
		if req.LLM.APIKey != nil && *req.LLM.APIKey != "" {
			next.LLM.APIKey = *req.LLM.APIKey
		}
		if req.LLM.Model != nil {
			next.LLM.Model = *req.LLM.Model
		}
		if req.LLM.APIKind != nil {
			next.LLM.APIKind = *req.LLM.APIKind
		}
		if req.LLM.TimeoutSec != nil {
			next.LLM.TimeoutSec = *req.LLM.TimeoutSec
		}
		if req.LLM.Retries != nil {
			next.LLM.Retries = *req.LLM.Retries
		}
		if req.LLM.MaxTokens != nil {
			next.LLM.MaxTokens = *req.LLM.MaxTokens
		}
		if req.LLM.Concurrency != nil {
			next.LLM.Concurrency = *req.LLM.Concurrency
		}
	}

	// 校验通过才真正保存（QA：设置实时保存但值需有效）。
	if err := config.Validate(next); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "CONFIG_INVALID", err.Error(), "")
		return
	}

	if s.configPath != "" {
		if err := config.SaveGlobal(s.configPath, next); err != nil {
			writeError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED",
				"全局配置写入失败", err.Error())
			return
		}
	}
	s.SetConfig(&next)
	s.audit.Write(r.Context(), "global", auditEntryOf(r, "config.updated", "global", "ok", ""))

	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": s.configView()})
}

// configView 脱敏视图（GET / PUT 响应体）。
func (s *Server) configView() configView {
	cfg := s.currentConfig()
	return configView{
		Port:         cfg.Port,
		RemoteAccess: cfg.RemoteAccess,
		APIToken:     maskSecret(cfg.APIToken),
		LLM: llmView{
			BaseURL:     cfg.LLM.BaseURL,
			APIKey:      maskSecret(cfg.LLM.APIKey),
			Model:       cfg.LLM.Model,
			APIKind:     cfg.LLM.APIKind,
			TimeoutSec:  cfg.LLM.TimeoutSec,
			Retries:     cfg.LLM.Retries,
			MaxTokens:   cfg.LLM.MaxTokens,
			Concurrency: cfg.LLM.Concurrency,
		},
	}
}

// maskSecret 掩码敏感凭据：前 4 位 + **** + 后 2 位；短值全掩。
func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	r := []rune(v)
	if len(r) <= 6 {
		return "******"
	}
	return string(r[:4]) + "****" + string(r[len(r)-2:])
}

type configView struct {
	Port         int     `json:"port"`
	RemoteAccess bool    `json:"remote_access"`
	APIToken     string  `json:"api_token"`
	LLM          llmView `json:"llm"`
}

type llmView struct {
	BaseURL     string `json:"base_url"`
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	APIKind     string `json:"api_kind"`
	TimeoutSec  int    `json:"timeout_sec"`
	Retries     int    `json:"retries"`
	MaxTokens   int    `json:"max_tokens"`
	Concurrency int    `json:"concurrency"`
}

type configPutReq struct {
	Port         *int        `json:"port"`
	RemoteAccess *bool       `json:"remote_access"`
	APIToken     *string     `json:"api_token"`
	LLM          *llmPutView `json:"llm"`
}

type llmPutView struct {
	BaseURL     *string `json:"base_url"`
	APIKey      *string `json:"api_key"`
	Model       *string `json:"model"`
	APIKind     *string `json:"api_kind"`
	TimeoutSec  *int    `json:"timeout_sec"`
	Retries     *int    `json:"retries"`
	MaxTokens   *int    `json:"max_tokens"`
	Concurrency *int    `json:"concurrency"`
}

// handleConfigTestLLM 测试大模型连接（POST /api/config/test，TF-041 引导 Step 1）。
//
// 语义：
//   - 仅 UI（配置含密钥）；豁免 X-Project（全局配置）；
//   - body 为**暂存** LLM 配置（未保存，引导流程先测后存）：
//     {base_url, api_key, model, api_kind}（api_key 为空 → 沿用已保存配置，
//     因为 GET 返回掩码不可逆，UI 留空表示不修改）；
//   - 用该配置构造临时 llm.Client 发一个最小请求（"ping"），成功 → {ok:true}；
//     失败 → 422 LLM_TEST_FAILED + 人类可读原因（不含密钥）。
func (s *Server) handleConfigTestLLM(w http.ResponseWriter, r *http.Request) {
	actor := auth.ActorFrom(r.Context())
	if actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"测试大模型连接仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	var req struct {
		BaseURL *string `json:"base_url"`
		APIKey  *string `json:"api_key"`
		Model   *string `json:"model"`
		APIKind *string `json:"api_kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	// 构造暂存配置：缺省字段沿用当前保存值。
	cur := s.currentConfig()
	cfg := cur.LLM
	if req.BaseURL != nil {
		cfg.BaseURL = strings.TrimSpace(*req.BaseURL)
	}
	if req.APIKey != nil && *req.APIKey != "" {
		cfg.APIKey = *req.APIKey
	}
	if req.Model != nil {
		cfg.Model = strings.TrimSpace(*req.Model)
	}
	if req.APIKind != nil {
		cfg.APIKind = strings.TrimSpace(*req.APIKind)
	}

	// 最小连通性测试：单条极短请求。
	client, err := llm.New(llm.FromConfig(cfg), s.logger)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "LLM_TEST_FAILED",
			"大模型未完整配置（base_url / api_key / model）", err.Error())
		return
	}
	_, err = client.Complete(r.Context(), llm.Request{System: "ping", User: "reply with ok"})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "LLM_TEST_FAILED",
			"连接失败: "+llmErrorText(err), "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]bool{"ok": true}})
}

// llmErrorText 将 LLM 错误转为简洁人类可读文本（不含密钥）。
func llmErrorText(err error) string {
	if err == nil {
		return ""
	}
	// 常见 LLM 错误码 → 中文提示。
	var code string
	if errors.Is(err, llm.ErrNotConfigured) {
		code = "LLM_NOT_CONFIGURED"
	} else if errors.Is(err, llm.ErrTimeout) {
		code = "LLM_TIMEOUT"
	} else if errors.Is(err, llm.ErrAPIStatus) {
		code = "LLM_API_ERROR"
	} else if errors.Is(err, llm.ErrInvalidResponse) {
		code = "LLM_INVALID_RESPONSE"
	} else if errors.Is(err, llm.ErrTruncated) {
		code = "LLM_TRUNCATED"
	}
	if code == "" {
		return err.Error()
	}
	switch code {
	case "LLM_NOT_CONFIGURED":
		return "未配置（base_url / api_key / model 缺失）"
	case "LLM_TIMEOUT":
		return "请求超时（检查 base_url 与网络）"
	case "LLM_API_ERROR":
		return "API 返回错误（检查 api_key / base_url / api_kind）"
	case "LLM_INVALID_RESPONSE":
		return "响应格式异常（检查 api_kind 兼容类型）"
	case "LLM_TRUNCATED":
		return "响应被截断"
	default:
		return err.Error()
	}
}
