package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Gemini   GeminiConfig   `mapstructure:"gemini"`
	Privacy  PrivacyConfig  `mapstructure:"privacy"`
	Collector CollectorConfig `mapstructure:"collector"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type GeminiConfig struct {
	APIKey      string  `mapstructure:"api_key"`
	ModelName   string  `mapstructure:"model_name"`
	MaxDailyCost float64 `mapstructure:"max_daily_cost"`
}

type PrivacyConfig struct {
	MaskIPs    bool `mapstructure:"mask_ips"`
	MaskEmails bool `mapstructure:"mask_emails"`
	MaskUsernames bool `mapstructure:"mask_usernames"`
}

type CollectorConfig struct {
	PollIntervalSeconds int `mapstructure:"poll_interval_seconds"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("database.path", "sys-sentient.db")
	v.SetDefault("gemini.model_name", "gemini-1.5-flash")
	v.SetDefault("gemini.max_daily_cost", 1.0)
	v.SetDefault("privacy.mask_ips", true)
	v.SetDefault("privacy.mask_emails", true)
	v.SetDefault("privacy.mask_usernames", true)
	v.SetDefault("collector.poll_interval_seconds", 2)

	// Environment variables
	v.SetEnvPrefix("SYS_SENTIENT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Config file
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/sys-sentient/")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is fine, we use defaults/env
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	return &cfg, nil
}
