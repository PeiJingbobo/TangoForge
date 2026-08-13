package mcp

import (
	"context"
	"os"
	"strconv"
	"tangoforge/internal/knowledge"

	"github.com/mark3labs/mcp-go/mcp"
)

// 知识库工具定义（docs/KNOWLEDGE-BASE.md §7.1，TF-050/051）。
// 每个工具首参 project（强制）；权限映射同 HTTP（read 默认只读、write/index 默认拒绝）。

// toolKnowledgeList 文档/库列表。
var toolKnowledgeList = mcp.NewTool("knowledge_list",
	mcp.WithDescription("知识库列表：文档与库（含文档数）。需显式 project 参数；kb_id/status 可选过滤。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("kb_id", mcp.Description("按库 ID 过滤文档")),
	mcp.WithString("status", mcp.Description("按状态过滤文档（ok/missing/failed）")),
)

// toolKnowledgeSearch 向量检索。
var toolKnowledgeSearch = mcp.NewTool("knowledge_search",
	mcp.WithDescription("知识库向量语义检索，返回文档 + 命中片段。需显式 project 参数；"+
		"q 必填；kb_id/top_k 可选。未配置 llm.embedding 时返回 EMBEDDING_NOT_CONFIGURED。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("q", mcp.Required(), mcp.Description("检索关键词")),
	mcp.WithString("kb_id", mcp.Description("库 ID 过滤（可选）")),
	mcp.WithString("top_k", mcp.Description("返回文档数（默认 10，上限 50）")),
)

// toolKnowledgeRead 文档详情/阅读。
var toolKnowledgeRead = mcp.NewTool("knowledge_read",
	mcp.WithDescription("读取知识库文档详情（真实路径/摘要/关联）或原文内容。需显式 project 参数；"+
		"id 或 path 二选一；content=true 时返回原文文本（二进制仅返回路径）。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("id", mcp.Description("文档 ID（与 path 二选一）")),
	mcp.WithString("path", mcp.Description("文档路径（与 id 二选一）")),
	mcp.WithString("content", mcp.Description("true = 返回原文内容")),
)

// toolKnowledgeLink 任务关联。
var toolKnowledgeLink = mcp.NewTool("knowledge_link",
	mcp.WithDescription("为任务关联知识库文档。需显式 project 参数；task_id 必填；"+
		"document_id 或 path 二选一（path 未注册自动入库）；copy 可选（none/copy/auto）。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("task_id", mcp.Required(), mcp.Description("任务 ID")),
	mcp.WithString("document_id", mcp.Description("文档 ID（与 path 二选一）")),
	mcp.WithString("path", mcp.Description("磁盘路径（与 document_id 二选一；未注册自动入库）")),
	mcp.WithString("copy", mcp.Description("外部文件拷贝语义：none/copy/auto（默认 auto）")),
)

// toolKnowledgeUnlink 解除任务关联。
var toolKnowledgeUnlink = mcp.NewTool("knowledge_unlink",
	mcp.WithDescription("解除任务与知识库文档的关联（文档本身保留）。需显式 project 参数；task_id + document_id 必填。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("task_id", mcp.Required(), mcp.Description("任务 ID")),
	mcp.WithString("document_id", mcp.Required(), mcp.Description("文档 ID")),
)

// toolKnowledgeRelink 重新链接。
var toolKnowledgeRelink = mcp.NewTool("knowledge_relink",
	mcp.WithDescription("重新链接丢失的知识库文档到新路径（重置并重建索引，保留库成员与任务关联）。"+
		"需显式 project 参数；id + new_path 必填。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("id", mcp.Required(), mcp.Description("文档 ID")),
	mcp.WithString("new_path", mcp.Required(), mcp.Description("新路径（必须存在且为文本）")),
	mcp.WithString("copy", mcp.Description("外部文件拷贝语义：none/copy/auto（默认 auto）")),
)

// toolKnowledgeScan 手动扫描。
var toolKnowledgeScan = mcp.NewTool("knowledge_scan",
	mcp.WithDescription("手动触发知识库文件扫描与重索引（变化检测 + 模型漂移重嵌）。需显式 project 参数；权限 knowledge.index。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
)

// toolKnowledgeEdit 编辑原文。
var toolKnowledgeEdit = mcp.NewTool("knowledge_edit",
	mcp.WithDescription("编辑知识库文档原文（直接写盘原文件 → 触发重新索引；二进制禁止）。"+
		"需显式 project 参数；id + content 必填。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("id", mcp.Required(), mcp.Description("文档 ID")),
	mcp.WithString("content", mcp.Required(), mcp.Description("新的文件内容")),
)

// handleKnowledgeList knowledge_list 处理器。
func (s *Server) handleKnowledgeList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "knowledge.read", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		f := knowledge.DocumentFilter{
			Status: strArg(args, "status", ""),
			Size:   knowledge.MaxPageSize,
		}
		if kb := strArg(args, "kb_id", ""); kb != "" {
			id, err := strconv.ParseInt(kb, 10, 64)
			if err == nil {
				f.KBID = id
			}
		}
		res, err := s.deps.Knowledge.ListDocuments(ctx, workdir, f)
		if err != nil {
			return nil, err
		}
		bases, err := s.deps.Knowledge.ListBases(ctx, workdir)
		if err != nil {
			return nil, err
		}
		return map[string]any{"documents": res.Items, "total": res.Total, "bases": bases}, nil
	})
}

// handleKnowledgeSearch knowledge_search 处理器。
func (s *Server) handleKnowledgeSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "knowledge.read", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		q := knowledge.SearchQuery{Q: strArg(args, "q", "")}
		if kb := strArg(args, "kb_id", ""); kb != "" {
			q.KBID, _ = strconv.ParseInt(kb, 10, 64)
		}
		if k := strArg(args, "top_k", ""); k != "" {
			q.TopK, _ = strconv.Atoi(k)
		}
		res, err := s.deps.Knowledge.Search(ctx, workdir, q)
		if err != nil {
			return nil, err
		}
		return res, nil
	})
}

// handleKnowledgeRead knowledge_read 处理器。
func (s *Server) handleKnowledgeRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "knowledge.read", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		id := strArg(args, "id", "")
		if id == "" {
			// 按 path 定位：列出文档按 abs_path 匹配（简单场景用注册路径）。
			path := strArg(args, "path", "")
			if path == "" {
				return nil, knowledge.NewDocumentInvalid("id 或 path 必填其一")
			}
			docs, err := s.deps.Knowledge.ListDocuments(ctx, workdir, knowledge.DocumentFilter{Q: path, Size: knowledge.MaxPageSize})
			if err != nil {
				return nil, err
			}
			for _, d := range docs.Items {
				if d.AbsPath == path || d.Path == path || d.RelPath == path {
					id = d.ID
					break
				}
			}
			if id == "" {
				return nil, knowledge.ErrDocumentNotFound
			}
		}
		doc, err := s.deps.Knowledge.GetDocument(ctx, workdir, id)
		if err != nil {
			return nil, err
		}
		if strArg(args, "content", "") == "true" && doc.Type == knowledge.DocTypeText {
			content, rerr := readTextFile(doc.AbsPath)
			if rerr != nil {
				return nil, knowledge.NewDocumentMissing("文件不可读: %s", doc.AbsPath)
			}
			return map[string]any{"document": doc, "content": content}, nil
		}
		return doc, nil
	})
}

// handleKnowledgeLink knowledge_link 处理器。
func (s *Server) handleKnowledgeLink(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "knowledge.write", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		err := s.deps.Knowledge.LinkTask(ctx, workdir,
			strArg(args, "task_id", ""),
			strArg(args, "document_id", ""),
			strArg(args, "path", ""),
			strArg(args, "copy", ""),
			nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "task_id": strArg(args, "task_id", "")}, nil
	})
}

// handleKnowledgeUnlink knowledge_unlink 处理器。
func (s *Server) handleKnowledgeUnlink(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "knowledge.write", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		err := s.deps.Knowledge.UnlinkTask(ctx, workdir, strArg(args, "task_id", ""), strArg(args, "document_id", ""))
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})
}

// handleKnowledgeRelink knowledge_relink 处理器。
func (s *Server) handleKnowledgeRelink(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "knowledge.write", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		doc, err := s.deps.Knowledge.RelinkDocument(ctx, workdir,
			strArg(args, "id", ""), strArg(args, "new_path", ""), strArg(args, "copy", ""))
		if err != nil {
			return nil, err
		}
		return doc, nil
	})
}

// handleKnowledgeScan knowledge_scan 处理器。
func (s *Server) handleKnowledgeScan(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "knowledge.index", req.GetArguments(), func(ctx context.Context, _ string, _ map[string]any) (any, error) {
		if s.deps.KnowledgeScanner == nil {
			return nil, knowledge.ErrIndexFailed
		}
		return s.deps.KnowledgeScanner.Scan(ctx)
	})
}

// handleKnowledgeEdit knowledge_edit 处理器。
func (s *Server) handleKnowledgeEdit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "knowledge.write", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		err := s.deps.Knowledge.UpdateContent(ctx, workdir, strArg(args, "id", ""), strArg(args, "content", ""))
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "id": strArg(args, "id", "")}, nil
	})
}

// readTextFile 读取文本文件（MCP 工具内辅助；上限 2MB）。
func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	const maxRead = 2 << 20
	if len(data) > maxRead {
		data = data[:maxRead]
	}
	return string(data), nil
}
