package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// markdownExts 目录扫描支持的文件扩展名。
var markdownExts = map[string]bool{".md": true, ".markdown": true}

// mergeFiles 读取多个 Markdown 文件并合并为一次解析内容。
//
//   - 文件路径相对 workdir 或绝对；按传入顺序（目录扫描时已按路径排序）读取；
//   - 每个文件前插入 `<!-- file: <绝对路径> -->` 注释头，便于 LLM 区分来源；
//   - sourceFile（覆盖单元标识）规则：单文件 → 该文件绝对路径；多文件 → 公共父目录。
func mergeFiles(workdir string, files []string) (content, sourceFile string, err error) {
	if len(files) == 0 {
		return "", "", fmt.Errorf("未匹配到任何 Markdown 文件")
	}
	abs := make([]string, 0, len(files))
	var b strings.Builder
	for _, f := range files {
		path := filepath.Clean(f)
		if !filepath.IsAbs(path) {
			path = filepath.Join(workdir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", "", fmt.Errorf("读取文件 %s: %v", path, err)
		}
		abs = append(abs, path)
		b.WriteString("<!-- file: " + path + " -->\n")
		b.Write(data)
		b.WriteString("\n\n")
	}
	if len(abs) == 1 {
		return b.String(), abs[0], nil
	}
	return b.String(), commonParentDir(abs), nil
}

// scanMarkdownFiles 递归扫描目录下全部 *.md / *.markdown 文件（路径字典序，稳定导入顺序）。
func scanMarkdownFiles(dir string) ([]string, error) {
	abs := filepath.Clean(dir)
	if !filepath.IsAbs(abs) {
		abs, _ = filepath.Abs(abs) // 调用方已确保绝对；防御性兜底
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("目录 %s 不可访问: %v", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s 不是目录", abs)
	}
	var out []string
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if markdownExts[strings.ToLower(filepath.Ext(path))] {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描目录 %s: %v", abs, err)
	}
	sort.Strings(out)
	return out, nil
}

// commonParentDir 计算路径列表的公共父目录（源文件分散在不同子目录时作为覆盖单元标识）。
// 兜底：无公共前缀时返回首个文件的父目录。
func commonParentDir(paths []string) string {
	if len(paths) == 1 {
		return filepath.Dir(paths[0])
	}
	split := func(p string) []string {
		clean := filepath.Clean(p)
		var parts []string
		for {
			parent := filepath.Dir(clean)
			if parent == clean {
				parts = append(parts, clean)
				break
			}
			parts = append(parts, filepath.Base(clean))
			clean = parent
		}
		// 反转成从根到叶。
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}
		return parts
	}
	first := split(paths[0])
	var common []string
	for i, part := range first {
		for _, p := range paths[1:] {
			parts := split(p)
			if i >= len(parts) || parts[i] != part {
				return filepath.Join(common...)
			}
		}
		common = append(common, part)
	}
	return filepath.Join(common...)
}
