package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultStateMachine 返回默认四态状态机（REQUIREMENTS.md §2.2）。
//
// archived 为系统保留态，由「归档/还原」操作专用，不出现在普通流转编辑中，
// 因此默认状态机仅含 todo / doing / done 三态与流转规则。
func DefaultStateMachine() StateMachine {
	return StateMachine{
		States: []State{
			{Key: "todo", Label: "待办", Color: "#9aa0a6"},
			{Key: "doing", Label: "进行中", Color: "#1a73e8"},
			{Key: "done", Label: "已完成", Color: "#34a853"},
		},
		Transitions: []Transition{
			{From: "todo", To: []string{"doing", "done"}},
			{From: "doing", To: []string{"todo", "done"}},
			{From: "done", To: []string{"doing", "todo"}},
		},
	}
}

// DefaultProjectConfig 返回项目配置默认值（默认状态机 + 默认导出模板）。
func DefaultProjectConfig() ProjectConfig {
	return ProjectConfig{
		StateMachine: DefaultStateMachine(),
		Export: ExportConfig{
			TemplatePath: "", // 空 = 使用内置默认模板
		},
	}
}

// LoadProject 读取项目配置（{workdir}/.taskboard/config.yaml）。
//
// 缺失文件容错：config.yaml 不存在时返回默认配置（nil error），
// 由 TF-004 项目初始化时 SaveProject 落盘。
func LoadProject(workdir string) (ProjectConfig, error) {
	return LoadProjectFile(ProjectConfigPath(workdir))
}

// LoadProjectFile 从指定路径读取项目配置（供测试与内部复用）。
func LoadProjectFile(path string) (ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultProjectConfig(), nil
		}
		return ProjectConfig{}, fmt.Errorf("config: read project %s: %w", path, err)
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, fmt.Errorf("config: parse project %s: %w", path, err)
	}
	// 状态机缺失/为空时回退默认四态（TF-006 将做完整语义校验）。
	if len(cfg.StateMachine.States) == 0 {
		cfg.StateMachine = DefaultStateMachine()
	}
	return cfg, nil
}

// SaveProject 将项目配置写入 {workdir}/.taskboard/config.yaml（自动创建目录）。
func SaveProject(workdir string, cfg ProjectConfig) error {
	path := ProjectConfigPath(workdir)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: marshal project: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("config: write project %s: %w", path, err)
	}
	return nil
}
