package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Settings struct {
	Profile      string
	Server       string
	MonitorURL   string
	Credentials  string
	Token        string
	AuthorityDir string
	OperatorSeed string
	Refresh      time.Duration
}

func Configure(v *viper.Viper, configFile string) error {
	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("find user config directory: %w", err)
		}
		v.AddConfigPath(filepath.Join(configDir, "nats-tui"))
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	v.SetEnvPrefix("NATS_TUI")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("read config: %w", err)
		}
	}
	return nil
}

func Load(v *viper.Viper) Settings {
	return Settings{
		Profile:      v.GetString("profile"),
		Server:       v.GetString("server"),
		MonitorURL:   v.GetString("monitor-url"),
		Credentials:  v.GetString("credentials"),
		Token:        v.GetString("token"),
		AuthorityDir: v.GetString("authority-dir"),
		OperatorSeed: v.GetString("authority-operator-seed"),
		Refresh:      v.GetDuration("refresh"),
	}
}
