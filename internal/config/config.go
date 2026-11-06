// Package config 管理本地配置：服务地址与令牌。
//
// 配置固定放在 $XDG_CONFIG_HOME/openlist-cli/config.json，缺省
// ~/.config/openlist-cli/config.json。不用 os.UserConfigDir()：它在 macOS 上
// 返回 ~/Library/Application Support，会让配置位置随平台漂移。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"nst/internal/jsonx"
)

const (
	EnvURL   = "OPENLIST_URL"
	EnvToken = "OPENLIST_TOKEN"
)

// ConfigError 表示配置文件损坏或无法写入。
type ConfigError struct {
	Msg string
}

func (e *ConfigError) Error() string { return e.Msg }

func configErrorf(format string, args ...any) *ConfigError {
	return &ConfigError{Msg: fmt.Sprintf(format, args...)}
}

// Dir 返回配置目录。
func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "openlist-cli")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".config", "openlist-cli")
}

// File 返回配置文件完整路径。
func File() string { return filepath.Join(Dir(), "config.json") }

// Load 读取配置；环境变量优先于文件。文件不存在不算错误。
func Load() (map[string]any, error) {
	cfg := map[string]any{}
	path := File()
	if data, err := os.ReadFile(path); err == nil {
		v, err := jsonx.Decode(data)
		if err != nil {
			return nil, configErrorf("配置文件损坏：%s（%v）", path, err)
		}
		if m := jsonx.Object(v); m != nil {
			cfg = m
		}
	} else if !os.IsNotExist(err) {
		return nil, configErrorf("配置文件无法读取：%s（%v）", path, err)
	}

	if v := os.Getenv(EnvURL); v != "" {
		cfg["url"] = v
	}
	if v := os.Getenv(EnvToken); v != "" {
		cfg["token"] = v
	}
	return cfg, nil
}

// Save 以 0600 权限原子写入配置，返回写到的路径。
func Save(cfg map[string]any) (string, error) {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", configErrorf("配置目录无法创建：%s（%v）", dir, err)
	}
	body, err := jsonx.Encode(cfg)
	if err != nil {
		return "", configErrorf("配置无法序列化：%v", err)
	}
	body = append(body, '\n')

	target := File()
	tmp := target + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", configErrorf("配置无法写入：%s（%v）", tmp, err)
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", configErrorf("配置无法写入：%s（%v）", tmp, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", configErrorf("配置无法写入：%s（%v）", tmp, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return "", configErrorf("配置无法落盘：%s（%v）", target, err)
	}
	return target, nil
}

// ClearToken 删除已保存的令牌。
func ClearToken() error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	delete(cfg, "token")
	_, err = Save(cfg)
	return err
}

// Str 读字符串键。
func Str(cfg map[string]any, key string) string { return jsonx.Str(cfg, key) }
