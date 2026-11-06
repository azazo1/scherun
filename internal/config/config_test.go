package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadCreatesDefault 验证首次加载会生成默认配置文件.
func TestLoadCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Version != CurrentVersion {
		t.Fatalf("版本号应为 %d, 实际 %d", CurrentVersion, cfg.Version)
	}
	if cfg.LocalBind != DefaultBind {
		t.Fatalf("默认监听地址应为 %s, 实际 %s", DefaultBind, cfg.LocalBind)
	}
	if cfg.LogRetentionDays != DefaultLogRetentionDays {
		t.Fatalf("默认日志保留天数应为 %d, 实际 %d", DefaultLogRetentionDays, cfg.LogRetentionDays)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("配置文件未生成: %v", err)
	}
}

// TestMigrateFromVersionZero 验证缺少 version 字段的旧配置被补齐.
func TestMigrateFromVersionZero(t *testing.T) {
	raw := map[string]any{
		"quiet_launch":       true,
		"log_retention_days": int64(3),
	}

	cfg, err := Migrate(raw)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if cfg.Version != CurrentVersion {
		t.Fatalf("迁移后版本应为 %d, 实际 %d", CurrentVersion, cfg.Version)
	}
	if !cfg.QuietLaunch {
		t.Fatal("已有字段 quiet_launch 应被保留")
	}
	if cfg.LogRetentionDays != 3 {
		t.Fatalf("已有字段 log_retention_days 应被保留, 实际 %d", cfg.LogRetentionDays)
	}
	if cfg.LocalBind != DefaultBind {
		t.Fatalf("缺失的 local_bind 应被补齐为 %s, 实际 %s", DefaultBind, cfg.LocalBind)
	}
}

// TestMigrateRejectsFutureVersion 验证高版本配置被拒绝, 而不是静默降级.
func TestMigrateRejectsFutureVersion(t *testing.T) {
	raw := map[string]any{"version": int64(CurrentVersion + 1)}
	if _, err := Migrate(raw); err == nil {
		t.Fatal("高于当前版本的配置应当报错")
	}
}

// TestLoadNormalizesInvalidRetention 验证非法保留天数被修正为默认值.
func TestLoadNormalizesInvalidRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "version = 1\nlog_retention_days = 0\nlocal_bind = \"127.0.0.1:20008\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.LogRetentionDays != DefaultLogRetentionDays {
		t.Fatalf("非法保留天数应回退为 %d, 实际 %d", DefaultLogRetentionDays, cfg.LogRetentionDays)
	}
}

// TestLoadBacksUpBrokenConfig 验证损坏的配置被备份而不是让应用起不来.
func TestLoadBacksUpBrokenConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("这不是 toml {{{"), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	cfg, err := Load(path)
	if err == nil {
		t.Fatal("损坏的配置应当返回错误供调用方记录")
	}
	if cfg.LocalBind != DefaultBind {
		t.Fatalf("损坏时应当返回默认配置, 实际 %s", cfg.LocalBind)
	}
	if _, statErr := os.Stat(path + ".broken"); statErr != nil {
		t.Fatalf("原配置应被备份: %v", statErr)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("应重新生成可用的配置文件: %v", statErr)
	}
}

// TestUpdatePersists 验证保存后重新加载能读回新值.
func TestUpdatePersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if err := cfg.Update(Config{QuietLaunch: true, LogRetentionDays: 7, LocalBind: cfg.LocalBind}); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if !reloaded.QuietLaunch || reloaded.LogRetentionDays != 7 {
		t.Fatalf("配置未正确持久化: %+v", reloaded)
	}
}
