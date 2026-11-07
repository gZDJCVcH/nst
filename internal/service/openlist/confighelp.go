package openlist

import "nst/internal/config"

func loadConfig() (map[string]any, error) { return store.Load() }

func saveConfig(cfg map[string]any) (string, error) { return store.Save(cfg) }

func configFile() string { return store.File() }

// loadConfigMasked 读配置并把令牌打码，供 config show 使用。
func loadConfigMasked() (map[string]any, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}
	if token := config.Str(cfg, "token"); token != "" {
		r := []rune(token)
		if len(r) > 24 {
			cfg["token"] = string(r[:12]) + "…" + string(r[len(r)-6:])
		} else {
			cfg["token"] = "***"
		}
	}
	return cfg, nil
}
