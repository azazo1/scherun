package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/azazo1/scherun/internal/task"
)

// newTestDB 创建一个临时数据库.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestMigrateIdempotent 验证重复打开同一数据库不会重复执行迁移.
func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	for attempt := 0; attempt < 2; attempt++ {
		db, err := Open(path)
		if err != nil {
			t.Fatalf("第 %d 次打开数据库失败: %v", attempt+1, err)
		}
		var count int
		if err := db.conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
			db.Close()
			t.Fatalf("统计迁移记录失败: %v", err)
		}
		db.Close()
		if count != 1 {
			t.Fatalf("迁移记录应为 1 条, 实际 %d 条", count)
		}
	}
}

// TestSaveAndGetTask 验证创建后能按 id 读回, 且字段完整.
func TestSaveAndGetTask(t *testing.T) {
	db := newTestDB(t)

	runAt := time.Now().Add(time.Hour).Truncate(time.Second)
	id, err := db.SaveTask(task.Task{
		Name:    "备份",
		Program: "/usr/bin/rsync",
		Args:    []string{"-a", "src", "dst"},
		EnvVars: map[string]string{"MODE": "fast"},
		Trigger: task.Trigger{Kind: task.TriggerRoutine, IntervalSeconds: 300},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	got, err := db.GetTask(id)
	if err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if got.Name != "备份" || got.Program != "/usr/bin/rsync" {
		t.Fatalf("基础字段不正确: %+v", got)
	}
	if len(got.Args) != 3 || got.Args[0] != "-a" {
		t.Fatalf("参数未正确往返: %v", got.Args)
	}
	if got.EnvVars["MODE"] != "fast" {
		t.Fatalf("环境变量未正确往返: %v", got.EnvVars)
	}
	if got.Trigger.Kind != task.TriggerRoutine || got.Trigger.IntervalSeconds != 300 {
		t.Fatalf("触发方式未正确往返: %+v", got.Trigger)
	}

	// Instant 的时间点也要能精确往返.
	id2, err := db.SaveTask(task.Task{
		Name:    "一次性",
		Program: "/bin/echo",
		Trigger: task.Trigger{Kind: task.TriggerInstant, RunAt: &runAt},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建一次性任务失败: %v", err)
	}
	got2, err := db.GetTask(id2)
	if err != nil {
		t.Fatalf("读取一次性任务失败: %v", err)
	}
	if got2.Trigger.RunAt == nil || !got2.Trigger.RunAt.Equal(runAt) {
		t.Fatalf("定时时间未正确往返: %v", got2.Trigger.RunAt)
	}
}

// TestSaveTaskRejectsInvalid 验证非法任务被拒绝.
func TestSaveTaskRejectsInvalid(t *testing.T) {
	db := newTestDB(t)

	cases := []struct {
		name string
		task task.Task
	}{
		{"缺少名称", task.Task{Program: "/bin/echo", Trigger: task.Trigger{Kind: task.TriggerManual}}},
		{"缺少程序", task.Task{Name: "x", Trigger: task.Trigger{Kind: task.TriggerManual}}},
		{"定时重复间隔为 0", task.Task{Name: "x", Program: "/bin/echo", Trigger: task.Trigger{Kind: task.TriggerRoutine}}},
		{"定时一次缺少时间", task.Task{Name: "x", Program: "/bin/echo", Trigger: task.Trigger{Kind: task.TriggerInstant}}},
	}
	for _, testCase := range cases {
		if _, err := db.SaveTask(testCase.task); err == nil {
			t.Fatalf("%s 应当被拒绝", testCase.name)
		}
	}
}

// TestUpdateTaskKeepsSortOrder 验证更新不改变排序位置.
func TestUpdateTaskKeepsSortOrder(t *testing.T) {
	db := newTestDB(t)

	id, err := db.SaveTask(task.Task{Name: "a", Program: "/bin/echo", Trigger: task.Trigger{Kind: task.TriggerManual}, Enabled: true})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	before, _ := db.GetTask(id)

	if _, err := db.SaveTask(task.Task{
		ID: id, Name: "a 改名", Program: "/bin/echo", Trigger: task.Trigger{Kind: task.TriggerManual},
	}); err != nil {
		t.Fatalf("更新任务失败: %v", err)
	}

	after, _ := db.GetTask(id)
	if after.Name != "a 改名" {
		t.Fatalf("名称未更新: %s", after.Name)
	}
	if after.SortOrder != before.SortOrder {
		t.Fatalf("更新不应改变排序: %d -> %d", before.SortOrder, after.SortOrder)
	}
}

// TestDeleteMissingTask 验证删除不存在的任务返回特定错误.
func TestDeleteMissingTask(t *testing.T) {
	db := newTestDB(t)
	if err := db.DeleteTask(9999); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("应返回 ErrTaskNotFound, 实际 %v", err)
	}
}

// TestReorderTasks 验证重排后列表顺序符合给定 id 顺序.
func TestReorderTasks(t *testing.T) {
	db := newTestDB(t)

	var ids []int64
	for _, name := range []string{"一", "二", "三"} {
		id, err := db.SaveTask(task.Task{
			Name: name, Program: "/bin/echo", Trigger: task.Trigger{Kind: task.TriggerManual}, Enabled: true,
		})
		if err != nil {
			t.Fatalf("创建任务失败: %v", err)
		}
		ids = append(ids, id)
	}

	// 反转顺序.
	reversed := []int64{ids[2], ids[0], ids[1]}
	if err := db.ReorderTasks(reversed); err != nil {
		t.Fatalf("重排失败: %v", err)
	}

	list, err := db.ListTasks()
	if err != nil {
		t.Fatalf("读取列表失败: %v", err)
	}
	for index, wantID := range reversed {
		if list[index].ID != wantID {
			t.Fatalf("第 %d 个任务应为 %d, 实际 %d", index, wantID, list[index].ID)
		}
	}
}

// TestSetLastRunAndEnabled 验证运行状态与启用状态的更新.
func TestSetLastRunAndEnabled(t *testing.T) {
	db := newTestDB(t)

	id, err := db.SaveTask(task.Task{Name: "x", Program: "/bin/echo", Trigger: task.Trigger{Kind: task.TriggerManual}, Enabled: true})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	code := 3
	if err := db.SetLastRun(id, now, &code); err != nil {
		t.Fatalf("更新运行状态失败: %v", err)
	}
	got, _ := db.GetTask(id)
	if got.LastExitCode == nil || *got.LastExitCode != 3 {
		t.Fatalf("退出码未记录: %v", got.LastExitCode)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(now) {
		t.Fatalf("运行时间未记录: %v", got.LastRunAt)
	}

	if err := db.SetEnabled(id, false); err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	got, _ = db.GetTask(id)
	if got.Enabled {
		t.Fatal("任务应处于停用状态")
	}
}
