// Package browser 负责用系统默认浏览器打开地址.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open 用默认浏览器打开 url.
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// rundll32 避免 start 的引号处理问题.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开浏览器失败: %w", err)
	}
	// 不等待浏览器退出, 但需要回收子进程避免僵尸.
	go func() { _ = cmd.Wait() }()
	return nil
}
