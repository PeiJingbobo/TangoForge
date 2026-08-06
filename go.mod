// Module 路径为占位默认值，首次 push 到远程仓库前请替换为实际仓库地址。
// go 指令由 1.22 提升至 1.25.0：modernc.org/sqlite v1.56.0 要求 Go >= 1.25（见 TF-001 任务总结）。
module tangoforge

go 1.25.0

require (
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-chi/chi/v5 v5.3.1
	github.com/google/uuid v1.6.0 // TF-005：任务 ID UUID v4 生成（纯 Go，无 CGO）
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.56.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
