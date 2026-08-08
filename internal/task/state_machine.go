package task

import (
	"context"
	"fmt"
	"strings"
	"tangoforge/internal/config"
)

// 状态机业务层（TF-006，docs/TASK-SEMANTICS.md §5.1/§5.2）。
//
// 语义要点：
//   - 项目级可配置状态机，定义于 {workdir}/.taskboard/config.yaml 的 state_machine 节；
//   - config.yaml 缺失 / state_machine 节缺失 → 回退默认四态（todo/doing/done + 保留态 archived）；
//   - 流转校验规则（QA Q1-B 宽松 + Q3-A 特例）：transitions 整体为空 → 拒绝一切；
//     非空时 from 有规则 → 目标必须在 to 列表；from 无规则 → 放行；
//   - 编辑校验：key 唯一、非空、非 archived、states ≥ 1、transitions 引用存在；
//     STATUS_IN_USE：占用状态（任务数 > 0，archived 任务不统计）不可删除/重命名。

// loadStateMachine 读取项目状态机定义（缺失回退默认四态）。
// 统一加载入口（QA Q10-A）：Create 存在性校验 / ChangeStatus 流转校验 /
// GetStateMachine / UpdateStateMachine 全部复用。
func loadStateMachine(workdir string) (config.StateMachine, error) {
	cfg, err := config.LoadProject(workdir)
	if err != nil {
		return config.StateMachine{}, fmt.Errorf("task: load state machine %s: %w", workdir, err)
	}
	return cfg.StateMachine, nil
}

// stateExists 判断 key 是否在状态机 states 中。
func stateExists(sm config.StateMachine, key string) bool {
	for _, st := range sm.States {
		if st.Key == key {
			return true
		}
	}
	return false
}

// validateTransition 校验 from → to 是否合法（docs/TASK-SEMANTICS.md §5.1）。
//
// 规则（QA Q1-B 宽松 + Q3-A 特例）：
//   - transitions 整体为空（states 自定义但无任何规则）→ 拒绝所有普通流转（安全默认）；
//   - 存在 from 规则：目标在 to 列表 → 放行；否则 INVALID_TRANSITION；
//   - from 未定义规则 → 放行任意流转（宽松）。
//
// 前置条件：from / to 均已被调用方校验存在于 states。
func validateTransition(sm config.StateMachine, from, to string) error {
	if len(sm.Transitions) == 0 {
		return NewInvalidTransition(from, to)
	}
	for _, tr := range sm.Transitions {
		if tr.From != from {
			continue
		}
		for _, t := range tr.To {
			if t == to {
				return nil
			}
		}
		return NewInvalidTransition(from, to)
	}
	return nil // from 无规则 → 放行（Q1-B）
}

// GetStateMachine 读取项目状态机定义（缺失回退默认四态）。
func (s *service) GetStateMachine(ctx context.Context, workdir string) (config.StateMachine, error) {
	if _, err := s.projectDB(ctx, workdir); err != nil {
		return config.StateMachine{}, err
	}
	return loadStateMachine(workdir)
}

// UpdateStateMachine 编辑状态机（docs/TASK-SEMANTICS.md §5.2）。
//
// 校验链：编辑校验（validateStateMachineEdit）→ 占用校验（STATUS_IN_USE）→
// 持久化 config.yaml（部分更新：替换 state_machine 节，保留 export 等其它节）→ 写钩子。
func (s *service) UpdateStateMachine(ctx context.Context, workdir string, sm config.StateMachine) (config.StateMachine, error) {
	// 1+2. 编辑校验 + 占用校验（与 /api/project-config 全量写入共用，TF-032）。
	norm, err := s.ValidateStateMachineUpdate(ctx, workdir, sm)
	if err != nil {
		return config.StateMachine{}, err
	}

	// 3. 持久化（部分更新，保留其它配置节，Q8-A / TF-032）。
	if err := config.UpdateProjectFile(workdir, func(cfg *config.ProjectConfig) {
		cfg.StateMachine = norm
	}); err != nil {
		return config.StateMachine{}, fmt.Errorf("task: save state machine: %w", err)
	}
	s.logger.Debug("state machine updated", "workdir", workdir)
	s.emit(ctx, workdir, "state_machine.changed", workdir)
	return norm, nil
}

// ValidateStateMachineUpdate 校验状态机编辑（编辑校验 + 占用校验），不做持久化。
//
// 供 /api/project-config 全量写入（PUT）等需要「校验但不单独落盘」的场景复用，
// 避免状态机校验逻辑在两处漂移。返回规范化后的状态机。
func (s *service) ValidateStateMachineUpdate(ctx context.Context, workdir string, sm config.StateMachine) (config.StateMachine, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return config.StateMachine{}, err
	}

	// 1. 编辑校验 + 规范化（去空白 key、去重 to）。
	norm, err := validateStateMachineEdit(sm)
	if err != nil {
		return config.StateMachine{}, err
	}

	// 2. 占用校验：旧状态机中消失的 key 若有任务占用 → STATUS_IN_USE。
	current, err := loadStateMachine(workdir)
	if err != nil {
		return config.StateMachine{}, err
	}
	usage, err := s.statusUsage(ctx, newSQLRepo(conn))
	if err != nil {
		return config.StateMachine{}, err
	}
	newKeys := make(map[string]bool, len(norm.States))
	for _, st := range norm.States {
		newKeys[st.Key] = true
	}
	for _, st := range current.States {
		if !newKeys[st.Key] {
			if n := usage[st.Key]; n > 0 {
				return config.StateMachine{}, NewStatusInUse(st.Key, n)
			}
		}
	}
	return norm, nil
}

// validateStateMachineEdit 状态机编辑校验与规范化（docs/TASK-SEMANTICS.md §5.2 Q5-A）。
//
// 校验项：states ≥ 1；key 必填/去空白/唯一/非 archived；transitions 的 from/to
// 必须存在于 states；to 去重（可为空 = 该状态不可流转出去）。
func validateStateMachineEdit(sm config.StateMachine) (config.StateMachine, error) {
	if len(sm.States) == 0 {
		return config.StateMachine{}, NewInvalid("状态机至少需要 1 个状态")
	}
	seen := make(map[string]bool, len(sm.States))
	norm := config.StateMachine{States: make([]config.State, 0, len(sm.States))}
	for _, st := range sm.States {
		key := strings.TrimSpace(st.Key)
		if key == "" {
			return config.StateMachine{}, NewInvalid("状态 key 必填")
		}
		if key == StatusArchived {
			return config.StateMachine{}, NewInvalid("archived 为系统保留态，不可出现在状态机中")
		}
		if seen[key] {
			return config.StateMachine{}, NewInvalid("状态 key 重复: %s", key)
		}
		seen[key] = true
		norm.States = append(norm.States, config.State{Key: key, Label: st.Label, Color: st.Color})
	}
	for _, tr := range sm.Transitions {
		from := strings.TrimSpace(tr.From)
		if !seen[from] {
			return config.StateMachine{}, NewInvalid("流转规则 from 状态不存在: %s", from)
		}
		toSeen := make(map[string]bool, len(tr.To))
		to := make([]string, 0, len(tr.To))
		for _, t := range tr.To {
			t = strings.TrimSpace(t)
			if !seen[t] {
				return config.StateMachine{}, NewInvalid("流转规则 to 状态不存在: %s", t)
			}
			if toSeen[t] {
				continue
			}
			toSeen[t] = true
			to = append(to, t)
		}
		norm.Transitions = append(norm.Transitions, config.Transition{From: from, To: to})
	}
	return norm, nil
}

// statusUsage 统计各状态占用任务数（archived 任务不参与统计，QA Q7-A）。
func (s *service) statusUsage(ctx context.Context, repo TaskRepo) (map[string]int, error) {
	tasks, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	usage := make(map[string]int)
	for _, t := range tasks {
		if t.Status == StatusArchived {
			continue
		}
		usage[t.Status]++
	}
	return usage, nil
}
