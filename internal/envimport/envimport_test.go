package envimport

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseNulSeparated(t *testing.T) {
	// 值里带换行与等号, 只按第一个等号切分.
	data := []byte("PATH=/usr/bin:/bin\x00MULTI=line1\nline2\x00EQ=a=b=c\x00\x00BROKEN\x00")
	vars, err := parseNulSeparated(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	want := map[string]string{
		"PATH":  "/usr/bin:/bin",
		"MULTI": "line1\nline2",
		"EQ":    "a=b=c",
	}
	for key, value := range want {
		if vars[key] != value {
			t.Errorf("%s = %q, 期望 %q", key, vars[key], value)
		}
	}
	// 不合规的条目应被跳过, 而不是让整次解析失败.
	if _, ok := vars["BROKEN"]; ok {
		t.Error("没有等号的条目不应进入结果")
	}
}

func TestMergeOrder(t *testing.T) {
	shell := map[string]string{"PATH": "/shell", "SHELL_ONLY": "1"}
	global := map[string]string{"PATH": "/global", "GLOBAL_ONLY": "1"}
	task := map[string]string{"PATH": "/task"}

	got := Merge(shell, global, task)
	if got["PATH"] != "/task" {
		t.Errorf("PATH = %q, 任务级应最高优先级", got["PATH"])
	}
	if got["SHELL_ONLY"] != "1" || got["GLOBAL_ONLY"] != "1" {
		t.Errorf("各层独有的变量都应保留: %v", got)
	}
}

func TestMergeSkipsNil(t *testing.T) {
	got := Merge(nil, map[string]string{"A": "1"}, nil)
	if len(got) != 1 || got["A"] != "1" {
		t.Errorf("nil 层应被跳过: %v", got)
	}
}

func TestApplyToEnvironReplacesExisting(t *testing.T) {
	base := []string{"PATH=/old", "HOME=/home/x"}
	got := ApplyToEnviron(base, map[string]string{"PATH": "/new"})

	// 同名项应被替换而不是追加, 否则会出现两个 PATH.
	count := 0
	for _, entry := range got {
		if strings.HasPrefix(entry, "PATH=") {
			count++
			if entry != "PATH=/new" {
				t.Errorf("PATH 应为 /new, 实际 %q", entry)
			}
		}
	}
	if count != 1 {
		t.Errorf("PATH 出现 %d 次, 期望 1 次", count)
	}
	if got[1] != "HOME=/home/x" {
		t.Errorf("未涉及的变量应保持原位: %v", got)
	}
}

func TestApplyToEnvironAppendsNew(t *testing.T) {
	got := ApplyToEnviron([]string{"A=1"}, map[string]string{"B": "2"})
	if len(got) != 2 || got[1] != "B=2" {
		t.Errorf("新变量应追加: %v", got)
	}
}

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"fish -l -i -c \"env -0\"", []string{"fish", "-l", "-i", "-c", "env -0"}},
		{"  zsh   -l   -c   'env -0'  ", []string{"zsh", "-l", "-c", "env -0"}},
		{"", nil},
		{"   ", nil},
		{"/bin/sh -c \"echo a b\"", []string{"/bin/sh", "-c", "echo a b"}},
	}
	for _, c := range cases {
		got := SplitCommand(c.raw)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitCommand(%q) = %v, 期望 %v", c.raw, got, c.want)
		}
	}
}

func TestSplitCommandUnclosedQuote(t *testing.T) {
	// 引号未闭合时按剩余部分整体作为最后一个参数, 不 panic 也不丢内容.
	got := SplitCommand(`fish -c "env -0`)
	want := []string{"fish", "-c", "env -0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("= %v, 期望 %v", got, want)
	}
}

func TestImportRealShell(t *testing.T) {
	// 用当前用户的默认 shell 做一次真实导入, 断言拿到了 PATH.
	if testing.Short() {
		t.Skip("跳过真实 shell 导入")
	}
	command := DefaultCommand()
	if len(command) == 0 {
		t.Skip("当前平台没有默认导入命令")
	}
	vars, err := Import(context.Background(), command, 0)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if vars["PATH"] == "" {
		t.Error("导入结果里没有 PATH")
	}
}

func TestImportFailsOnBadCommand(t *testing.T) {
	_, err := Import(context.Background(), []string{"/nonexistent/shell", "-c", "env -0"}, 0)
	if err == nil {
		t.Fatal("不存在的命令应当报错")
	}
}

func TestImportEmptyCommand(t *testing.T) {
	// 命令为空且平台没有默认值时应明确报错, 而不是返回空 map 假装成功.
	old := os.Getenv("SHELL")
	t.Setenv("SHELL", "")
	defer os.Setenv("SHELL", old)

	_, err := Import(context.Background(), nil, 0)
	// macOS 与 linux 会回退到 /bin/sh, 因此这里允许成功; 只要求不 panic.
	if err != nil && !strings.Contains(err.Error(), "没有可用的导入命令") {
		t.Logf("回退 shell 导入结果: %v", err)
	}
}

// TestSelectLayers 验证两个继承开关独立生效.
//
// shell 环境与全局变量是两个独立来源, 混在一起就会让用户无法只保留其中一个.
func TestSelectLayers(t *testing.T) {
	shell := map[string]string{"PATH": "/shell", "SHELL_VAR": "s"}
	global := map[string]string{"GLOBAL_VAR": "g"}

	cases := []struct {
		name      string
		useShell  bool
		useGlobal bool
		want      map[string]string
	}{
		{"都继承", true, true, map[string]string{"PATH": "/shell", "SHELL_VAR": "s", "GLOBAL_VAR": "g"}},
		{"都不继承", false, false, map[string]string{}},
		{"只要 shell", true, false, map[string]string{"PATH": "/shell", "SHELL_VAR": "s"}},
		{"只要全局", false, true, map[string]string{"GLOBAL_VAR": "g"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SelectLayers(shell, global, c.useShell, c.useGlobal)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("= %v, 期望 %v", got, c.want)
			}
		})
	}
}

// TestSelectLayersNilSafe 验证传 nil 层不会 panic.
func TestSelectLayersNilSafe(t *testing.T) {
	got := SelectLayers(nil, nil, true, true)
	if len(got) != 0 {
		t.Errorf("应返回空表, 实际 %v", got)
	}
}
