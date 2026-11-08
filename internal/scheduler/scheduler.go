// Package scheduler 按触发方式调度后台任务, 并维护运行状态.
//
// 设计要点:
//   - 每个任务有独立的 guard 协程, 负责该任务的触发与进程生命周期.
//   - 进程退出通过 channel 回报给 guard 协程, 由它决定是否重启.
//   - 运行状态与告警保存在带锁的结构里, 供 HTTP 层读取快照.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/azazo1/scherun/internal/envimport"
	"github.com/azazo1/scherun/internal/proc"
	"github.com/azazo1/scherun/internal/store"
	"github.com/azazo1/scherun/internal/task"
)

// Status 是任务当前的运行状态.
type Status string

// 运行状态取值.
const (
	// StatusIdle 空闲, 可以触发.
	StatusIdle Status = "idle"
	// StatusRunning 正在运行.
	StatusRunning Status = "running"
	// StatusSuspended 因连续失败被暂停自动触发.
	StatusSuspended Status = "suspended"
	// StatusPaused 用户主动暂停了自动触发.
	//
	// 与 StatusDisabled 的区别: 停用是任务级配置, 会持久化, 重启应用后依然生效;
	// 暂停只是本次运行的临时意愿, 点运行即可恢复, 不写回任务定义.
	StatusPaused Status = "paused"
	// StatusDisabled 任务已停用.
	StatusDisabled Status = "disabled"
)

// autoRestartable 判断触发方式是否会自动拉起进程.
//
// 这类任务被用户停止后必须记住 "别再自动拉起", 否则停止按钮对它毫无作用:
// 进程一退出就被调度器重新启动, 界面闪一下又回到运行中.
func autoRestartable(kind task.TriggerKind) bool {
	switch kind {
	case task.TriggerKeepAlive, task.TriggerRoutine, task.TriggerUntilSucceed:
		return true
	default:
		return false
	}
}

// Warning 是一条运行时告警.
type Warning struct {
	TaskID    int64     `json:"task_id,omitempty"`
	TaskName  string    `json:"task_name,omitempty"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Snapshot 是任务的运行状态快照.
type Snapshot struct {
	TaskID    int64      `json:"task_id"`
	Status    Status     `json:"status"`
	PID       int        `json:"pid,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	LastExit  *int       `json:"last_exit_code,omitempty"`
	Restarts  int        `json:"restarts"`
	Failures  int        `json:"failures"`
}

// 挂起参数: 在窗口期内累计失败达到阈值就暂停该任务的自动触发.
const (
	suspensionWindow   = 3 * time.Second
	suspensionFailures = 6
	// maxWarnings 限制告警队列长度, 防止长时间运行后无限增长.
	maxWarnings = 200
)

// detector 是挂起检测器.
type detector struct {
	windowStart time.Time
	failures    int
	suspended   bool
}

// fail 记录一次失败并判断是否应当挂起.
func (d *detector) fail() bool {
	if d.suspended {
		return true
	}
	now := time.Now()
	if d.windowStart.IsZero() || now.Sub(d.windowStart) > suspensionWindow {
		d.windowStart = now
		d.failures = 0
	}
	d.failures++
	if d.failures >= suspensionFailures {
		d.suspended = true
		return true
	}
	return false
}

// reset 清除挂起状态.
func (d *detector) reset() {
	d.windowStart = time.Time{}
	d.failures = 0
	d.suspended = false
}

// taskState 保存单个任务的运行状态.
type taskState struct {
	mu        sync.Mutex
	status    Status
	pid       int
	startedAt *time.Time
	lastRunAt *time.Time
	lastExit  *int
	restarts  int
	failures  int
}

// snapshot 生成状态快照.
func (st *taskState) snapshot(id int64) Snapshot {
	st.mu.Lock()
	defer st.mu.Unlock()
	return Snapshot{
		TaskID:    id,
		Status:    st.status,
		PID:       st.pid,
		StartedAt: st.startedAt,
		LastRunAt: st.lastRunAt,
		LastExit:  st.lastExit,
		Restarts:  st.restarts,
		Failures:  st.failures,
	}
}

// Scheduler 调度全部任务.
type Scheduler struct {
	db     *store.DB
	logger *slog.Logger

	// env 提供任务进程的基础环境变量, 可为 nil.
	//
	// 分两层返回: shellEnv 来自登录 shell 导入, globalEnv 来自设置里的全局注入.
	// 任务按自己的 UseShellEnv / UseGlobalEnv 开关决定要哪一层, 两者独立.
	env func() (shellEnv, globalEnv map[string]string)

	mu       sync.Mutex
	states   map[int64]*taskState
	warnings []Warning

	guards   map[int64]*guard
	guardMu  sync.Mutex
	runCtx   context.Context
	cancel   context.CancelFunc
	stopped  chan struct{}
	stopOnce sync.Once
}

// guard 是任务的调度协程句柄.
type guard struct {
	cancel  context.CancelFunc
	done    chan struct{}
	control chan controlMsg
}

// controlMsg 是发给 guard 协程的即时指令.
type controlMsg struct {
	kind  controlKind
	reply chan error
}

// controlKind 是指令类型.
type controlKind int

const (
	// controlRun 手动运行一次.
	controlRun controlKind = iota
	// controlStop 停止当前进程.
	controlStop
)

// New 创建调度器, 不启动任务.
func New(db *store.DB, logger *slog.Logger) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		db:      db,
		logger:  logger,
		states:  map[int64]*taskState{},
		guards:  map[int64]*guard{},
		runCtx:  ctx,
		cancel:  cancel,
		stopped: make(chan struct{}),
	}
}

// SetEnvProvider 设置任务进程的环境变量来源.
//
// 传入的函数会在每次启动任务时调用, 因此可以在设置变更后立刻生效.
// 两个返回值分别是登录 shell 导入的环境与设置里的全局变量, 由任务按自己的开关选择.
// 传 nil 表示不注入额外变量, 任务继承应用自身环境.
func (s *Scheduler) SetEnvProvider(provider func() (shellEnv, globalEnv map[string]string)) {
	s.env = provider
}

// envForTask 返回某个任务本次启动要注入的环境变量.
//
// 任务级 env_vars 不在这里合并, 由 proc 层按最高优先级覆盖.
func (s *Scheduler) envForTask(t task.Task) map[string]string {
	if s.env == nil {
		return nil
	}
	shellEnv, globalEnv := s.env()
	return envimport.SelectLayers(shellEnv, globalEnv, t.UseShellEnv, t.UseGlobalEnv)
}

// Start 加载全部任务并启动调度.
func (s *Scheduler) Start() error {
	tasks, err := s.db.ListTasks()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		s.mu.Lock()
		status := StatusIdle
		if !t.Enabled {
			status = StatusDisabled
		}
		s.states[t.ID] = &taskState{status: status}
		s.mu.Unlock()
		s.spawnGuard(t)
	}
	s.logger.Info("调度器已启动", "tasks", len(tasks))
	return nil
}

// spawnGuard 为任务启动调度协程, 已有协程时先停掉旧的.
func (s *Scheduler) spawnGuard(t task.Task) {
	s.stopGuard(t.ID)

	ctx, cancel := context.WithCancel(s.runCtx)
	g := &guard{
		cancel:  cancel,
		done:    make(chan struct{}),
		control: make(chan controlMsg, 4),
	}

	s.guardMu.Lock()
	s.guards[t.ID] = g
	s.guardMu.Unlock()

	go func() {
		defer close(g.done)
		s.runTask(ctx, t, g)
	}()
}

// stopGuard 停止任务的调度协程并等待退出.
func (s *Scheduler) stopGuard(id int64) {
	s.guardMu.Lock()
	g, ok := s.guards[id]
	if ok {
		delete(s.guards, id)
	}
	s.guardMu.Unlock()
	if !ok {
		return
	}
	g.cancel()
	select {
	case <-g.done:
	case <-time.After(proc.StopTimeout + 2*time.Second):
		s.logger.Warn("任务调度协程未按时退出", "task_id", id)
	}
}

// processExited 携带子进程退出码.
type processExited struct {
	code int
}

// runTask 是单个任务的调度循环.
func (s *Scheduler) runTask(ctx context.Context, initial task.Task, g *guard) {
	id := initial.ID
	state := s.state(id)
	det := &detector{}

	var (
		current *proc.Process
		exitCh  = make(chan processExited, 1)
		// instantDone 表示定时一次已经执行过.
		instantDone = false
		// pausedByUser 表示用户主动暂停了自动触发.
		//
		// 只有 guard 协程读写, 不需要加锁. 自动重启类任务被停止后必须记住这件事:
		// 否则进程一退出就被重新拉起, 停止按钮形同虚设.
		pausedByUser = false
	)

	// reload 读取任务最新定义, 任务已被删除时返回 false.
	reload := func() (task.Task, bool) {
		latest, err := s.db.GetTask(id)
		if err != nil {
			if !errors.Is(err, store.ErrTaskNotFound) {
				s.warn(id, initial.Name, fmt.Sprintf("读取任务失败: %v", err))
			}
			return task.Task{}, false
		}
		return latest, true
	}

	// launch 启动任务进程; 已在运行时返回错误.
	launch := func(triggeredBy string) error {
		if current != nil {
			return fmt.Errorf("任务正在运行")
		}
		latest, ok := reload()
		if !ok {
			return fmt.Errorf("任务不存在")
		}
		if !latest.Enabled {
			return fmt.Errorf("任务已停用")
		}
		process, err := proc.Start(proc.SpecFromTask(latest).WithBaseEnv(s.envForTask(latest)))
		if err != nil {
			state.mu.Lock()
			state.failures++
			state.mu.Unlock()
			s.warn(latest.ID, latest.Name, fmt.Sprintf("启动失败: %v", err))
			if det.fail() {
				s.suspend(latest)
			}
			return err
		}

		now := time.Now()
		state.mu.Lock()
		state.status = StatusRunning
		state.pid = process.PID()
		state.startedAt = &now
		state.lastRunAt = &now
		state.mu.Unlock()

		current = process
		_ = s.db.SetLastRun(latest.ID, now, nil)
		s.logger.Info("任务已启动",
			"task_id", latest.ID, "task", latest.Name, "pid", process.PID(), "triggered_by", triggeredBy)

		// 进程退出时向调度循环投递通知; proc.Process 内部保证 Wait 只执行一次.
		done := process.Done()
		go func() {
			<-done
			select {
			case exitCh <- processExited{code: process.Wait()}:
			default:
			}
		}()
		return nil
	}

	// stopCurrent 停止当前进程.
	//
	// 这里只负责发信号与等待退出, 退出后的状态更新与重启决策统一由主循环的
	// exitCh 分支处理, 避免两处都去消费退出通知导致状态残留.
	stopCurrent := func(reason string) {
		if current == nil {
			return
		}
		if _, err := current.Stop(); err != nil {
			s.warn(id, initial.Name, fmt.Sprintf("停止任务失败: %v", err))
		}
		s.logger.Debug("任务已停止", "task_id", id, "reason", reason)
	}

	// handleExit 处理进程退出后的状态与重启决策.
	handleExit := func(code int) {
		current = nil
		now := time.Now()

		_ = s.db.SetLastExitCode(id, code)
		s.logger.Info("任务已退出",
			"task_id", id, "task", initial.Name, "exit_code", code, "at", now.Format(time.RFC3339))

		latest, ok := reload()
		if !ok {
			// 任务已被删除, 状态随后会被调度器丢弃, 这里只清理运行信息.
			state.mu.Lock()
			state.status = StatusIdle
			state.pid = 0
			state.startedAt = nil
			state.lastExit = &code
			state.mu.Unlock()
			return
		}

		// 用户主动暂停的自动任务显示为 paused, 让界面区分 "空闲" 与 "用户按停".
		// 判定用最新定义: 用户可能刚把触发方式改成自动类, 这时暂停标记依然要生效.
		paused := pausedByUser && autoRestartable(latest.Trigger.Kind)

		state.mu.Lock()
		// 挂起状态要在覆盖之前读取, 覆盖之后就只剩 idle / paused 了.
		suspended := state.status == StatusSuspended
		switch {
		case suspended:
			// 挂起是持续状态, 进程退出不该把它抹掉.
			state.status = StatusSuspended
		case paused:
			state.status = StatusPaused
		default:
			state.status = StatusIdle
		}
		state.pid = 0
		state.startedAt = nil
		state.lastExit = &code
		state.mu.Unlock()

		if !latest.Enabled || suspended {
			return
		}
		// 用户主动停止后不再自动拉起, 直到用户重新运行.
		//
		// 这一层判断必须在触发方式分支之前: 保活与直到成功都是 "退出就重启",
		// 少了它, 用户按停止得到的只是几十毫秒的空闲.
		if paused {
			s.logger.Debug("任务已暂停, 跳过自动重启", "task_id", id)
			return
		}

		switch latest.Trigger.Kind {
		case task.TriggerKeepAlive:
			if code != 0 {
				if det.fail() {
					s.suspend(latest)
					return
				}
			} else {
				det.reset()
			}
			state.mu.Lock()
			state.restarts++
			state.mu.Unlock()
			if err := launch("keep-alive"); err != nil {
				s.logger.Debug("保活重启失败", "task_id", id, "error", err)
			}
		case task.TriggerUntilSucceed:
			if code != 0 {
				if err := launch("until-succeed"); err != nil {
					s.logger.Debug("重试失败", "task_id", id, "error", err)
				}
			} else {
				det.reset()
			}
		}
	}

	// 退出时确保子进程被回收.
	defer func() {
		if current != nil {
			if _, err := current.Stop(); err != nil {
				s.logger.Warn("退出时停止任务失败", "task_id", id, "error", err)
			}
			current = nil
		}
	}()

	// 启动时的初始触发.
	switch initial.Trigger.Kind {
	case task.TriggerStartup, task.TriggerKeepAlive, task.TriggerUntilSucceed:
		if initial.Enabled {
			if err := launch("startup"); err != nil {
				s.logger.Debug("启动触发失败", "task_id", id, "error", err)
			}
		}
	}

	// 定时一次的等待由独立协程完成, 到点后向循环投递触发信号.
	instantCh := make(chan struct{}, 1)
	if initial.Trigger.Kind == task.TriggerInstant && initial.Trigger.RunAt != nil && initial.Enabled {
		alreadyRan := initial.LastRunAt != nil && !initial.LastRunAt.Before(*initial.Trigger.RunAt)
		if !alreadyRan {
			runAt := *initial.Trigger.RunAt
			go func() {
				delay := time.Until(runAt)
				if delay > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(delay):
					}
				}
				select {
				case instantCh <- struct{}{}:
				default:
				}
			}()
		} else {
			instantDone = true
		}
	}

	var routineTicker *time.Ticker
	if initial.Trigger.Kind == task.TriggerRoutine && initial.Trigger.IntervalSeconds > 0 {
		routineTicker = time.NewTicker(time.Duration(initial.Trigger.IntervalSeconds) * time.Second)
		defer routineTicker.Stop()
	}

	for {
		var tick <-chan time.Time
		if routineTicker != nil {
			tick = routineTicker.C
		}
		select {
		case <-ctx.Done():
			return

		case msg := <-g.control:
			switch msg.kind {
			case controlRun:
				det.reset()
				s.clearSuspended(state)
				// 手动运行即视为恢复, 用户点了运行就说明希望它继续跑.
				pausedByUser = false
				// 先退回空闲再启动, 启动失败时界面不会残留一个已经失效的暂停标记.
				state.mu.Lock()
				if state.status == StatusPaused {
					state.status = StatusIdle
				}
				state.mu.Unlock()
				err := launch("manual")
				if msg.reply != nil {
					msg.reply <- err
				}
			case controlStop:
				// 先记暂停再停进程: 退出通知由 handleExit 处理,
				// 顺序反过来的话退出分支会先看到未暂停而把进程重新拉起.
				pausedByUser = true
				wasRunning := current != nil
				stopCurrent("manual")
				// 进程本来就没在跑时不会有退出通知, 这里直接把状态置为暂停,
				// 否则界面看不出用户刚刚按了停止. 挂起状态不覆盖, 它表达的是
				// "连续失败" 这一更重要的信息.
				if !wasRunning && autoRestartable(initial.Trigger.Kind) {
					state.mu.Lock()
					if state.status != StatusSuspended {
						state.status = StatusPaused
					}
					state.mu.Unlock()
				}
				if msg.reply != nil {
					msg.reply <- nil
				}
			}

		case <-tick:
			state.mu.Lock()
			suspended := state.status == StatusSuspended
			state.mu.Unlock()
			if suspended || pausedByUser || current != nil {
				continue
			}
			if err := launch("routine"); err != nil {
				s.logger.Debug("定时触发失败", "task_id", id, "error", err)
			}

		case <-instantCh:
			if instantDone || current != nil {
				continue
			}
			instantDone = true
			if err := launch("instant"); err != nil {
				s.logger.Debug("定时一次触发失败", "task_id", id, "error", err)
			}

		case exited := <-exitCh:
			handleExit(exited.code)
		}
	}
}

// suspend 把任务标记为暂停并记录告警.
func (s *Scheduler) suspend(t task.Task) {
	state := s.state(t.ID)
	state.mu.Lock()
	state.status = StatusSuspended
	state.mu.Unlock()
	s.warn(t.ID, t.Name, fmt.Sprintf("连续失败 %d 次, 已暂停自动触发", suspensionFailures))
}

// clearSuspended 解除挂起状态.
func (s *Scheduler) clearSuspended(state *taskState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.status == StatusSuspended {
		state.status = StatusIdle
	}
	state.failures = 0
}

// state 获取或创建任务状态.
func (s *Scheduler) state(id int64) *taskState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[id]
	if !ok {
		st = &taskState{status: StatusIdle}
		s.states[id] = st
	}
	return st
}

// warn 记录一条告警.
func (s *Scheduler) warn(taskID int64, taskName, message string) {
	s.logger.Warn("任务告警", "task_id", taskID, "task", taskName, "message", message)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.warnings) >= maxWarnings {
		s.warnings = s.warnings[len(s.warnings)-maxWarnings+1:]
	}
	s.warnings = append(s.warnings, Warning{
		TaskID:    taskID,
		TaskName:  taskName,
		Message:   message,
		CreatedAt: time.Now(),
	})
}

// TakeWarnings 取走当前累积的告警.
func (s *Scheduler) TakeWarnings() []Warning {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.warnings) == 0 {
		return nil
	}
	out := s.warnings
	s.warnings = nil
	return out
}

// Snapshots 返回全部任务的运行状态快照.
func (s *Scheduler) Snapshots() map[int64]Snapshot {
	s.mu.Lock()
	states := make(map[int64]*taskState, len(s.states))
	for id, st := range s.states {
		states[id] = st
	}
	s.mu.Unlock()

	out := make(map[int64]Snapshot, len(states))
	for id, st := range states {
		out[id] = st.snapshot(id)
	}
	return out
}

// Snapshot 返回单个任务的状态.
func (s *Scheduler) Snapshot(id int64) Snapshot {
	s.mu.Lock()
	st, ok := s.states[id]
	s.mu.Unlock()
	if !ok {
		return Snapshot{TaskID: id, Status: StatusIdle}
	}
	return st.snapshot(id)
}

// SaveTask 保存任务并按新定义重建调度协程.
func (s *Scheduler) SaveTask(t task.Task) (int64, error) {
	id, err := s.db.SaveTask(t)
	if err != nil {
		return 0, err
	}
	latest, err := s.db.GetTask(id)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	status := StatusIdle
	if !latest.Enabled {
		status = StatusDisabled
	}
	s.states[id] = &taskState{status: status}
	s.mu.Unlock()

	s.spawnGuard(latest)
	return id, nil
}

// DeleteTask 删除任务并停止它的调度协程.
func (s *Scheduler) DeleteTask(id int64) error {
	s.stopGuard(id)
	if err := s.db.DeleteTask(id); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.states, id)
	s.mu.Unlock()
	return nil
}

// SetEnabled 启用或停用任务.
func (s *Scheduler) SetEnabled(id int64, enabled bool) error {
	if err := s.db.SetEnabled(id, enabled); err != nil {
		return err
	}
	latest, err := s.db.GetTask(id)
	if err != nil {
		return err
	}

	state := s.state(id)
	state.mu.Lock()
	if enabled {
		state.status = StatusIdle
		state.failures = 0
	} else {
		state.status = StatusDisabled
	}
	state.mu.Unlock()

	// 停用需要先停掉正在运行的进程, 再重建协程让新状态生效.
	s.stopGuard(id)
	s.spawnGuard(latest)
	return nil
}

// RunTask 手动运行任务.
func (s *Scheduler) RunTask(id int64) error {
	return s.sendControl(id, controlRun)
}

// StopTask 停止正在运行的任务进程.
func (s *Scheduler) StopTask(id int64) error {
	return s.sendControl(id, controlStop)
}

// sendControl 向任务 guard 发送指令并等待处理结果.
func (s *Scheduler) sendControl(id int64, kind controlKind) error {
	s.guardMu.Lock()
	g, ok := s.guards[id]
	s.guardMu.Unlock()
	if !ok {
		return fmt.Errorf("任务 %d 未在调度中", id)
	}
	reply := make(chan error, 1)
	select {
	case g.control <- controlMsg{kind: kind, reply: reply}:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("任务 %d 未响应指令", id)
	}
	select {
	case err := <-reply:
		return err
	case <-time.After(proc.StopTimeout + 5*time.Second):
		return fmt.Errorf("任务 %d 处理指令超时", id)
	}
}

// Close 停止全部任务并关闭调度器.
func (s *Scheduler) Close() {
	s.stopOnce.Do(func() {
		s.cancel()

		s.guardMu.Lock()
		ids := make([]int64, 0, len(s.guards))
		for id := range s.guards {
			ids = append(ids, id)
		}
		s.guardMu.Unlock()

		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Add(1)
			go func(id int64) {
				defer wg.Done()
				s.stopGuard(id)
			}(id)
		}
		wg.Wait()
		close(s.stopped)
		s.logger.Info("调度器已停止")
	})
}

// Done 在调度器关闭后关闭.
func (s *Scheduler) Done() <-chan struct{} { return s.stopped }
