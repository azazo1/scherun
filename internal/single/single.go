// Package single 实现按数据目录绑定的单实例锁.
//
// 锁文件放在数据目录内, 因此隔离数据目录的调试实例可以与日常实例并行运行.
package single

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gofrs/flock"
)

// ErrAlreadyRunning 表示已有实例持有锁.
var ErrAlreadyRunning = errors.New("已有实例在运行")

// Lock 是持有中的单实例锁.
type Lock struct {
	file *flock.Flock
	path string
}

// Acquire 尝试获取单实例锁.
//
// 获取失败时返回 ErrAlreadyRunning, 调用方应当转发启动参数给已有实例后退出.
func Acquire(path string) (*Lock, error) {
	file := flock.New(path)
	locked, err := file.TryLock()
	if err != nil {
		return nil, fmt.Errorf("获取单实例锁失败: %w", err)
	}
	if !locked {
		return nil, ErrAlreadyRunning
	}
	// 写入 pid 便于排查, 失败不影响加锁结果.
	_ = os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600)
	return &Lock{file: file, path: path}, nil
}

// Release 释放锁.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	if err := l.file.Unlock(); err != nil {
		return fmt.Errorf("释放单实例锁失败: %w", err)
	}
	return nil
}

// HolderPID 读取锁文件里记录的 pid, 读不到时返回 0.
func HolderPID(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return 0
	}
	return pid
}
