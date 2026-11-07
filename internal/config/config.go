// Package config 管理本地配置：每个服务一份，互不干扰。
//
// 目录固定在 $XDG_CONFIG_HOME 或 ~/.config 下，不用 os.UserConfigDir()：
// 它在 macOS 上返回 ~/Library/Application Support，会让配置位置随平台漂移。
//
// OpenList 沿用旧路径 ~/.config/openlist-cli/config.json，避免已登录用户失效；
// 新服务使用 ~/.config/nst/<服务>/config.json。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nst/internal/jsonx"
)

// ConfigError 表示配置文件损坏或无法写入。
type ConfigError struct {
	Msg string
}

func (e *ConfigError) Error() string { return e.Msg }

func configErrorf(format string, args ...any) *ConfigError {
	return &ConfigError{Msg: fmt.Sprintf(format, args...)}
}

// Store 是单个服务的配置读写器。零值不可用，请用 OpenList 或 New 构造。
type Store struct {
	Service  string
	DirName  string
	EnvURL   string
	EnvToken string
}

// OpenList 返回 OpenList 的配置读写器，沿用旧目录与环境变量名。
func OpenList() *Store {
	return &Store{
		Service:  "openlist",
		DirName:  "openlist-cli",
		EnvURL:   "OPENLIST_URL",
		EnvToken: "OPENLIST_TOKEN",
	}
}

// New 返回新服务的配置读写器：目录 nst/<服务>，环境变量 NST_<服务>_URL/TOKEN。
func New(service string) *Store {
	name := strings.TrimSpace(service)
	upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return &Store{
		Service:  name,
		DirName:  filepath.Join("nst", name),
		EnvURL:   "NST_" + upper + "_URL",
		EnvToken: "NST_" + upper + "_TOKEN",
	}
}

func baseDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".config")
}

// Dir 返回该服务的配置目录。
func (s *Store) Dir() string { return filepath.Join(baseDir(), s.DirName) }

// File 返回该服务配置文件的完整路径。
func (s *Store) File() string { return filepath.Join(s.Dir(), "config.json") }

// Load 读取配置；环境变量优先于文件。文件不存在不算错误。
func (s *Store) Load() (map[string]any, error) {
	cfg := map[string]any{}
	path := s.File()
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

	if v := os.Getenv(s.EnvURL); v != "" {
		cfg["url"] = v
	}
	if v := os.Getenv(s.EnvToken); v != "" {
		cfg["token"] = v
	}
	return cfg, nil
}

// Save 以 0600 权限原子写入配置，返回写到的路径。
func (s *Store) Save(cfg map[string]any) (string, error) {
	dir := s.Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", configErrorf("配置目录无法创建：%s（%v）", dir, err)
	}
	body, err := jsonx.Encode(cfg)
	if err != nil {
		return "", configErrorf("配置无法序列化：%v", err)
	}
	body = append(body, '\n')

	target := s.File()
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
func (s *Store) ClearToken() error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}
	delete(cfg, "token")
	_, err = s.Save(cfg)
	return err
}

// Str 读字符串键。
func Str(cfg map[string]any, key string) string { return jsonx.Str(cfg, key) }
