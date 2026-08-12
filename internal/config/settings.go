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
	Profile       string
	Server        string
	MonitorURL    string
	Credentials   string
	Token         string
	Username      string
	Password      string
	NKeySeed      string
	UserJWT       string
	UserSeed      string
	TLSCA         string
	TLSCert       string
	TLSKey        string
	TLSServerName string
	TLSInsecure   bool
	AuthorityDir  string
	OperatorSeed  string
	Refresh       time.Duration
	Mouse         bool
	Theme         string
	KeyQuit       string
	KeyHelp       string
	KeyRefresh    string
	KeyCommands   string
	KeyBookmarks  string
	KeyActivity   string
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
		Profile:       v.GetString("profile"),
		Server:        v.GetString("server"),
		MonitorURL:    v.GetString("monitor-url"),
		Credentials:   v.GetString("credentials"),
		Token:         v.GetString("token"),
		Username:      v.GetString("username"),
		Password:      v.GetString("password"),
		NKeySeed:      v.GetString("nkey-seed"),
		UserJWT:       v.GetString("user-jwt"),
		UserSeed:      v.GetString("user-seed"),
		TLSCA:         v.GetString("tls-ca"),
		TLSCert:       v.GetString("tls-cert"),
		TLSKey:        v.GetString("tls-key"),
		TLSServerName: v.GetString("tls-server-name"),
		TLSInsecure:   v.GetBool("tls-insecure"),
		AuthorityDir:  v.GetString("authority-dir"),
		OperatorSeed:  v.GetString("authority-operator-seed"),
		Refresh:       v.GetDuration("refresh"),
		Mouse:         v.GetBool("mouse"),
		Theme:         v.GetString("theme"),
		KeyQuit:       v.GetString("key-quit"),
		KeyHelp:       v.GetString("key-help"),
		KeyRefresh:    v.GetString("key-refresh"),
		KeyCommands:   v.GetString("key-commands"),
		KeyBookmarks:  v.GetString("key-bookmarks"),
		KeyActivity:   v.GetString("key-activity"),
	}
}
