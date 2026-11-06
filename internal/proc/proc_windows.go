//go:build windows

package proc

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// createNoWindow 让子进程不弹出控制台窗口.
const createNoWindow = 0x08000000

// configure 处理 Windows 特有的启动选项.
//
// no_console 时既隐藏窗口, 也把标准流置空: GUI 程序没有可继承的句柄,
// 保留默认继承会让 CreateProcess 因无效句柄失败.
func configure(cmd *exec.Cmd, spec Spec) {
	if !spec.NoConsole {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	if spec.Stdin == "" {
		cmd.Stdin = nil
	}
	if spec.Stdout == "" {
		cmd.Stdout = nil
	}
	if spec.Stderr == "" {
		cmd.Stderr = nil
	}
}

// stop 终止进程, Windows 没有与 SIGINT 等价的优雅停止手段.
func (p *Process) stop() error {
	if err := p.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("终止进程失败: %w", err)
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(StopTimeout):
		return fmt.Errorf("进程在强制终止后仍未退出")
	}
}

// exitCode 返回退出码.
func exitCode(err *exec.ExitError) int {
	return err.ExitCode()
}

// executable 在 Windows 上不检查权限位, 只看是否为普通文件.
func executable(info os.FileInfo) bool {
	return info.Mode().IsRegular()
}
