package config

import (
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Cache  CacheConfig  `mapstructure:"cache"`
	SQLite SQLiteConfig `mapstructure:"sqlite"`
	Queue  QueueConfig  `mapstructure:"queue"`
	Slims    SlimsConfig    `mapstructure:"slims"`
	SlimsAPI SlimsAPIConfig `mapstructure:"slims_api"`
	Library  LibraryConfig  `mapstructure:"library"`
	KohaAPI  KohaAPIConfig  `mapstructure:"koha_api"`
	Log      LogConfig      `mapstructure:"log"`
	Web      WebConfig      `mapstructure:"web"`
}

// WebConfig controls dashboard serving. Empty Dir serves the copy embedded in
// the binary; set it to a folder (e.g. "web/dist") to serve from disk instead.
type WebConfig struct {
	Dir string `mapstructure:"dir"`
}

type ServerConfig struct {
	Address string `mapstructure:"address"`
	Port    int    `mapstructure:"port"`
}

type CacheConfig struct {
	MemoryMaxItems int64 `mapstructure:"memory_max_items"`
	MemoryTTLHours int   `mapstructure:"memory_ttl_hours"`
}

type SQLiteConfig struct {
	Path string `mapstructure:"path"`
}

type QueueConfig struct {
	Size int `mapstructure:"size"`
}

type SlimsConfig struct {
	MySQLDSN string `mapstructure:"mysql_dsn"`
	Enabled  bool   `mapstructure:"enabled"`
}

type SlimsAPIConfig struct {
	BaseURL           string `mapstructure:"base_url"`
	Token             string `mapstructure:"token"`
	SyncIntervalHours int    `mapstructure:"sync_interval_hours"`
}

// LibraryConfig selects the request-time lookup backend: "slims" (default,
// direct MySQL) or "koha" (RfidGateApi plugin over HTTP).
type LibraryConfig struct {
	Backend string `mapstructure:"backend"`
}

// KohaAPIConfig points at the Koha RfidGateApi plugin namespace root, e.g.
// http://localhost:8001/api/v1/contrib/rfid_gate. APIKey is sent as X-API-Key.
type KohaAPIConfig struct {
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`

	// Offline mirror sync (GET /rfid/status). All optional; see mirror.Config.
	SyncIntervalSeconds int `mapstructure:"sync_interval_seconds"` // delta sync period, default 60
	FullSyncHours       int `mapstructure:"full_sync_hours"`       // max age of last full sync, default 6
	OverlapMinutes      int `mapstructure:"overlap_minutes"`       // delta overlap window, default 15

	// Title/cover (biblio) sync (GET /rfid/biblio).
	BiblioSyncIntervalMinutes int `mapstructure:"biblio_sync_interval_minutes"` // default 10
	BiblioFullSyncHours       int `mapstructure:"biblio_full_sync_hours"`       // default 24
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

// Load reads the configuration. Pass the executable's directory as extraPaths
// so config.yaml is found when the binary is run from a different working
// directory (e.g. as a Windows service).
func Load(extraPaths ...string) (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./internal/config")
	for _, p := range extraPaths {
		if p != "" {
			viper.AddConfigPath(p)
		}
	}

	setDefaults()

	viper.SetEnvPrefix("RFID")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Resolve a relative SQLite path relative to where config.yaml was found,
	// not the process working directory, so the service always uses the right file.
	if !filepath.IsAbs(cfg.SQLite.Path) && viper.ConfigFileUsed() != "" {
		cfg.SQLite.Path = filepath.Join(
			filepath.Dir(viper.ConfigFileUsed()),
			cfg.SQLite.Path,
		)
	}

	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("server.address", "127.0.0.1")
	viper.SetDefault("server.port", 8080)

	viper.SetDefault("cache.memory_max_items", 100000)
	viper.SetDefault("cache.memory_ttl_hours", 24)

	viper.SetDefault("queue.size", 1000)

	viper.SetDefault("log.level", "info")

	viper.SetDefault("slims_api.base_url", "")
	viper.SetDefault("slims_api.token", "")
	viper.SetDefault("slims_api.sync_interval_hours", 6)

	viper.SetDefault("web.dir", "")

	viper.SetDefault("library.backend", "slims")
	viper.SetDefault("koha_api.base_url", "")
	viper.SetDefault("koha_api.api_key", "")
	viper.SetDefault("koha_api.sync_interval_seconds", 60)
	viper.SetDefault("koha_api.full_sync_hours", 6)
	viper.SetDefault("koha_api.overlap_minutes", 15)
	viper.SetDefault("koha_api.biblio_sync_interval_minutes", 10)
	viper.SetDefault("koha_api.biblio_full_sync_hours", 24)
}
