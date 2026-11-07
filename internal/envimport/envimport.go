// Package envimport 从用户的登录 shell 读取环境变量.
//
// 从 Finder 或 Dock 启动的 macOS 应用只继承 launchd 的最小环境, PATH 通常只有
// /usr/bin:/bin:/usr/sbin:/sbin, 于是 adb, node, ffmpeg 这类装在 homebrew 或
// 用户目录下的命令全部找不到. 这里在启动时跑一次登录 shell, 把它的环境抄过来.
package envimport

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// DefaultTimeout 是执行导入命令的最长等待时间.
//
// 交互式 shell (fish -i) 可能加载插件而变慢, 这里给足余量;
// 超时只影响导入, 不阻止应用启动.
const DefaultTimeout = 10 * time.Second

// DefaultCommand 返回当前平台的默认导入命令.
//
// 取用户默认 shell 的登录模式: -l 让 shell 读取登录配置, 里面通常才有
// homebrew 与 SDK 的 PATH 设置. -c 'env -0' 用 NUL 分隔输出, 变量值里有换行也不会解析错.
func DefaultCommand() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			// Windows 没有登录 shell 概念, 默认不导入.
			return nil
		}
		shell = "/bin/sh"
	}
	return []string{shell, "-l", "-c", "env -0"}
}

// Import 执行导入命令, 返回解析出的环境变量.
//
// command 为空时用 DefaultCommand. 返回的 map 不含顺序信息,
// 需要顺序时用 SortedPairs.
func Import(ctx context.Context, command []string, timeout time.Duration) (map[string]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(command) == 0 {
		command = DefaultCommand()
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("没有可用的导入命令")
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	// 子 shell 不该继承当前进程的 PATH: 我们要的是它自己算出来的结果.
	cmd.Env = baseEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("导入命令超时 (%s): %s", timeout, strings.Join(command, " "))
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("导入命令失败: %v: %s", err, firstLine(detail))
		}
		return nil, fmt.Errorf("导入命令失败: %w", err)
	}

	vars, err := parseNulSeparated(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	if len(vars) == 0 {
		return nil, fmt.Errorf("导入命令没有输出任何环境变量")
	}
	return vars, nil
}

// baseEnv 是执行导入命令时传给子 shell 的环境.
//
// 只保留 HOME, USER, SHELL 这些 shell 初始化必需的量, 避免把当前进程
// 那份残缺的 PATH 混进去, 否则 shell 可能顺着它找到错误的程序.
func baseEnv() []string {
	keep := []string{"HOME", "USER", "LOGNAME", "SHELL", "TERM", "LANG", "LC_ALL", "TMPDIR"}
	env := make([]string, 0, len(keep))
	for _, key := range keep {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

// parseNulSeparated 解析 env -0 的输出.
//
// 每项形如 KEY=VALUE, 以 NUL 分隔. 值里可以有换行与等号, 只按第一个等号切分.
func parseNulSeparated(data []byte) (map[string]string, error) {
	vars := make(map[string]string)
	for _, entry := range bytes.Split(data, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		key, value, ok := bytes.Cut(entry, []byte{'='})
		if !ok || len(key) == 0 {
			// 不合规的条目直接跳过, 不让它拖垮整次导入.
			continue
		}
		vars[string(key)] = string(value)
	}
	return vars, nil
}

// SortedPairs 把变量表转成排序后的 KEY=VALUE 列表, 便于日志与稳定输出.
func SortedPairs(vars map[string]string) []string {
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+vars[key])
	}
	return pairs
}

// Merge 按顺序合并多份变量, 后面的覆盖前面的.
//
// nil 与空 map 会被跳过, 便于调用方直接把可能为空的多层传进来.
func Merge(layers ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, layer := range layers {
		for key, value := range layer {
			merged[key] = value
		}
	}
	return merged
}

// SelectLayers 按任务的开关选择要合并的层.
//
// 调度器启动任务与界面判定可执行性都要用它, 两处必须一致:
// 判定用的是哪份 PATH, 启动就该用哪份, 否则会出现 "界面说不可执行但任务在跑".
func SelectLayers(shellEnv, globalEnv map[string]string, useShell, useGlobal bool) map[string]string {
	merged := make(map[string]string)
	if useShell {
		for key, value := range shellEnv {
			merged[key] = value
		}
	}
	if useGlobal {
		for key, value := range globalEnv {
			merged[key] = value
		}
	}
	return merged
}

// ApplyToEnviron 把变量合并进一份 os.Environ 风格的列表.
//
// 同名变量以 vars 为准, 其余保持原样; 返回的列表可直接赋给 cmd.Env.
func ApplyToEnviron(base []string, vars map[string]string) []string {
	if len(vars) == 0 {
		return base
	}

	// 先按 KEY= 建索引, 便于覆盖同名项而不是简单追加.
	index := make(map[string]int, len(base))
	for i, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		index[key] = i
	}

	result := append([]string(nil), base...)
	for key, value := range vars {
		entry := key + "=" + value
		if i, ok := index[key]; ok {
			result[i] = entry
			continue
		}
		result = append(result, entry)
	}
	return result
}

// CommandDisplay 把命令列表转成便于展示与日志的一行文本.
func CommandDisplay(command []string) string {
	if len(command) == 0 {
		return ""
	}
	return strings.Join(command, " ")
}

// ValidateCommand 检查导入命令是否可执行, 返回解析后的绝对路径.
//
// 只做存在性检查, 不真的运行: 运行由 Import 负责, 这里用于在设置保存时给出即时反馈.
func ValidateCommand(command []string) error {
	if len(command) == 0 {
		return nil
	}
	program := command[0]
	if !strings.ContainsRune(program, os.PathSeparator) {
		found, err := exec.LookPath(program)
		if err != nil {
			return fmt.Errorf("找不到命令 %q: %w", program, err)
		}
		program = found
	}
	info, err := os.Stat(program)
	if err != nil {
		return fmt.Errorf("无法访问 %s: %w", program, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s 是目录, 不是可执行文件", program)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s 没有可执行权限", program)
	}
	return nil
}

// ExpandHome 展开命令里以 ~ 开头的路径.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	// ~user 形式交给 shell 处理, 这里不动.
	return path
}

// SplitCommand 把用户填写的命令字符串切成 argv.
//
// 支持用空白分隔与用引号包裹含空格的参数, 例如:
//
//	fish -l -i -c "env -0"
//
// 不做变量展开与通配, 那些由被执行的 shell 自己负责.
// 引号未闭合时按剩余部分整体作为最后一个参数, 不报错: 用户可能正在输入.
func SplitCommand(raw string) []string {
	var (
		args    []string
		current strings.Builder
		quote   rune
		has     bool
	)
	for _, r := range raw {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			has = true
		case r == '"' || r == '\'':
			quote = r
			has = true
		case r == ' ' || r == '\t' || r == '\n':
			if has {
				args = append(args, current.String())
				current.Reset()
				has = false
			}
		default:
			current.WriteRune(r)
			has = true
		}
	}
	if has {
		args = append(args, current.String())
	}
	return args
}

// firstLine 取多行文本的第一行, 避免把整段错误塞进提示.
func firstLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}
