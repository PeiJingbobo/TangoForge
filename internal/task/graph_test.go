package task

import (
	"context"
	"testing"
)

// TestGraph_Basic（TF-017：graph 数据组装，MCP graph_get 与 HTTP /api/graph 复用）。
func TestGraph_Basic(t *testing.T) {
	svc, wd := newTestEnv(t)
	ctx := context.Background()

	child := mustCreate(t, svc, wd, CreateInput{Title: "子任务"})
	parent := mustCreate(t, svc, wd, CreateInput{Title: "父任务", DependsOn: []string{child.ID}})
	if _, err := svc.Update(ctx, wd, child.ID, UpdateInput{ParentID: strPtrPtr(&parent.ID)}); err != nil {
		t.Fatal(err)
	}
	// 归档一个任务（应从 nodes 排除）。
	archived := mustCreate(t, svc, wd, CreateInput{Title: "归档任务"})
	if _, err := svc.Archive(ctx, wd, archived.ID); err != nil {
		t.Fatal(err)
	}

	g, err := svc.Graph(ctx, wd)
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("nodes 应排除 archived: %d", len(g.Nodes))
	}
	// parent 边：父→子；dependency 边：被依赖→依赖（child→parent）。
	hasParent := false
	hasDep := false
	for _, e := range g.Edges {
		if e.Type == "parent" && e.From == parent.ID && e.To == child.ID {
			hasParent = true
		}
		if e.Type == "dependency" && e.From == child.ID && e.To == parent.ID {
			hasDep = true
		}
	}
	if !hasParent || !hasDep {
		t.Fatalf("边不完整: %+v", g.Edges)
	}
}

// TestGraph_EmptyProject：nodes/edges 为空数组（非 nil，JSON 友好）。
func TestGraph_EmptyProject(t *testing.T) {
	svc, wd := newTestEnv(t)
	g, err := svc.Graph(context.Background(), wd)
	if err != nil {
		t.Fatal(err)
	}
	if g.Nodes == nil || g.Edges == nil {
		t.Fatalf("nodes/edges 应为空数组: %+v", g)
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Fatalf("空项目应为空: %+v", g)
	}
}
