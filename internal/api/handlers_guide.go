package api

import (
	"net/http"
	"tangoforge/internal/guide"
)

// handleGuide GET /api/guide（完全免鉴权，QA-S3）：返回 TangoForge 系统使用说明书（Markdown）。
// 内容由 internal/guide 单一来源渲染（端点表/工具表/CLI 表/语义速查）。
// 注册在 /api 中间件链之外（remoteAccessMiddleware / projectMiddleware / 来源识别均不经过），
// 任何来源（含局域网）可读；AI Agent 无 Skill 时先读本说明书即可掌握系统用法。
func (s *Server) handleGuide(w http.ResponseWriter, _ *http.Request) {
	md := guide.Render(s.currentConfig().Port)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(md))
}
