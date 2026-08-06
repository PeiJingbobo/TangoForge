// Package skill 负责 AI Skill 文件的扫描、索引与 skill_info 查询。
//
// 职责边界（REQUIREMENTS.md §8 / TASK-SEMANTICS.md §15，TF-020）：
//   - 文件系统为唯一数据源：{workdir}/.taskboard/skills/ 目录（一级，不递归）；
//   - skills 表（项目库）仅作缓存，扫描时同步 upsert / 删除失效行，**不反写文件**；
//   - 支持 YAML（name/version/description/instructions）与 Markdown（首个 # 标题为 name，全文为内容）双格式；
//   - 解析失败仅日志告警并跳过该文件，不阻断扫描；
//   - 扫描时机：启动时 + 每次 List / Info 查询时（轻量重扫，天然满足"删除文件后索引同步"），
//     不引入 fsnotify 常驻 watcher（QA P4-1）。
//
// 分层铁律（AGENTS.md §3.2）：本包为业务层，禁止引用 api / mcp / cmd。
package skill

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"tangoforge/internal/db"
)

// ErrProjectNotFound 表示工作目录未导入为项目（无 {workdir}/.taskboard/meta.db）。
var ErrProjectNotFound = errors.New("skill: project not found")

// ErrSkillNotFound 表示指定名称的 Skill 不存在。
var ErrSkillNotFound = errors.New("skill: not found")

// Skill 单个 Skill 信息（skill_info 返回结构）。
type Skill struct {
	// Name 唯一标识：YAML 的 name 字段 / Markdown 首个 # 标题。
	Name string `json:"name"`
	// Version 版本号（YAML 可选；Markdown 为空）。
	Version string `json:"version"`
	// Description 一句话描述（YAML 可选；Markdown 为空）。
	Description string `json:"description"`
	// Instructions 使用指引：YAML 的 instructions 字段；Markdown 为全文。
	Instructions string `json:"instructions"`
	// Content 原始文件内容（详情展示用）。
	Content string `json:"content"`
	// UpdatedAt 文件修改时间（RFC3339 本地时区）。
	UpdatedAt string `json:"updated_at"`
}

// Service Skill 业务服务：扫描文件系统 + 缓存同步 + 查询。
type Service struct {
	mu     sync.Mutex
	dbs    map[string]*sql.DB
	logger *slog.Logger
}

// NewService 构造 Skill 服务。
func NewService(logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{dbs: make(map[string]*sql.DB), logger: logger}
}

// projectDB 打开并缓存项目库连接（语义同 task.Service.projectDB）。
func (s *Service) projectDB(workdir string) (*sql.DB, error) {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("%w: %s 不是绝对路径", ErrProjectNotFound, workdir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, ok := s.dbs[clean]; ok {
		return conn, nil
	}
	if _, err := os.Stat(db.MetaDBPath(clean)); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProjectNotFound, workdir)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(clean))
	if err != nil {
		return nil, fmt.Errorf("skill: open project db %s: %w", clean, err)
	}
	s.dbs[clean] = conn
	return conn, nil
}

// List 返回全部 Skill：先重扫文件系统并同步缓存，再返回缓存内容（按名称排序）。
func (s *Service) List(ctx context.Context, workdir string) ([]Skill, error) {
	if err := s.scanAndSync(ctx, workdir); err != nil {
		return nil, err
	}
	conn, err := s.projectDB(workdir)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx,
		`SELECT name, version, description, instructions, content, updated_at FROM skills ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("skill: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Skill, 0)
	for rows.Next() {
		var sk Skill
		var version, desc, instr sql.NullString
		if err := rows.Scan(&sk.Name, &version, &desc, &instr, &sk.Content, &sk.UpdatedAt); err != nil {
			return nil, fmt.Errorf("skill: scan: %w", err)
		}
		sk.Version, sk.Description, sk.Instructions = version.String, desc.String, instr.String
		out = append(out, sk)
	}
	return out, rows.Err()
}

// Info 返回单个 Skill 详情；不存在 → ErrSkillNotFound。
func (s *Service) Info(ctx context.Context, workdir, name string) (Skill, error) {
	if err := s.scanAndSync(ctx, workdir); err != nil {
		return Skill{}, err
	}
	conn, err := s.projectDB(workdir)
	if err != nil {
		return Skill{}, err
	}
	var sk Skill
	var version, desc, instr sql.NullString
	err = conn.QueryRowContext(ctx,
		`SELECT name, version, description, instructions, content, updated_at FROM skills WHERE name = ?`,
		name).Scan(&sk.Name, &version, &desc, &instr, &sk.Content, &sk.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Skill{}, fmt.Errorf("%w: %s", ErrSkillNotFound, name)
	}
	if err != nil {
		return Skill{}, fmt.Errorf("skill: info %s: %w", name, err)
	}
	sk.Version, sk.Description, sk.Instructions = version.String, desc.String, instr.String
	return sk, nil
}

// Close 关闭全部缓存的项目库连接。
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for wd, conn := range s.dbs {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("skill: close %s: %w", wd, err)
		}
		delete(s.dbs, wd)
	}
	return firstErr
}

// scanAndSync 扫描 skills/ 目录并同步缓存表（upsert 新内容 + 删除失效行）。
func (s *Service) scanAndSync(ctx context.Context, workdir string) error {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return err
	}
	dir := filepath.Join(workdir, ".taskboard", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// 目录缺失（异常环境）：视为空，清空缓存。
			if _, err := conn.ExecContext(ctx, `DELETE FROM skills`); err != nil {
				return fmt.Errorf("skill: clear cache: %w", err)
			}
			return nil
		}
		return fmt.Errorf("skill: read dir %s: %w", dir, err)
	}

	found := make(map[string]bool)
	now := time.Now().Format(time.RFC3339)
	for _, e := range entries {
		if e.IsDir() {
			continue // 仅一级目录文件（QA P4-1）。
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".md" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			s.logger.Warn("skill: read failed, skip", "file", path, "err", err)
			continue
		}
		sk, ok := parseSkill(e.Name(), data)
		if !ok {
			s.logger.Warn("skill: parse failed, skip", "file", path)
			continue
		}
		found[sk.Name] = true
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO skills (name, version, description, instructions, content, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET version=excluded.version, description=excluded.description,
			   instructions=excluded.instructions, content=excluded.content, updated_at=excluded.updated_at`,
			sk.Name, sk.Version, sk.Description, sk.Instructions, sk.Content, now); err != nil {
			s.logger.Warn("skill: cache upsert failed", "name", sk.Name, "err", err)
			continue
		}
	}

	// 删除文件系统中已不存在的缓存行（索引同步）：逐行比对 found 集合。
	rows, err := conn.QueryContext(ctx, `SELECT name FROM skills`)
	if err != nil {
		return fmt.Errorf("skill: list cache: %w", err)
	}
	var stale []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("skill: scan cache: %w", err)
		}
		if !found[name] {
			stale = append(stale, name)
		}
	}
	_ = rows.Close()
	for _, name := range stale {
		if _, err := conn.ExecContext(ctx, `DELETE FROM skills WHERE name = ?`, name); err != nil {
			return fmt.Errorf("skill: delete stale %s: %w", name, err)
		}
	}
	return nil
}

// parseSkill 解析单个 Skill 文件；返回 (Skill, 是否有效)。
//
//   - .yaml/.yml：YAML 结构化（name 必填，缺失视为解析失败）；
//   - .md：首个 # 标题为 name（strip），全文为 Instructions/Content；无标题视为解析失败。
func parseSkill(filename string, data []byte) (Skill, bool) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".md" {
		text := string(data)
		title := firstMarkdownTitle(text)
		if title == "" {
			return Skill{}, false
		}
		return Skill{Name: title, Instructions: text, Content: text}, true
	}

	// YAML。
	var raw struct {
		Name         string `yaml:"name"`
		Version      string `yaml:"version"`
		Description  string `yaml:"description"`
		Instructions string `yaml:"instructions"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Skill{}, false
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		return Skill{}, false
	}
	return Skill{
		Name:         name,
		Version:      strings.TrimSpace(raw.Version),
		Description:  strings.TrimSpace(raw.Description),
		Instructions: strings.TrimSpace(raw.Instructions),
		Content:      string(data),
	}, true
}

// firstMarkdownTitle 提取 Markdown 首个 # 标题文本（去 # 与空白）。
func firstMarkdownTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}
