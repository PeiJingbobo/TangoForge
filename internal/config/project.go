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
//
// 注意：SaveProject 是「全量替换」——yaml.Marshal 仅输出 ProjectConfig 已知字段，
// config.yaml 中的未知节（未来扩展字段）会被丢弃。需要保留未知节时请使用
// UpdateProjectFile（部分更新，TF-032）。
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

// UpdateProjectFile 部分更新项目配置（{workdir}/.taskboard/config.yaml，TF-032）。
//
// 语义：读原文件 → mutate 修改已知节（state_machine / export）→ 仅替换这两个顶层
// key，config.yaml 中的其它未知节（未来扩展字段）原样保留；文件缺失时按默认值创建。
// 与 SaveProject 的全量替换不同，UpdateProjectFile 适合「配置编辑页」的整写场景。
func UpdateProjectFile(workdir string, mutate func(*ProjectConfig)) error {
	path := ProjectConfigPath(workdir)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultProjectConfig()
			mutate(&cfg)
			return SaveProject(workdir, cfg)
		}
		return fmt.Errorf("config: read project %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("config: parse project %s: %w", path, err)
	}
	var cfg ProjectConfig
	if err := doc.Decode(&cfg); err != nil {
		return fmt.Errorf("config: decode project %s: %w", path, err)
	}
	// 状态机缺失/为空时回退默认四态（与 LoadProjectFile 一致）。
	if len(cfg.StateMachine.States) == 0 {
		cfg.StateMachine = DefaultStateMachine()
	}
	mutate(&cfg)
	if err := setProjectNode(&doc, cfg); err != nil {
		return fmt.Errorf("config: merge project %s: %w", path, err)
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("config: marshal project %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("config: write project %s: %w", path, err)
	}
	return nil
}

// setProjectNode 在 yaml 文档顶层 mapping 中替换/追加 state_machine 与 export 两节
// （未知节不动，保持原键序）。
func setProjectNode(doc *yaml.Node, cfg ProjectConfig) error {
	root := mappingOf(doc)
	if root == nil {
		return errors.New("yaml 文档顶层不是 mapping")
	}
	smNode, err := marshalNode(cfg.StateMachine)
	if err != nil {
		return err
	}
	exNode, err := marshalNode(cfg.Export)
	if err != nil {
		return err
	}
	setMapping(root, "state_machine", smNode)
	setMapping(root, "export", exNode)
	return nil
}

// mappingOf 返回文档顶层 mapping 节点（跳过 DocumentNode 包装）。
func mappingOf(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind == yaml.MappingNode {
		return doc
	}
	return nil
}

// setMapping 在 mapping 中按 key 替换值节点；不存在则追加（保持原键序）。
func setMapping(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
}

// marshalNode 将任意值序列化为 yaml 内容节点（解包 DocumentNode）。
func marshalNode(v any) (*yaml.Node, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		return nil, err
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0], nil
	}
	return &n, nil
}
