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
