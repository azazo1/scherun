// Package app 装配并运行 scherun 的全部组件.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/azazo1/scherun/internal/appdirs"
	"github.com/azazo1/scherun/internal/browser"
	"github.com/azazo1/scherun/internal/config"
	"github.com/azazo1/scherun/internal/localapi"
	"github.com/azazo1/scherun/internal/logging"
	"github.com/azazo1/scherun/internal/scheduler"
	"github.com/azazo1/scherun/internal/single"
	"github.com/azazo1/scherun/internal/store"
	"github.com/azazo1/scherun/internal/tray"
	"github.com/azazo1/scherun/internal/version"
	"github.com/azazo1/scherun/internal/webui"
)

// Options 是启动参数.
type Options struct {
	// Bind 覆盖配置中的监听地址, 为空时用配置值.
	Bind string
	// NoBrowser 为真时不自动打开浏览器.
	NoBrowser bool
	// LogLevel 覆盖日志级别, 为空时用环境变量或 info.
	LogLevel string
}

// Run 启动应用并阻塞到退出.
func Run(opts Options) error {
	// 1. 日志: 尽量早初始化, 后续失败也能留下线索.
	logFile, err := appdirs.LogFile()
	if err != nil {
		return err
	}
	cfg, cfgErr := config.LoadFromDefaultPath()
	if cfgErr != nil {
		// 配置损坏已经退化为默认值, 只记录, 不阻止启动.
		cfg = config.Default()
	}

	level := opts.LogLevel
	if level == "" {
		level = os.Getenv("SCHERUN_LOG_LEVEL")
	}
	log, err := logging.Init(logging.Options{
		File:          logFile,
		Level:         level,
		RetentionDays: cfg.LogRetentionDays,
	})
	if err != nil {
		return err
	}
	defer log.Close()
	logger := log.Logger
	if cfgErr != nil {
		logger.Warn("配置加载有异常, 已使用默认值", "error", cfgErr)
	}

	v, commit, _ := version.Info()
	logger.Info("scherun 启动",
		"version", v, "commit", commit, "log_file", logFile, "pid", os.Getpid())

	// 2. 单实例锁: 拿不到就把请求转给已有实例.
	lockPath, err := appdirs.LockPath()
	if err != nil {
		return err
	}
	lock, err := single.Acquire(lockPath)
	if err != nil {
		if err == single.ErrAlreadyRunning {
			logger.Info("已有实例在运行, 转发打开请求后退出")
			forwardToRunning(logger)
			return nil
		}
		return err
	}
	defer lock.Release()

	// 3. 数据库与调度器.
	dbPath, err := appdirs.DatabasePath()
	if err != nil {
		return err
	}
	db, err := store.Open(dbPath)
	if err != nil {
		logger.Error("打开数据库失败", "path", dbPath, "error", err)
		return err
	}
	defer db.Close()
	logger.Info("数据库已就绪", "path", dbPath)

	sched := scheduler.New(db, logger)
	if err := sched.Start(); err != nil {
		return fmt.Errorf("启动调度器失败: %w", err)
	}

	// 4. 退出控制: 所有退出入口都汇入同一个函数.
	var (
		exitOnce sync.Once
		exitCh   = make(chan struct{})
		appCtx   context.Context
		cancel   context.CancelFunc
	)
	appCtx, cancel = context.WithCancel(context.Background())
	defer cancel()

	requestExit := func(reason string) {
		exitOnce.Do(func() {
			logger.Info("开始退出", "reason", reason)
			close(exitCh)
		})
	}

	// 5. 本地 HTTP 服务.
	bind := cfg.LocalBind
	if opts.Bind != "" {
		bind = opts.Bind
	}
	secretPath, err := appdirs.SecretPath()
	if err != nil {
		return err
	}
	runtimePath, err := appdirs.RuntimePath()
	if err != nil {
		return err
	}
	// server 先声明再赋值, 因为 OnExit 与 OnOpen 回调需要引用它.
	var server *localapi.Server
	server, err = localapi.New(localapi.Options{
		Bind:        bind,
		WebFS:       webui.FS,
		Version:     version.Display(),
		Commit:      commit,
		RuntimePath: runtimePath,
		SecretPath:  secretPath,
		Config:      &cfg,
		DB:          db,
		Scheduler:   sched,
		Logger:      logger,
		OnExit:      func() { requestExit("界面退出") },
		OnOpen:      func() { openInterface(logger, server, true) },
	})
	if err != nil {
		return err
	}
	if err := server.Start(); err != nil {
		sched.Close()
		return err
	}

	// 6. 托盘状态刷新.
	trayController := tray.New("scherun", version.Display(), tray.Actions{
		Open: func() { openInterface(logger, server, true) },
		Quit: func() { requestExit("托盘退出") },
	}, logger)
	// tray.HideDockIcon() // 临时禁用, 验证崩溃原因

	stopTrayStatus := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopTrayStatus:
				return
			case <-ticker.C:
				trayController.Update(trayState(sched))
			}
		}
	}()

	// 7. 信号: Ctrl+C 与 SIGTERM 走同一条优雅退出路径.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case sig := <-signals:
			requestExit("收到信号 " + sig.String())
		case <-exitCh:
		}
	}()

	// 8. 自动打开浏览器.
	if !opts.NoBrowser && !cfg.QuietLaunch {
		go func() {
			// 稍等让服务稳定, 避免浏览器打开过快导致连接被拒.
			time.Sleep(300 * time.Millisecond)
			openInterface(logger, server, false)
		}()
	}

	// 9. 托盘事件循环必须占用主 goroutine: macOS 的 AppKit 要求 UI 在主线程上跑,
	// 放到子协程会触发 LaunchServices 断言并崩溃.
	//
	// 退出流程因此移到后台协程: 收到退出请求后先收尾, 再让托盘循环结束.
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		<-exitCh
		close(stopTrayStatus)

		// 收尾顺序: 停任务 -> 关 HTTP -> 刷盘日志 -> 释放锁.
		logger.Info("停止全部任务")
		sched.Close()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)

		// 取消 ctx 让托盘循环退出, 把主 goroutine 还给 main.
		cancel()
	}()

	if err := trayController.Run(appCtx); err != nil {
		logger.Error("托盘运行失败", "error", err)
	}

	// 托盘已退出, 若收尾还没跑完 (例如托盘自行退出), 主动触发一次.
	select {
	case <-cleanupDone:
	case <-time.After(2 * time.Second):
		requestExit("托盘循环结束")
		<-cleanupDone
	}

	logger.Info("scherun 已退出")
	return nil
}

// trayState 汇总托盘需要的状态.
func trayState(sched *scheduler.Scheduler) tray.State {
	snapshots := sched.Snapshots()
	state := tray.State{Healthy: true, Total: len(snapshots)}
	for _, snap := range snapshots {
		switch snap.Status {
		case scheduler.StatusRunning:
			state.Running++
		case scheduler.StatusSuspended:
			state.Failed++
		}
	}
	return state
}

// openInterface 在浏览器中打开管理界面.
//
// force 为真时即使界面已经打开也重新打开, 用于托盘点击; 自动启动时用 false.
func openInterface(logger *slog.Logger, server *localapi.Server, force bool) {
	if server == nil {
		return
	}
	url := server.URL()
	if err := browser.Open(url); err != nil {
		logger.Error("打开浏览器失败, 请手动访问", "url", server.BaseURL(), "error", err)
		return
	}
	logger.Info("已打开管理界面", "url", server.BaseURL())
}

// forwardToRunning 把打开请求转发给已在运行的实例.
func forwardToRunning(logger *slog.Logger) {
	runtimePath, err := appdirs.RuntimePath()
	if err != nil {
		logger.Warn("无法定位运行状态文件", "error", err)
		return
	}
	state, err := localapi.ReadRuntime(runtimePath)
	if err != nil {
		logger.Warn("读取运行状态失败, 无法转发", "error", err)
		return
	}
	if err := localapi.Focus(state.Port, state.Token); err != nil {
		logger.Warn("转发打开请求失败", "error", err)
	}
}
