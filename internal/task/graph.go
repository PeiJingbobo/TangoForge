package task

import (
	"context"
)

// GraphEdge 图边：from → to（方向见 docs/TASK-SEMANTICS.md §9：A.depends_on=[B] → B→A）。
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // parent | dependency
}

// GraphData 全景图数据（服务端不聚簇，REQUIREMENTS.md §6；MCP graph_get 与 HTTP 复用）。
type GraphData struct {
	Nodes []Task      `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Graph 返回全景图全量数据（TF-017 从 api 层下沉，供 MCP graph_get 与 HTTP /api/graph 复用）。
//
// 语义（docs/TASK-SEMANTICS.md §12.5）：nodes = 全量未归档任务（排除 archived）；
// edges = parent（父→子）与 dependency（被依赖→依赖）两类；服务端不聚簇。
func (s *service) Graph(ctx context.Context, workdir string) (GraphData, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return GraphData{}, err
	}
	all, err := newSQLRepo(conn).List(ctx)
	if err != nil {
		return GraphData{}, err
	}

	nodes := make([]Task, 0, len(all))
	for _, t := range all {
		if t.Status == StatusArchived {
			continue
		}
		nodes = append(nodes, t)
	}

	edges := make([]GraphEdge, 0)
	for _, t := range nodes {
		if t.ParentID != nil && *t.ParentID != "" {
			edges = append(edges, GraphEdge{From: *t.ParentID, To: t.ID, Type: "parent"})
		}
		for _, dep := range t.DependsOn {
			edges = append(edges, GraphEdge{From: dep, To: t.ID, Type: "dependency"})
		}
	}
	if nodes == nil {
		nodes = []Task{}
	}
	if edges == nil {
		edges = []GraphEdge{}
	}
	return GraphData{Nodes: nodes, Edges: edges}, nil
}
