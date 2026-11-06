// Package version 保存构建期注入的版本信息.
//
// 发布构建通过 -ldflags "-X ..." 覆盖这几个变量, 日常构建保持默认值.
package version

import (
	"runtime/debug"
	"strings"
)

// 构建期注入点, 不要改动变量名, 发布脚本依赖它.
var (
	Version   = "dev-build"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info 返回版本, 提交与构建时间.
//
// 未注入版本时退化为从构建信息里读 vcs.revision, 保证开发构建也能看出对应提交.
func Info() (string, string, string) {
	commit := Commit
	if commit == "unknown" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range bi.Settings {
				if setting.Key == "vcs.revision" && setting.Value != "" {
					commit = setting.Value
					if len(commit) > 7 {
						commit = commit[:7]
					}
					break
				}
			}
		}
	}
	return Version, commit, BuildTime
}

// Display 返回给用户看的版本字符串, 例如 v0.1.0 或 dev-build.
func Display() string {
	v, commit, _ := Info()
	if v == "" {
		v = "dev-build"
	}
	if v == "dev-build" && commit != "unknown" {
		return v + "-" + commit
	}
	return v
}

// IsDev 判断是否为开发构建.
func IsDev() bool {
	v, _, _ := Info()
	return v == "" || strings.HasPrefix(v, "dev-build")
}
