// Package appdirs 解析 scherun 使用的各类目录.
//
// 目录划分遵循平台习惯: 配置放用户配置目录, 数据与日志放用户数据目录.
// 环境变量可以覆盖, 供 just debug 之类需要隔离数据的场景使用.
package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "scherun"

// 环境变量名, 供调试与测试隔离.
const (
	envConfigDir = "SCHERUN_CONFIG_DIR"
	envDataDir   = "SCHERUN_DATA_DIR"
	envLogFile   = "SCHERUN_LOG_FILE"
)

// ConfigDir 返回配置目录并确保存在.
func ConfigDir() (string, error) {
	if dir := os.Getenv(envConfigDir); dir != "" {
		return ensure(dir)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户配置目录: %w", err)
	}
	return ensure(filepath.Join(base, appName))
}

// DataDir 返回数据目录并确保存在.
//
// Linux 与 macOS 都走 os.UserConfigDir 之外的语义, 这里显式区分平台:
// Linux 用 XDG_DATA_HOME, Windows 用 LOCALAPPDATA, macOS 用 ~/Library/Application Support.
func DataDir() (string, error) {
	if dir := os.Getenv(envDataDir); dir != "" {
		return ensure(dir)
	}
	switch runtime.GOOS {
	case "windows":
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return ensure(filepath.Join(base, appName))
		}
	case "linux", "freebsd", "openbsd", "netbsd":
		if base := os.Getenv("XDG_DATA_HOME"); base != "" {
			return ensure(filepath.Join(base, appName))
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("无法确定家目录: %w", err)
		}
		return ensure(filepath.Join(home, ".local", "share", appName))
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户数据目录: %w", err)
	}
	return ensure(filepath.Join(base, appName))
}

// LogDir 返回日志目录并确保存在.
func LogDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return ensure(filepath.Join(dir, "logs"))
}

// LogFile 返回主日志文件路径.
func LogFile() (string, error) {
	if file := os.Getenv(envLogFile); file != "" {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return "", fmt.Errorf("创建日志目录失败: %w", err)
		}
		return file, nil
	}
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scherun.log"), nil
}

// ConfigPath 返回配置文件路径.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// DatabasePath 返回 SQLite 数据库路径.
func DatabasePath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scherun.db"), nil
}

// RuntimePath 返回运行状态文件路径.
func RuntimePath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "runtime.json"), nil
}

// SecretPath 返回 cookie 签名密钥路径.
func SecretPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.secret"), nil
}

// LockPath 返回单实例锁文件路径.
//
// 锁按数据目录绑定, 因此 just debug 的隔离实例不会与日常实例互相阻塞.
func LockPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scherun.lock"), nil
}

func ensure(path string) (string, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("创建目录 %s 失败: %w", path, err)
	}
	return path, nil
}
