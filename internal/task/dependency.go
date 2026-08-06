package task

import (
	"context"
)

// 依赖关系（depends_on）校验（TF-008，docs/TASK-SEMANTICS.md §9）。
//
// 语义（QA 确认）：
//   - 方向：A.depends_on=[B] = A 依赖 B（B 是 A 的前置）；环 = 沿 depends_on 图走回自身；
//   - 依赖任务必须存在（DEPENDENCY_NOT_FOUND，Q2-A）；
//   - 依赖已归档任务允许（Q3-A，归档不阻断依赖，与 TF-007 提示语义闭环）；
//   - 自依赖（X 依赖 X）→ CIRCULAR_DEPENDENCY（Q7-A）；
//   - 多跳环（Q5-A）：基于"更新后的集合 + 其余任务现有集合"多跳 DFS 检测；
//   - 校验先于写入（Q4-A）：Create/Update 均为单语句原子操作，校验失败直接返回，
//     不产生任何脏数据（语义等价"校验在事务内完成"）。

// validateDependencies 校验任务 taskID 的 depends_on 集合。
// taskID 为空（创建新任务）时跳过环检测中的自引用（新任务无入边，仅校验存在性）。
func (s *service) validateDependencies(ctx context.Context, repo TaskRepo, taskID string, dependsOn []string) error {
	if len(dependsOn) == 0 {
		return nil
	}

	// 1. 存在性 + 自依赖。
	for _, d := range dependsOn {
		if d == taskID {
			return NewCircularDependency(taskID)
		}
		t, err := repo.GetByID(ctx, d)
		if err != nil {
			return err
		}
		if t == nil {
			return NewDependencyNotFound(d)
		}
	}
	if taskID == "" {
		return nil // 新任务无入边，不存在多跳环
	}

	// 2. 多跳环检测：构建依赖图（目标任务用更新后集合，其余用现有集合）。
	all, err := repo.List(ctx)
	if err != nil {
		return err
	}
	graph := make(map[string][]string, len(all))
	for i := range all {
		if all[i].ID == taskID {
			graph[taskID] = dependsOn
		} else {
			graph[all[i].ID] = all[i].DependsOn
		}
	}
	for _, d := range dependsOn {
		if cycleReaches(graph, d, taskID, make(map[string]bool)) {
			return NewCircularDependency(taskID)
		}
	}
	return nil
}

// cycleReaches 从 start 沿依赖图 DFS，判断能否到达 target（环判定）。
// visited 防止共享依赖节点重复遍历。
func cycleReaches(graph map[string][]string, start, target string, visited map[string]bool) bool {
	if start == target {
		return true
	}
	if visited[start] {
		return false
	}
	visited[start] = true
	for _, next := range graph[start] {
		if cycleReaches(graph, next, target, visited) {
			return true
		}
	}
	return false
}
