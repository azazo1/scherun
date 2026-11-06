package scheduler

import (
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/azazo1/scherun/internal/store"
	"github.com/azazo1/scherun/internal/task"
)

// newTestScheduler 创建使用临时数据库的调度器.
func newTestScheduler(t *testing.T) *Scheduler {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sched := New(db, logger)
	t.Cleanup(sched.Close)
	return sched
}

// shellCommand 返回一个在当前平台可用的简单命令.
//
// Windows 没有 sh, 用 ping 的等价物太麻烦, 因此该平台的用例直接跳过.
func shellCommand(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("该用例依赖 POSIX shell")
	}
	_ = script
	return "/bin/sh"
}

// TestDetectorSuspendsAfterRepeatedFailures 验证窗口期内连续失败会触发挂起.
func TestDetectorSuspendsAfterRepeatedFailures(t *testing.T) {
	det := &detector{}

	// 前几次失败不该挂起.
	for attempt := 1; attempt < suspensionFailures; attempt++ {
		if det.fail() {
			t.Fatalf("第 %d 次失败不应挂起", attempt)
		}
	}
	// 达到阈值后挂起.
	if !det.fail() {
		t.Fatal("达到失败阈值后应当挂起")
	}
	// 挂起后继续保持.
	if !det.fail() {
		t.Fatal("挂起状态应当保持")
	}
}

// TestDetectorResetsAfterWindow 验证窗口期过后计数重新开始.
func TestDetectorResetsAfterWindow(t *testing.T) {
	det := &detector{}
	det.windowStart = time.Now().Add(-2 * suspensionWindow)
	det.failures = suspensionFailures - 1

	// 窗口已过期, 本次失败应当重新开始计数, 不触发挂起.
	if det.fail() {
		t.Fatal("窗口过期后不应立即挂起")
	}
	if det.failures != 1 {
		t.Fatalf("失败计数应重置为 1, 实际 %d", det.failures)
	}
}

// TestDetectorReset 验证 reset 清除挂起.
func TestDetectorReset(t *testing.T) {
	det := &detector{}
	for det.fail() {
		// 一直失败直到挂起.
	}
	det.reset()
	if det.suspended || det.failures != 0 {
		t.Fatalf("reset 后应清除挂起状态: %+v", det)
	}
}

// TestManualRunExecutesProgram 验证手动运行会真正启动进程并记录退出码.
func TestManualRunExecutesProgram(t *testing.T) {
	sched := newTestScheduler(t)
	if err := sched.Start(); err != nil {
		t.Fatalf("启动调度器失败: %v", err)
	}

	sh := shellCommand(t, "")
	id, err := sched.SaveTask(task.Task{
		Name:    "立即退出",
		Program: sh,
		Args:    []string{"-c", "exit 0"},
		Trigger: task.Trigger{Kind: task.TriggerManual},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	if err := sched.RunTask(id); err != nil {
		t.Fatalf("手动运行失败: %v", err)
	}

	// 等待进程退出并写回退出码.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap := sched.Snapshot(id)
		if snap.Status == StatusIdle && snap.LastExit != nil {
			if *snap.LastExit != 0 {
				t.Fatalf("退出码应为 0, 实际 %d", *snap.LastExit)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("任务未在预期时间内结束: %+v", sched.Snapshot(id))
}

// TestStopTaskTerminatesProcess 验证停止任务会终止进程.
func TestStopTaskTerminatesProcess(t *testing.T) {
	sched := newTestScheduler(t)
	if err := sched.Start(); err != nil {
		t.Fatalf("启动调度器失败: %v", err)
	}

	sh := shellCommand(t, "")
	id, err := sched.SaveTask(task.Task{
		Name:    "长时间运行",
		Program: sh,
		Args:    []string{"-c", "sleep 60"},
		Trigger: task.Trigger{Kind: task.TriggerManual},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	if err := sched.RunTask(id); err != nil {
		t.Fatalf("手动运行失败: %v", err)
	}

	// 等到确实在运行.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sched.Snapshot(id).Status == StatusRunning {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sched.Snapshot(id).Status != StatusRunning {
		t.Fatal("任务未进入运行状态")
	}

	if err := sched.StopTask(id); err != nil {
		t.Fatalf("停止任务失败: %v", err)
	}

	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if sched.Snapshot(id).Status != StatusRunning {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("任务在停止后仍在运行")
}

// TestDisabledTaskRefusesRun 验证停用的任务拒绝手动运行.
func TestDisabledTaskRefusesRun(t *testing.T) {
	sched := newTestScheduler(t)
	if err := sched.Start(); err != nil {
		t.Fatalf("启动调度器失败: %v", err)
	}

	sh := shellCommand(t, "")
	id, err := sched.SaveTask(task.Task{
		Name:    "已停用",
		Program: sh,
		Args:    []string{"-c", "exit 0"},
		Trigger: task.Trigger{Kind: task.TriggerManual},
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	if err := sched.RunTask(id); err == nil {
		t.Fatal("停用的任务不应被启动")
	}
}

// TestDeleteTaskRemovesState 验证删除任务会清掉调度状态.
func TestDeleteTaskRemovesState(t *testing.T) {
	sched := newTestScheduler(t)
	if err := sched.Start(); err != nil {
		t.Fatalf("启动调度器失败: %v", err)
	}

	sh := shellCommand(t, "")
	id, err := sched.SaveTask(task.Task{
		Name:    "将被删除",
		Program: sh,
		Args:    []string{"-c", "exit 0"},
		Trigger: task.Trigger{Kind: task.TriggerManual},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if err := sched.DeleteTask(id); err != nil {
		t.Fatalf("删除任务失败: %v", err)
	}

	snapshots := sched.Snapshots()
	if _, ok := snapshots[id]; ok {
		t.Fatal("删除后不应残留调度状态")
	}
}

// TestWarningsAreDrained 验证告警取走后就清空.
func TestWarningsAreDrained(t *testing.T) {
	sched := newTestScheduler(t)
	sched.warn(1, "示例", "测试告警")

	warnings := sched.TakeWarnings()
	if len(warnings) != 1 || warnings[0].Message != "测试告警" {
		t.Fatalf("应取到一条告警, 实际 %+v", warnings)
	}
	if again := sched.TakeWarnings(); len(again) != 0 {
		t.Fatalf("告警应被取空, 实际 %+v", again)
	}
}
