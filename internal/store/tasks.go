package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/azazo1/scherun/internal/task"
)

// ErrTaskNotFound 表示指定 id 的任务不存在.
var ErrTaskNotFound = errors.New("任务不存在")

// taskColumns 是任务查询使用的列顺序, 与 scanTask 一一对应.
const taskColumns = `id, sort_order, name, program, args, stdin, stdout, stderr,
	working_dir, env_vars, trigger_tag, trigger_value, enabled, no_console,
	use_shell_env, use_global_env, last_exit_code, last_run_at, created_at, updated_at`

// ListTasks 返回全部任务, 按 sort_order 与 id 升序.
func (d *DB) ListTasks() ([]task.Task, error) {
	rows, err := d.conn.Query(
		`SELECT ` + taskColumns + ` FROM tasks ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询任务列表失败: %w", err)
	}
	defer rows.Close()

	var tasks []task.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历任务列表失败: %w", err)
	}
	return tasks, nil
}

// GetTask 按 id 查询任务.
func (d *DB) GetTask(id int64) (task.Task, error) {
	row := d.conn.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, ErrTaskNotFound
	}
	return t, err
}

// SaveTask 新建或更新任务, 返回任务 id.
//
// 新任务的 sort_order 取当前最大值加一, 保证追加在列表末尾.
func (d *DB) SaveTask(t task.Task) (int64, error) {
	if err := t.Validate(); err != nil {
		return 0, err
	}
	now := time.Now()
	triggerValue := encodeTriggerValue(t.Trigger)

	if t.ID == 0 {
		var nextOrder int64
		if err := d.conn.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM tasks`).Scan(&nextOrder); err != nil {
			return 0, fmt.Errorf("计算任务排序位置失败: %w", err)
		}
		res, err := d.conn.Exec(`INSERT INTO tasks
			(sort_order, name, program, args, stdin, stdout, stderr, working_dir, env_vars,
			 trigger_tag, trigger_value, enabled, no_console, use_shell_env, use_global_env,
			 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nextOrder, t.Name, t.Program, task.EncodeArgs(t.Args),
			nullString(t.Stdin), nullString(t.Stdout), nullString(t.Stderr),
			nullString(t.WorkingDir), task.EncodeEnv(t.EnvVars),
			string(t.Trigger.Kind), triggerValue, boolToInt(t.Enabled), boolToInt(t.NoConsole),
			boolToInt(t.UseShellEnv), boolToInt(t.UseGlobalEnv),
			formatTime(now), formatTime(now),
		)
		if err != nil {
			return 0, fmt.Errorf("创建任务失败: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("获取新任务 id 失败: %w", err)
		}
		return id, nil
	}

	// 更新时不动 sort_order, 排序由单独的 reorder 负责.
	res, err := d.conn.Exec(`UPDATE tasks SET
		name = ?, program = ?, args = ?, stdin = ?, stdout = ?, stderr = ?,
		working_dir = ?, env_vars = ?, trigger_tag = ?, trigger_value = ?,
		no_console = ?, use_shell_env = ?, use_global_env = ?, updated_at = ?
		WHERE id = ?`,
		t.Name, t.Program, task.EncodeArgs(t.Args),
		nullString(t.Stdin), nullString(t.Stdout), nullString(t.Stderr),
		nullString(t.WorkingDir), task.EncodeEnv(t.EnvVars),
		string(t.Trigger.Kind), triggerValue, boolToInt(t.NoConsole),
		boolToInt(t.UseShellEnv), boolToInt(t.UseGlobalEnv),
		formatTime(now), t.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("更新任务 %d 失败: %w", t.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("确认任务 %d 更新结果失败: %w", t.ID, err)
	}
	if affected == 0 {
		return 0, ErrTaskNotFound
	}
	return t.ID, nil
}

// DeleteTask 删除任务, 任务不存在时返回 ErrTaskNotFound.
func (d *DB) DeleteTask(id int64) error {
	res, err := d.conn.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除任务 %d 失败: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("确认任务 %d 删除结果失败: %w", id, err)
	}
	if affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// SetEnabled 启用或停用任务.
func (d *DB) SetEnabled(id int64, enabled bool) error {
	res, err := d.conn.Exec(`UPDATE tasks SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), formatTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("切换任务 %d 启用状态失败: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("确认任务 %d 状态失败: %w", id, err)
	}
	if affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// SetLastRun 记录任务最近一次运行时间与退出码, 退出码为 nil 表示尚无结果.
func (d *DB) SetLastRun(id int64, runAt time.Time, exitCode *int) error {
	var code any
	if exitCode != nil {
		code = *exitCode
	}
	if _, err := d.conn.Exec(
		`UPDATE tasks SET last_run_at = ?, last_exit_code = ?, updated_at = ? WHERE id = ?`,
		formatTime(runAt), code, formatTime(time.Now()), id,
	); err != nil {
		return fmt.Errorf("更新任务 %d 运行状态失败: %w", id, err)
	}
	return nil
}

// SetLastExitCode 单独更新退出码.
func (d *DB) SetLastExitCode(id int64, exitCode int) error {
	if _, err := d.conn.Exec(
		`UPDATE tasks SET last_exit_code = ?, updated_at = ? WHERE id = ?`,
		exitCode, formatTime(time.Now()), id,
	); err != nil {
		return fmt.Errorf("更新任务 %d 退出码失败: %w", id, err)
	}
	return nil
}

// ReorderTasks 按给定 id 顺序重排任务, 第一个 id 排最前.
func (d *DB) ReorderTasks(orderedIDs []int64) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("开启重排事务失败: %w", err)
	}
	defer tx.Rollback()

	now := formatTime(time.Now())
	for index, id := range orderedIDs {
		if _, err := tx.Exec(
			`UPDATE tasks SET sort_order = ?, updated_at = ? WHERE id = ?`,
			index, now, id,
		); err != nil {
			return fmt.Errorf("更新任务 %d 排序失败: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交重排事务失败: %w", err)
	}
	return nil
}

// rowScanner 抽象 *sql.Row 与 *sql.Rows 的扫描能力.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanTask 把一行记录扫描成任务.
func scanTask(s rowScanner) (task.Task, error) {
	var (
		t            task.Task
		argsRaw      string
		envRaw       string
		stdin        sql.NullString
		stdout       sql.NullString
		stderr       sql.NullString
		workingDir   sql.NullString
		triggerTag   string
		triggerValue sql.NullString
		enabled      int
		noConsole    int
		useShellEnv  int
		useGlobalEnv int
		lastExitCode sql.NullInt64
		lastRunAt    sql.NullString
		createdAt    string
		updatedAt    string
	)
	err := s.Scan(
		&t.ID, &t.SortOrder, &t.Name, &t.Program, &argsRaw,
		&stdin, &stdout, &stderr, &workingDir, &envRaw,
		&triggerTag, &triggerValue, &enabled, &noConsole,
		&useShellEnv, &useGlobalEnv,
		&lastExitCode, &lastRunAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return task.Task{}, err
	}

	t.Args = task.DecodeArgs(argsRaw)
	t.EnvVars = task.DecodeEnv(envRaw)
	t.Stdin = stdin.String
	t.Stdout = stdout.String
	t.Stderr = stderr.String
	t.WorkingDir = workingDir.String
	t.Enabled = enabled != 0
	t.NoConsole = noConsole != 0
	t.UseShellEnv = useShellEnv != 0
	t.UseGlobalEnv = useGlobalEnv != 0
	t.Trigger = decodeTrigger(triggerTag, triggerValue)

	if lastExitCode.Valid {
		code := int(lastExitCode.Int64)
		t.LastExitCode = &code
	}
	if lastRunAt.Valid {
		if parsed, err := parseTime(lastRunAt.String); err == nil {
			t.LastRunAt = &parsed
		}
	}
	t.CreatedAt, _ = parseTime(createdAt)
	t.UpdatedAt, _ = parseTime(updatedAt)
	return t, nil
}

// timeLayout 是数据库里时间字段的存储格式.
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(timeLayout, raw)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
