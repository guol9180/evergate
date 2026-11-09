// Package config 负责 EverGate 引导配置（evergate.yml）的加载与校验。
// YAML 仅做引导；业务数据一律存 DB。
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 是 evergate.yml 的完整结构。
type Config struct {
	Server   Server   `yaml:"server"`
	Storage  Storage  `yaml:"storage"`
	Security Security `yaml:"security"`
	Defaults Defaults `yaml:"defaults"`
}

// Server 是 HTTP 服务引导配置。
type Server struct {
	Port     int    `yaml:"port"`
	BasePath string `yaml:"base_path"`
}

// Storage 是存储引导配置。
type Storage struct {
	SQLitePath       string `yaml:"sqlite_path"`
	LogRetentionDays int    `yaml:"log_retention_days"`
}

// Security 是主密钥与会话密钥（为空则首次启动自动生成并写回，T2 实现）。
type Security struct {
	MasterKey     string `yaml:"master_key"`
	SessionSecret string `yaml:"session_secret"`
}

// Defaults 是调度与超时的全局默认值（渠道可覆盖）。
type Defaults struct {
	ConnectTimeout    Duration    `yaml:"connect_timeout"`
	FirstTokenTimeout Duration    `yaml:"first_token_timeout"`
	OverallTimeout    Duration    `yaml:"overall_timeout"`
	RetryBudget       RetryBudget `yaml:"retry_budget"`
	Cooldown          Cooldown    `yaml:"cooldown"`
}

// RetryBudget 是单请求重试预算。
type RetryBudget struct {
	Channels       int      `yaml:"channels"`
	KeysPerChannel int      `yaml:"keys_per_channel"`
	Total          Duration `yaml:"total"`
}

// Cooldown 是 key 冷却退避参数。
type Cooldown struct {
	FailThreshold int      `yaml:"fail_threshold"`
	Base          Duration `yaml:"base"`
	Max           Duration `yaml:"max"`
}

// Duration 包装 time.Duration，使 YAML 支持 "10s" / "30m" 这类写法。
type Duration struct {
	time.Duration
}

// UnmarshalYAML 解析 "10s" 风格的时间字符串。
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = v
	return nil
}

// MarshalYAML 供配置写回时序列化为 "10s" 风格（T2 使用）。
func (d Duration) MarshalYAML() (any, error) {
	return d.Duration.String(), nil
}

// Default 返回与 evergate.yml 模板一致的默认配置。
func Default() Config {
	return Config{
		Server:  Server{Port: 8787, BasePath: ""},
		Storage: Storage{SQLitePath: "./data/evergate.db", LogRetentionDays: 14},
		Defaults: Defaults{
			ConnectTimeout:    Duration{Duration: 10 * time.Second},
			FirstTokenTimeout: Duration{Duration: 30 * time.Second},
			OverallTimeout:    Duration{Duration: 300 * time.Second},
			RetryBudget:       RetryBudget{Channels: 3, KeysPerChannel: 2, Total: Duration{Duration: 60 * time.Second}},
			Cooldown:          Cooldown{FailThreshold: 3, Base: Duration{Duration: 30 * time.Second}, Max: Duration{Duration: 30 * time.Minute}},
		},
	}
}

// Load 读取 path 的 YAML 配置。文件不存在时返回默认值与 nil 错误（由调用方提示）；
// YAML 中缺省的字段保留默认值。
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("validate config %s: %w", path, err)
	}
	return cfg, nil
}

// Validate 尽早暴露非法配置，避免运行到一半才失败。
func (c Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port 必须在 1-65535，当前 %d", c.Server.Port)
	}
	if c.Server.BasePath != "" && (!strings.HasPrefix(c.Server.BasePath, "/") || strings.HasSuffix(c.Server.BasePath, "/")) {
		return fmt.Errorf("server.base_path 必须以 / 开头且不以 / 结尾，当前 %q", c.Server.BasePath)
	}
	if c.Storage.SQLitePath == "" {
		return errors.New("storage.sqlite_path 不能为空")
	}
	if c.Storage.LogRetentionDays < 1 {
		return fmt.Errorf("storage.log_retention_days 必须 >= 1，当前 %d", c.Storage.LogRetentionDays)
	}
	d := c.Defaults
	if d.ConnectTimeout.Duration <= 0 || d.FirstTokenTimeout.Duration <= 0 || d.OverallTimeout.Duration <= 0 {
		return errors.New("defaults 各超时必须 > 0")
	}
	if d.RetryBudget.Channels < 1 || d.RetryBudget.KeysPerChannel < 1 || d.RetryBudget.Total.Duration <= 0 {
		return errors.New("defaults.retry_budget 各字段必须为正值")
	}
	if d.Cooldown.FailThreshold < 1 || d.Cooldown.Base.Duration <= 0 || d.Cooldown.Max.Duration < d.Cooldown.Base.Duration {
		return errors.New("defaults.cooldown 非法：fail_threshold >= 1，base > 0，max >= base")
	}
	return nil
}
