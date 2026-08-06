package task

import (
	"strconv"
)

// priorityAliases 字符串别名 → 归一值（docs/TASK-SEMANTICS.md §3）。
//
// 注意：`low` 归一为 1（QA Q5 确认，区间 1–2 取低值，与 normal=3 / high=4 拉开梯度）。
var priorityAliases = map[string]int{
	"lowest":   0,
	"none":     0,
	"low":      1,
	"normal":   3,
	"default":  3,
	"high":     4,
	"highest":  5,
	"critical": 5,
	"urgent":   5,
}

// NormalizePriority 将 priority 输入归一化为 0–5 整数（严格模式，非法值拒绝）。
//
// 支持输入（docs/TASK-SEMANTICS.md §3）：
//   - nil → 0（缺省无优先级）
//   - int / int64 / float64（JSON number，须为整数）→ 原值并校验 0–5
//   - string → 别名表；否则按数字字符串解析（"3"），仍失败则拒绝
//
// 非法输入返回 *Error（Code=CodeTaskInvalid），不静默 fallback。
func NormalizePriority(v any) (int, error) {
	if v == nil {
		return 0, nil
	}
	switch t := v.(type) {
	case int:
		return checkPriorityRange(t)
	case int64:
		return checkPriorityRange(int(t))
	case float64:
		if t != float64(int(t)) {
			return 0, NewInvalid("priority 必须为 0-5 整数或字符串别名，got %v", t)
		}
		return checkPriorityRange(int(t))
	case string:
		if alias, ok := priorityAliases[t]; ok {
			return alias, nil
		}
		n, err := strconv.Atoi(t)
		if err != nil {
			return 0, NewInvalid("priority 必须为 0-5 整数或字符串别名，got %q", t)
		}
		return checkPriorityRange(n)
	default:
		return 0, NewInvalid("priority 必须为 0-5 整数或字符串别名，got %T", v)
	}
}

// checkPriorityRange 校验整数在 0–5 区间。
func checkPriorityRange(p int) (int, error) {
	if p < 0 || p > 5 {
		return 0, NewInvalid("priority 超出范围 0-5: %d", p)
	}
	return p, nil
}
