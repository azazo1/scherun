package pathutil

import (
	"path/filepath"
	"testing"
)

// TestExpandHome 验证开头的 ~ 被展开成家目录.
func TestExpandHome(t *testing.T) {
	// 用临时目录冒充家目录, 避免触碰真实用户目录.
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"单独的家目录", "~", home},
		{"家目录下的路径", "~/logs/app.log", filepath.Join(home, "logs", "app.log")},
		{"绝对路径不变", "/var/log/app.log", "/var/log/app.log"},
		{"相对路径不变", "logs/app.log", "logs/app.log"},
		{"空串不变", "", ""},
		// 中间出现 ~ 是合法文件名字符, 不能展开.
		{"路径中间的波浪号不展开", "/tmp/a~b/c", "/tmp/a~b/c"},
		{"以波浪号开头的普通文件名不展开", "~backup/file", "~backup/file"},
	}
	for _, testCase := range cases {
		if got := ExpandHome(testCase.in); got != testCase.want {
			t.Fatalf("%s: ExpandHome(%q) = %q, 期望 %q", testCase.name, testCase.in, got, testCase.want)
		}
	}
}

// TestResolve 验证先展开 ~ 再按基准目录拼接.
func TestResolve(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := Resolve("/base", "~/x"); got != filepath.Join(home, "x") {
		t.Fatalf("~ 应优先于基准目录展开, 实际 %q", got)
	}
	if got := Resolve("/base", "y"); got != filepath.Join("/base", "y") {
		t.Fatalf("相对路径应按基准目录拼接, 实际 %q", got)
	}
	if got := Resolve("/base", "/abs"); got != "/abs" {
		t.Fatalf("绝对路径应原样返回, 实际 %q", got)
	}
	if got := Resolve("", "y"); got != "y" {
		t.Fatalf("基准为空时应保持相对路径, 实际 %q", got)
	}
}

// TestExpandHomeWithoutHome 验证取不到家目录时原样返回, 不 panic.
func TestExpandHomeWithoutHome(t *testing.T) {
	// 清空 HOME, 让 UserHomeDir 走回退逻辑或失败.
	t.Setenv("HOME", "")
	// 无论能否取到家目录, 都不应 panic, 且返回值非空.
	if got := ExpandHome("~/x"); got == "" {
		t.Fatal("取不到家目录时也应返回原始路径")
	}
}
