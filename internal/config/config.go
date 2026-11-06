// Package config 负责配置文件的读写与版本迁移.
//
// 配置文件带显式的 version 字段, 每次新增字段都追加一个迁移函数,
// 这样旧配置文件能被自动补齐, 而不是让程序带着缺失字段跑.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/azazo1/scherun/internal/appdirs"
)

// CurrentVersion 是当前配置文件结构版本.
const CurrentVersion = 1

// DefaultBind 是本地 HTTP 服务的默认监听地址.
const DefaultBind = "127.0.0.1:20008"

// DefaultLogRetentionDays 是日志默认保留天数.
const DefaultLogRetentionDays = 5

// Config 是应用配置.
type Config struct {
	// Version 是配置文件结构版本, 用于迁移.
	Version int `toml:"version"`

	// QuietLaunch 为真时启动不自动打开浏览器.
	QuietLaunch bool `toml:"quiet_launch"`

	// LogRetentionDays 是日志保留天数, 最小 1.
	LogRetentionDays int `toml:"log_retention_days"`

	// LocalBind 是本地 HTTP 监听地址.
	LocalBind string `toml:"local_bind"`

	// path 记录配置来源文件, 不参与序列化.
	path string
}

// Default 返回默认配置.
func Default() Config {
	return Config{
		Version:          CurrentVersion,
		QuietLaunch:      false,
		LogRetentionDays: DefaultLogRetentionDays,
		LocalBind:        DefaultBind,
	}
}

// Load 读取配置文件; 文件不存在时生成默认配置并落盘.
func Load(path string) (Config, error) {
	cfg := Default()
	cfg.path = path

	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := cfg.Save(); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("读取配置 %s 失败: %w", path, err)
	}

	// 先解析成通用 map, 判断版本后再迁移.
	raw := map[string]any{}
	if strings.TrimSpace(string(content)) != "" {
		if err := toml.Unmarshal(content, &raw); err != nil {
			// 配置损坏时备份原文件, 用默认配置继续运行, 避免应用直接起不来.
			backup := path + ".broken"
			if renameErr := os.Rename(path, backup); renameErr != nil {
				return cfg, fmt.Errorf("配置 %s 解析失败且无法备份: %w", path, err)
			}
			if saveErr := cfg.Save(); saveErr != nil {
				return cfg, saveErr
			}
			return cfg, fmt.Errorf("配置 %s 解析失败, 已备份为 %s: %w", path, backup, err)
		}
	}

	migrated, err := Migrate(raw)
	if err != nil {
		return cfg, err
	}
	migrated.path = path
	cfg = migrated

	// 迁移或字段补齐后回写, 保证文件与内存一致.
	if err := cfg.Save(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// LoadFromDefaultPath 从默认路径读取配置.
func LoadFromDefaultPath() (Config, error) {
	path, err := appdirs.ConfigPath()
	if err != nil {
		return Default(), err
	}
	return Load(path)
}

// Migrate 把任意版本的原始配置迁移到当前版本.
//
// 缺 version 字段视为 v0, 按顺序补齐默认值后升到 CurrentVersion.
func Migrate(raw map[string]any) (Config, error) {
	cfg := Default()

	version := 0
	if v, ok := raw["version"]; ok {
		switch typed := v.(type) {
		case int64:
			version = int(typed)
		case int:
			version = typed
		default:
			return cfg, fmt.Errorf("配置字段 version 类型不正确: %T", v)
		}
	}
	if version > CurrentVersion {
		return cfg, fmt.Errorf("配置文件版本 %d 高于程序支持的 %d, 请升级 scherun", version, CurrentVersion)
	}

	// v0 -> v1: 早期配置没有任何 version 字段, 也缺少 local_bind.
	if version < 1 {
		if _, ok := raw["local_bind"]; !ok {
			raw["local_bind"] = DefaultBind
		}
		version = 1
	}

	// 用 TOML 往返把 map 解析成结构体, 未出现的字段保留默认值.
	buf, err := toml.Marshal(raw)
	if err != nil {
		return cfg, fmt.Errorf("配置迁移序列化失败: %w", err)
	}
	parsed := Default()
	if err := toml.Unmarshal(buf, &parsed); err != nil {
		return cfg, fmt.Errorf("配置迁移解析失败: %w", err)
	}
	parsed.Version = CurrentVersion
	parsed.normalize()
	return parsed, nil
}

// normalize 修正非法取值.
func (c *Config) normalize() {
	if c.LogRetentionDays < 1 {
		c.LogRetentionDays = DefaultLogRetentionDays
	}
	if strings.TrimSpace(c.LocalBind) == "" {
		c.LocalBind = DefaultBind
	}
}

// Validate 校验配置取值.
func (c Config) Validate() error {
	if c.LogRetentionDays < 1 {
		return fmt.Errorf("log_retention_days 必须大于等于 1")
	}
	if !strings.Contains(c.LocalBind, ":") {
		return fmt.Errorf("local_bind 必须是 host:port 形式, 当前为 %q", c.LocalBind)
	}
	return nil
}

// Save 把配置写回来源文件; 未绑定文件时返回错误.
func (c *Config) Save() error {
	if c.path == "" {
		return fmt.Errorf("配置未绑定文件路径, 无法保存")
	}
	c.Version = CurrentVersion
	c.normalize()

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建配置目录 %s 失败: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return fmt.Errorf("创建临时配置文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	encoder := toml.NewEncoder(tmp)
	if err := encoder.Encode(c); err != nil {
		tmp.Close()
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时配置文件失败: %w", err)
	}
	if err := os.Rename(tmpName, c.path); err != nil {
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	return nil
}

// Path 返回配置来源路径.
func (c Config) Path() string { return c.path }

// Update 用新值覆盖可修改字段并保存.
//
// 只接受用户可改的字段, version 与文件路径保持不变.
func (c *Config) Update(next Config) error {
	c.QuietLaunch = next.QuietLaunch
	c.LogRetentionDays = next.LogRetentionDays
	if strings.TrimSpace(next.LocalBind) != "" {
		c.LocalBind = next.LocalBind
	}
	c.normalize()
	if err := c.Validate(); err != nil {
		return err
	}
	return c.Save()
}
