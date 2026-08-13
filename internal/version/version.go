// Package version 守护进程/CLI 版本信息。
//
// 版本号通过构建注入：-ldflags "-X tangoforge/internal/version.Version=<ver>"，
// 由 Makefile / CI 从 app/package.json 的 version 读取（与 APP 强一致，release.yml 校验）。
// 未注入时默认 "dev"（本地 make build / go build 场景）。
package version

// Version 守护进程构建版本（与 APP 版本一致）。
// 默认 dev：本地开发构建；发布构建由 ldflags 覆盖。
var Version = "dev"

// String 返回当前版本（非空兜底）。
func String() string {
	if Version == "" {
		return "dev"
	}
	return Version
}
