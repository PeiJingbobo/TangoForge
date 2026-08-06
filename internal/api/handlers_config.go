package api

import (
	"encoding/json"
	"net/http"

	"tangoforge/internal/auth"
	"tangoforge/internal/config"
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
