//go:build !windows

package proc

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// configure 让子进程独占一个进程组, 停止时可以给整组发信号.
func configure(cmd *exec.Cmd, spec Spec) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// stop 向进程组发送 SIGINT, 超时后强杀.
//
// 只负责发信号与等待退出结果, 不调用 cmd.Wait: 等待由 Start 里唯一的协程完成.
func (p *Process) stop() error {
	pid := p.PID()
	if pid <= 0 {
		return fmt.Errorf("进程未启动")
	}

	// 负 pid 表示向整个进程组发信号, 任务派生的子进程也能一起退出.
	if err := syscall.Kill(-pid, syscall.SIGINT); err != nil {
		// 进程组不存在时退回到只杀进程本身.
		if killErr := p.cmd.Process.Kill(); killErr != nil {
			return fmt.Errorf("发送停止信号失败: %w", err)
		}
	}

	select {
	case <-p.done:
		return nil
	case <-time.After(StopTimeout):
	}

	// 超时后强制终止整个进程组, 避免残留子进程.
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = p.cmd.Process.Kill()

	select {
	case <-p.done:
		return nil
	case <-time.After(StopTimeout):
		return fmt.Errorf("进程在强制终止后仍未退出")
	}
}

// exitCode 从 exec.ExitError 里取出退出码, 被信号杀死时按 128+signal 处理.
func exitCode(err *exec.ExitError) int {
	if status, ok := err.Sys().(syscall.WaitStatus); ok {
		if status.Signaled() {
			return 128 + int(status.Signal())
		}
		return status.ExitStatus()
	}
	return err.ExitCode()
}

// executable 判断文件是否带可执行位.
func executable(info os.FileInfo) bool {
	return info.Mode()&0o111 != 0
}
