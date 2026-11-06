// Package proc 负责启动与终止任务子进程.
//
// 平台差异集中在 proc_unix.go 与 proc_windows.go:
// unix 用独立进程组 + SIGINT 停止, windows 用终止进程.
package proc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/azazo1/scherun/internal/pathutil"
	"github.com/azazo1/scherun/internal/task"
)

// StopTimeout 是发送停止信号后等待进程退出的最长时间, 超时后强制终止.
const StopTimeout = 5 * time.Second

// Spec 描述一次进程启动.
type Spec struct {
	Program    string
	Args       []string
	WorkingDir string
	EnvVars    map[string]string
	Stdin      string
	Stdout     string
	Stderr     string
	NoConsole  bool
}

// SpecFromTask 由任务定义生成启动参数.
func SpecFromTask(t task.Task) Spec {
	return Spec{
		Program:    t.Program,
		Args:       t.Args,
		WorkingDir: t.WorkingDir,
		EnvVars:    t.EnvVars,
		Stdin:      t.Stdin,
		Stdout:     t.Stdout,
		Stderr:     t.Stderr,
		NoConsole:  t.NoConsole,
	}
}

// Process 是一个已启动的子进程.
//
// exec.Cmd.Wait 只能调用一次, 因此启动后立刻用一个内部协程完成等待,
// 结果缓存在 done 与 code 里; 外部无论调用多少次 Wait 都拿到同一结果.
type Process struct {
	cmd  *exec.Cmd
	spec Spec

	done     chan struct{}
	code     int
	waitOnce sync.Once

	openMu sync.Mutex
	opened []*os.File
}

// PID 返回子进程 id, 进程未启动时返回 0.
func (p *Process) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// Wait 等待子进程退出并返回退出码, 可重复调用.
func (p *Process) Wait() int {
	if p == nil || p.cmd == nil {
		return -1
	}
	<-p.done
	return p.code
}

// Done 在子进程退出后关闭.
func (p *Process) Done() <-chan struct{} {
	if p == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return p.done
}

// Stop 请求子进程退出, 超时后强制终止, 返回退出码.
//
// unix 下先向整个进程组发 SIGINT, 让任务有机会做清理; windows 直接终止.
func (p *Process) Stop() (int, error) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return -1, fmt.Errorf("进程未启动")
	}
	if err := p.stop(); err != nil {
		return p.Wait(), err
	}
	return p.Wait(), nil
}

// closeFiles 关闭为子进程打开的重定向文件.
func (p *Process) closeFiles() {
	p.openMu.Lock()
	defer p.openMu.Unlock()
	for _, file := range p.opened {
		_ = file.Close()
	}
	p.opened = nil
}

// track 记录需要关闭的文件.
func (p *Process) track(file *os.File) {
	p.openMu.Lock()
	defer p.openMu.Unlock()
	p.opened = append(p.opened, file)
}

// Start 启动子进程.
func Start(spec Spec) (*Process, error) {
	if strings.TrimSpace(spec.Program) == "" {
		return nil, fmt.Errorf("程序路径为空")
	}

	program := pathutil.ExpandHome(spec.Program)
	var args []string
	// macOS 的 .app 是目录, 必须经 open 启动, 此时无法重定向标准流也无法获取真实退出码.
	if runtime.GOOS == "darwin" && isAppBundle(program) {
		args = append([]string{"-a", program}, spec.Args...)
		program = "/usr/bin/open"
	} else {
		args = spec.Args
	}

	cmd := exec.Command(program, args...)
	cmd.Dir = workingDir(spec)

	// 继承当前环境后再覆盖任务自定义变量.
	env := os.Environ()
	for key, value := range spec.EnvVars {
		env = append(env, key+"="+value)
	}
	cmd.Env = env

	process := &Process{
		cmd:  cmd,
		spec: spec,
		done: make(chan struct{}),
	}
	if err := process.attachStdio(); err != nil {
		process.closeFiles()
		return nil, err
	}
	configure(cmd, spec)

	if err := cmd.Start(); err != nil {
		process.closeFiles()
		return nil, fmt.Errorf("启动程序 %s 失败: %w", spec.Program, err)
	}

	// 只在这里等待一次, 结果对所有调用方可见.
	go func() {
		waitErr := cmd.Wait()
		process.code = exitCodeOf(waitErr)
		process.closeFiles()
		close(process.done)
	}()
	return process, nil
}

// exitCodeOf 把 Wait 的错误转成退出码.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok {
		return exitCode(exitErr)
	}
	return -1
}

// asExitError 是 errors.As 的薄包装, 避免在平台文件里重复导入 errors.
func asExitError(err error, target **exec.ExitError) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		*target = exitErr
		return true
	}
	return false
}

// attachStdio 按配置重定向标准输入输出.
//
// 相对路径按任务的工作目录解析, 而不是按 schrun 自身的 cwd:
// 后者会让 out.log 悄悄落到应用进程的目录里, 与用户预期不符.
func (p *Process) attachStdio() error {
	base := p.cmd.Dir

	if p.spec.Stdin != "" {
		// 输入文件只读取, 不创建父目录: 目录不存在说明配置有误, 如实报错.
		file, err := os.Open(pathutil.Resolve(base, p.spec.Stdin))
		if err != nil {
			return fmt.Errorf("打开标准输入文件失败: %w", err)
		}
		p.track(file)
		p.cmd.Stdin = file
	}
	if p.spec.Stdout != "" {
		target := pathutil.Resolve(base, p.spec.Stdout)
		if err := ensureParentDir(target); err != nil {
			return fmt.Errorf("创建标准输出目录失败: %w", err)
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("打开标准输出文件失败: %w", err)
		}
		p.track(file)
		p.cmd.Stdout = file
	}
	if p.spec.Stderr != "" {
		target := pathutil.Resolve(base, p.spec.Stderr)
		if err := ensureParentDir(target); err != nil {
			return fmt.Errorf("创建标准错误目录失败: %w", err)
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("打开标准错误文件失败: %w", err)
		}
		p.track(file)
		p.cmd.Stderr = file
	}
	return nil
}

// ensureParentDir 创建输出文件所在的父目录.
//
// 日志目录常常还没建立, 不预先创建会让任务在启动阶段直接失败.
// 路径本身没有目录部分 (相对文件名) 时无需处理.
func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" || dir == string(filepath.Separator) {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// workingDir 决定子进程的工作目录.
//
// 优先使用任务指定目录, 否则退回到程序所在目录, 让相对路径行为符合直觉.
func workingDir(spec Spec) string {
	if spec.WorkingDir != "" {
		return pathutil.ExpandHome(spec.WorkingDir)
	}
	parent := filepath.Dir(pathutil.ExpandHome(spec.Program))
	if parent == "." || parent == "" {
		return ""
	}
	return parent
}

// isAppBundle 判断路径是否是 macOS 的 .app 目录.
func isAppBundle(path string) bool {
	if !strings.HasSuffix(strings.ToLower(path), ".app") {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Runnable 判断程序是否可执行, 用于界面提前提示.
//
// 判断逻辑必须与 Start 保持一致: 只有程序名 (不含路径分隔符) 时,
// exec.Command 会沿 PATH 查找, 这里也要先 LookPath, 否则会出现
// "任务在跑但界面显示程序不可执行" 的矛盾.
func Runnable(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	resolved := pathutil.ExpandHome(path)
	if !strings.ContainsAny(path, `/\`) {
		found, err := exec.LookPath(path)
		if err != nil {
			return false
		}
		resolved = found
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return false
	}
	if info.IsDir() {
		// 目录只接受 macOS 的 .app.
		return runtime.GOOS == "darwin" && isAppBundle(resolved)
	}
	return executable(info)
}
