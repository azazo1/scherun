package localapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/azazo1/scherun/internal/task"
)

// TestBuildTaskViewsIncludesRunnable 验证视图里带上了可执行判定.
//
// 列表接口与 SSE 曾各自拼一份视图, SSE 那份漏了 Runnable, 导致任务
// 明明在运行, 界面却显示 "程序不可执行". 抽成共享构造后仍需要断言,
// 防止字段被误删.
func TestBuildTaskViewsIncludesRunnable(t *testing.T) {
	server := newTestServer(t)

	// 造一个真实可执行文件.
	program := filepath.Join(t.TempDir(), "demo")
	if err := os.WriteFile(program, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("创建可执行文件失败: %v", err)
	}

	views := server.buildTaskViews([]task.Task{
		{ID: 1, Name: "可执行", Program: program},
		{ID: 2, Name: "不存在", Program: filepath.Join(t.TempDir(), "missing")},
	})
	if len(views) != 2 {
		t.Fatalf("视图数量应为 2, 实际 %d", len(views))
	}
	if !views[0].Runnable {
		t.Fatal("可执行文件的 Runnable 应为 true")
	}
	if views[1].Runnable {
		t.Fatal("不存在文件的 Runnable 应为 false")
	}
	// 没有调度状态的任务应回退为 idle, 而不是留空.
	if views[0].Status != "idle" {
		t.Fatalf("无状态任务的 Status 应为 idle, 实际 %q", views[0].Status)
	}
}
