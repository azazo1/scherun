// Package logging 初始化 slog, 同时输出到终端与轮转日志文件.
//
// 日志文件按大小轮转并保留有限份数, 启动时清理超过保留期的旧文件.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// 单文件大小与备份份数上限, 与保留天数共同限制磁盘占用.
const (
	maxFileSizeMB = 10
	maxBackups    = 10
)

// Options 是日志初始化参数.
type Options struct {
	// File 是主日志文件路径, 为空时只输出到终端.
	File string
	// Level 是日志级别, 支持 debug / info / warn / error, 为空时用 info.
	Level string
	// RetentionDays 是日志保留天数, 小于 1 时按 5 天处理.
	RetentionDays int
}

// Logger 包装 slog.Logger 与关闭逻辑.
type Logger struct {
	*slog.Logger
	closer io.Closer
}

// Init 创建日志器.
//
// 终端保留 info 及以上级别, 文件按 Level 记录, 因此调试实例可以把细节只写进文件.
func Init(opts Options) (*Logger, error) {
	level := parseLevel(opts.Level)
	retention := opts.RetentionDays
	if retention < 1 {
		retention = 5
	}

	var handlers []slog.Handler
	if stderrUsable() {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	}

	var closer io.Closer
	if opts.File != "" {
		if err := os.MkdirAll(filepath.Dir(opts.File), 0o700); err != nil {
			return nil, fmt.Errorf("创建日志目录失败: %w", err)
		}
		cleanupExpired(opts.File, retention)

		rotator := &lumberjack.Logger{
			Filename:   opts.File,
			MaxSize:    maxFileSizeMB,
			MaxBackups: maxBackups,
			MaxAge:     retention,
			LocalTime:  true,
			Compress:   false,
		}
		handlers = append(handlers, slog.NewTextHandler(rotator, &slog.HandlerOptions{Level: level}))
		closer = rotator
	}

	return &Logger{Logger: slog.New(&multiHandler{handlers: handlers}), closer: closer}, nil
}

// stderrUsable 判断终端输出是否可用.
//
// Windows 安装版声明为 GUI 子系统, 此时 stderr 句柄无效, 写进去只会失败;
// 而 release 构建不带控制台, 终端输出本来也没人看. 文件日志不受影响.
//
// 用 Stat 探测而不是按平台写死: 从终端启动的调试构建仍能保留终端日志.
func stderrUsable() bool {
	_, err := os.Stderr.Stat()
	return err == nil
}

// Close 刷盘并关闭日志文件.
func (l *Logger) Close() error {
	if l == nil || l.closer == nil {
		return nil
	}
	return l.closer.Close()
}

// parseLevel 把字符串转成 slog 级别.
func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "trace":
		// slog 没有 trace, 用 debug 承载最细的诊断信息.
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// multiHandler 把同一条记录分发给多个 handler.
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, record.Level) {
			continue
		}
		if err := h.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: next}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: next}
}

// cleanupExpired 删除超过保留期的日志文件.
//
// lumberjack 自己按 MaxAge 清理轮转文件, 这里额外处理主文件之外的历史文件.
func cleanupExpired(mainFile string, retentionDays int) {
	dir := filepath.Dir(mainFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	deadline := time.Now().AddDate(0, 0, -retentionDays)
	base := strings.TrimSuffix(filepath.Base(mainFile), filepath.Ext(mainFile))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(deadline) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}
