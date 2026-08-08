package task

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// 任务简短编号（TF-040）：格式 T{n:03d}（T01、T02…），项目库内唯一。
// - 创建任务：调用方未指定 → 自动分配下一个序号；
// - LLM 导入（ImportTasks）：沿用文档自带编号（如 P0），空/冲突 → 自动分配。
// 编号创建后不可修改（Update 不含 number 列）。

var numRe = regexp.MustCompile(`(?i)^t(\d+)$`)

// nextTaskNumber 计算下一个 T 序号：扫描库内全部 number（含归档），取最大 T 序号 + 1。
// 库内无任务 → T01。调用方负责在事务外读快照（WAL 铁律）。
func nextTaskNumber(ctx context.Context, repo TaskRepo) (string, error) {
	tasks, err := repo.List(ctx)
	if err != nil {
		return "", fmt.Errorf("task: list for next number: %w", err)
	}
	maxN := 0
	for _, t := range tasks {
		if n, ok := parseTaskNumber(t.Number); ok && n > maxN {
			maxN = n
		}
	}
	return formatTaskNumber(maxN + 1), nil
}

// ensureTaskNumbers 批量分配编号（ImportTasks 事务外调用）：
// - 已指定且未冲突 → 沿用（文档编号优先）；
// - 空或冲突（库内已用 / 本批内重复）→ 自动分配 T 序号（从库内最大序号 + 本批已分配推进）。
// 就地修改 tasks[i].Number。
func ensureTaskNumbers(ctx context.Context, repo TaskRepo, tasks []Task) error {
	used := make(map[string]bool) // 库内已用编号
	all, err := repo.List(ctx)
	if err != nil {
		return fmt.Errorf("task: list for numbers: %w", err)
	}
	maxN := 0
	for _, t := range all {
		if t.Number != "" {
			used[t.Number] = true
		}
		if n, ok := parseTaskNumber(t.Number); ok && n > maxN {
			maxN = n
		}
	}
	for i := range tasks {
		num := tasks[i].Number
		if num != "" && !used[num] {
			used[num] = true // 沿用文档编号
			continue
		}
		// 空或冲突 → 分配下一个 T 序号。
		for {
			maxN++
			next := formatTaskNumber(maxN)
			if !used[next] {
				used[next] = true
				tasks[i].Number = next
				break
			}
		}
	}
	return nil
}

// parseTaskNumber 解析 T 序号（大小写不敏感）；非 T 格式返回 false。
func parseTaskNumber(s string) (int, bool) {
	m := numRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// formatTaskNumber 格式化为 T{n:03d}（超过 999 自然扩展位数）。
func formatTaskNumber(n int) string {
	return fmt.Sprintf("T%03d", n)
}

// numberAvailable 检查编号在库内是否可用（含归档；编号全局唯一）。
func (s *service) numberAvailable(ctx context.Context, repo TaskRepo, number string) bool {
	all, err := repo.List(ctx)
	if err != nil {
		return false
	}
	for _, t := range all {
		if t.Number == number {
			return false
		}
	}
	return true
}
