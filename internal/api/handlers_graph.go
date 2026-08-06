package api

import (
	"net/http"

	"tangoforge/internal/auth"
	"tangoforge/internal/task"
)

// GraphEdge 图边：from → to（方向见 TASK-SEMANTICS §9：A.depends_on=[B] 表示 A 依赖 B）。
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // parent | dependency
}

// GraphData 全景图数据（服务端不聚簇，REQUIREMENTS.md §6）。
type GraphData struct {
	Nodes []task.Task `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// handleGraph 全景图全量数据（GET /api/graph，graph.read）。
// 返回全部未归档任务（排除回收站）的扁平列表 + 父子/依赖边；服务端不聚簇。
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	res, err := s.tasks.List(r.Context(), workdir, task.ListFilter{})
	if err != nil {
		writeBizError(w, err)
		return
	}

	nodes := flattenTree(res.Tree)
	edges := buildEdges(nodes)
	if nodes == nil {
		nodes = []task.Task{}
	}
	if edges == nil {
		edges = []GraphEdge{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": GraphData{Nodes: nodes, Edges: edges}})
}

// flattenTree 将任务树展平为全量扁平列表（保持任务完整字段，含 parent/depends 关系）。
func flattenTree(tree []*task.TaskTreeNode) []task.Task {
	var out []task.Task
	var walk func(nodes []*task.TaskTreeNode)
	walk = func(nodes []*task.TaskTreeNode) {
		for _, n := range nodes {
			out = append(out, n.Task)
			walk(n.Children)
		}
	}
	walk(tree)
	return out
}

// buildEdges 从任务列表构造边：parent（父子）+ dependency（A 依赖 B：A.depends_on=[B] → B→A）。
func buildEdges(nodes []task.Task) []GraphEdge {
	var edges []GraphEdge
	for _, t := range nodes {
		if t.ParentID != nil && *t.ParentID != "" {
			edges = append(edges, GraphEdge{From: *t.ParentID, To: t.ID, Type: "parent"})
		}
		for _, dep := range t.DependsOn {
			edges = append(edges, GraphEdge{From: dep, To: t.ID, Type: "dependency"})
		}
	}
	return edges
}
