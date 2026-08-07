package guide

import (
	"strings"
	"testing"
)

func TestRender_ContainsSections(t *testing.T) {
	md := Render(19810)
	for _, section := range []string{
		"# TangoForge 使用指南",
		"## 1. 核心概念",
		"## 2. HTTP API",
		"## 3. MCP",
		"## 4. CLI",
		"## 5. 业务语义速查",
		"`/api/guide`",
		"`tangoforge mcp`",
		"http://127.0.0.1:19810",
	} {
		if !strings.Contains(md, section) {
			t.Fatalf("说明书缺少段落: %q", section)
		}
	}
}

func TestRender_EndpointsAndToolsCoverage(t *testing.T) {
	md := Render(0)
	for _, path := range []string{"`/api/tasks`", "`/api/projects`", "`/api/skills/install`", "`/api/guide`"} {
		if !strings.Contains(md, path) {
			t.Fatalf("说明书缺少端点: %s", path)
		}
	}
	// 工具表关键项。
	for _, tool := range []string{"`task_list`", "`guide`", "`skill_install`"} {
		if !strings.Contains(md, tool) {
			t.Fatalf("说明书缺少工具: %s", tool)
		}
	}
	// 语义速查关键项。
	for _, kw := range []string{"PROJECT_NOT_FOUND", "INVALID_TRANSITION", "PERMISSION_DENIED", "STATUS_IN_USE"} {
		if !strings.Contains(md, kw) {
			t.Fatalf("说明书缺少语义: %s", kw)
		}
	}
}

func TestRender_DefaultPort(t *testing.T) {
	if !strings.Contains(Render(0), "http://127.0.0.1:19810") {
		t.Fatal("默认端口应为 19810")
	}
	if !strings.Contains(Render(8080), "http://127.0.0.1:8080") {
		t.Fatal("应使用传入端口")
	}
}
