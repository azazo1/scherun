//go:build !windows

package proc

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRedirectRelativePathNoParent 验证相对路径按任务工作目录解析.
//
// 这是一个容易写错且很难察觉的地方: 按 schrun 自身的 cwd 解析时,
// 文件会悄悄落到应用进程的目录里, 而不是任务的工作目录.
func TestRedirectRelativePathNoParent(t *testing.T) {
	dir := t.TempDir()

	process, err := Start(Spec{
		Program:    "/bin/sh",
		Args:       []string{"-c", "echo hi"},
		WorkingDir: dir,
		Stdout:     "out.log",
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if code := process.Wait(); code != 0 {
		t.Fatalf("退出码应为 0, 实际 %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.log")); err != nil {
		t.Fatalf("相对路径的输出文件应写在工作目录下: %v", err)
	}
}

// TestStdinMissingFails 验证输入文件不存在时报错, 而不是凭空创建.
func TestStdinMissingFails(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope", "input.txt")

	if _, err := Start(Spec{
		Program: "/bin/sh",
		Args:    []string{"-c", "cat"},
		Stdin:   missing,
	}); err == nil {
		t.Fatal("输入文件不存在时应当报错")
	}
	// 输入路径的父目录不应被创建.
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Fatal("读取输入文件时不应创建其父目录")
	}
}

// TestStartResolvesProgramFromBaseEnv 验证只写程序名时按注入的 PATH 解析.
//
// 这是从 Finder 启动的核心场景: 应用自身 PATH 只有系统默认值,
// 程序要能通过登录 shell 导入的 PATH 找到, 否则配置了环境也照样启动失败.
func TestStartResolvesProgramFromBaseEnv(t *testing.T) {
	dir := t.TempDir()
	// 在临时目录里放一个可执行脚本, 它不在系统默认 PATH 里.
	script := filepath.Join(dir, "my-tool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatalf("写脚本失败: %v", err)
	}

	// 确保进程自身 PATH 里没有它.
	t.Setenv("PATH", "/usr/bin:/bin")

	// 不注入时应当失败, 证明这个场景确实依赖注入.
	if _, err := Start(Spec{Program: "my-tool"}); err == nil {
		t.Fatal("没有注入 PATH 时不应启动成功")
	}

	process, err := Start(Spec{Program: "my-tool"}.WithBaseEnv(map[string]string{
		"PATH": dir + ":/usr/bin:/bin",
	}))
	if err != nil {
		t.Fatalf("注入 PATH 后应能启动: %v", err)
	}
	if code := process.Wait(); code != 7 {
		t.Fatalf("退出码应为脚本里的 7, 实际 %d", code)
	}
}

// TestTaskEnvOverridesBaseEnv 验证任务级变量覆盖全局层.
func TestTaskEnvOverridesBaseEnv(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "value.txt")

	process, err := Start(Spec{
		Program: "/bin/sh",
		Args:    []string{"-c", "printf %s \"$GREETING\" > " + out},
		BaseEnv: map[string]string{"GREETING": "from-global"},
		EnvVars: map[string]string{"GREETING": "from-task"},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if code := process.Wait(); code != 0 {
		t.Fatalf("退出码应为 0, 实际 %d", code)
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("读取输出失败: %v", err)
	}
	if string(content) != "from-task" {
		t.Errorf("应为任务级的值, 实际 %q", string(content))
	}
}

// TestRunnableWithEnvUsesInjectedPath 验证可执行判定与启动行为一致.
func TestRunnableWithEnvUsesInjectedPath(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "another-tool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("写脚本失败: %v", err)
	}
	t.Setenv("PATH", "/usr/bin:/bin")

	if Runnable("another-tool") {
		t.Error("进程 PATH 里没有它, 不应判为可执行")
	}
	if !RunnableWithEnv("another-tool", map[string]string{"PATH": dir + ":/usr/bin:/bin"}) {
		t.Error("注入 PATH 后应判为可执行, 否则界面会误报不可执行")
	}
}
