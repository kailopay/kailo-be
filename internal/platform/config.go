package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Health   HealthConfig
	Database DatabaseConfig
	Logging  LoggingConfig
}

type AppConfig struct {
	Environment string
	Version     string
}

type HTTPConfig struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type HealthConfig struct {
	CheckTimeout time.Duration
}

type DatabaseConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

type LoggingConfig struct {
	Level  string
	Format string
}

func Load() (Config, error) {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)
	if err := readConfigFile(v); err != nil {
		return Config{}, err
	}
	bindEnvironment(v)

	cfg := Config{
		App: AppConfig{
			Environment: strings.ToLower(strings.TrimSpace(v.GetString("app.environment"))),
			Version:     strings.TrimSpace(v.GetString("app.version")),
		},
		HTTP: HTTPConfig{
			Address:           v.GetString("http.address"),
			ReadHeaderTimeout: v.GetDuration("http.read_header_timeout"),
			ReadTimeout:       v.GetDuration("http.read_timeout"),
			WriteTimeout:      v.GetDuration("http.write_timeout"),
			IdleTimeout:       v.GetDuration("http.idle_timeout"),
			ShutdownTimeout:   v.GetDuration("http.shutdown_timeout"),
		},
		Health: HealthConfig{
			CheckTimeout: v.GetDuration("health.check_timeout"),
		},
		Database: DatabaseConfig{
			DSN:             v.GetString("database.dsn"),
			MaxOpenConns:    v.GetInt("database.max_open_conns"),
			MaxIdleConns:    v.GetInt("database.max_idle_conns"),
			ConnMaxLifetime: v.GetDuration("database.conn_max_lifetime"),
			ConnMaxIdleTime: v.GetDuration("database.conn_max_idle_time"),
			PingTimeout:     v.GetDuration("database.ping_timeout"),
		},
		Logging: LoggingConfig{
			Level:  strings.ToLower(strings.TrimSpace(v.GetString("logging.level"))),
			Format: strings.ToLower(strings.TrimSpace(v.GetString("logging.format"))),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.App.Environment == "" {
		return errors.New("app environment is required")
	}
	if c.App.Version == "" {
		return errors.New("app version is required")
	}
	if c.HTTP.Address == "" {
		return errors.New("http address is required")
	}
	if c.HTTP.ReadHeaderTimeout <= 0 || c.HTTP.ReadTimeout <= 0 ||
		c.HTTP.WriteTimeout <= 0 || c.HTTP.IdleTimeout <= 0 ||
		c.HTTP.ShutdownTimeout <= 0 {
		return errors.New("http timeouts must be positive")
	}
	if c.Health.CheckTimeout <= 0 {
		return errors.New("health check timeout must be positive")
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		return errors.New("database DSN is required")
	}
	if c.Database.MaxOpenConns <= 0 || c.Database.MaxIdleConns < 0 ||
		c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return errors.New("database connection pool limits are invalid")
	}
	if c.Database.ConnMaxLifetime <= 0 || c.Database.ConnMaxIdleTime <= 0 ||
		c.Database.PingTimeout <= 0 {
		return errors.New("database durations must be positive")
	}
	if !validLogLevel(c.Logging.Level) {
		return fmt.Errorf("unsupported log level %q", c.Logging.Level)
	}
	if c.Logging.Format != "json" && c.Logging.Format != "text" {
		return fmt.Errorf("unsupported log format %q", c.Logging.Format)
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.environment", "local")
	v.SetDefault("app.version", "dev")
	v.SetDefault("http.address", ":8080")
	v.SetDefault("http.read_header_timeout", 5*time.Second)
	v.SetDefault("http.read_timeout", 15*time.Second)
	v.SetDefault("http.write_timeout", 15*time.Second)
	v.SetDefault("http.idle_timeout", 60*time.Second)
	v.SetDefault("http.shutdown_timeout", 10*time.Second)
	v.SetDefault("health.check_timeout", 2*time.Second)
	v.SetDefault("database.max_open_conns", 10)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 30*time.Minute)
	v.SetDefault("database.conn_max_idle_time", 5*time.Minute)
	v.SetDefault("database.ping_timeout", 3*time.Second)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
}

func bindEnvironment(v *viper.Viper) {
	for key, env := range environmentBindings() {
		if _, ok := os.LookupEnv(env); ok {
			_ = v.BindEnv(key, env)
		}
	}
}

func environmentBindings() map[string]string {
	return map[string]string{
		"app.environment":             "APP_ENV",
		"app.version":                 "APP_VERSION",
		"http.address":                "HTTP_ADDRESS",
		"http.read_header_timeout":    "HTTP_READ_HEADER_TIMEOUT",
		"http.read_timeout":           "HTTP_READ_TIMEOUT",
		"http.write_timeout":          "HTTP_WRITE_TIMEOUT",
		"http.idle_timeout":           "HTTP_IDLE_TIMEOUT",
		"http.shutdown_timeout":       "HTTP_SHUTDOWN_TIMEOUT",
		"health.check_timeout":        "HEALTH_CHECK_TIMEOUT",
		"database.dsn":                "DATABASE_DSN",
		"database.max_open_conns":     "DATABASE_MAX_OPEN_CONNS",
		"database.max_idle_conns":     "DATABASE_MAX_IDLE_CONNS",
		"database.conn_max_lifetime":  "DATABASE_CONN_MAX_LIFETIME",
		"database.conn_max_idle_time": "DATABASE_CONN_MAX_IDLE_TIME",
		"database.ping_timeout":       "DATABASE_PING_TIMEOUT",
		"logging.level":               "LOG_LEVEL",
		"logging.format":              "LOG_FORMAT",
	}
}

func readConfigFile(v *viper.Viper) error {
	path := os.Getenv("CONFIG_FILE")
	if path == "" {
		if _, err := os.Stat(".env"); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("checking .env: %w", err)
		}
		path = ".env"
	}

	v.SetConfigFile(path)
	if strings.EqualFold(path, ".env") || strings.HasSuffix(strings.ToLower(path), ".env") {
		v.SetConfigType("env")
	}
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) && os.Getenv("CONFIG_FILE") == "" {
			return nil
		}
		return fmt.Errorf("reading config file %q: %w", path, err)
	}

	// Dotenv keys are uppercase while application settings use canonical dotted
	// keys. Copy only known settings into the canonical namespace.
	for key, env := range environmentBindings() {
		if value := v.Get(env); value != nil {
			if text, ok := value.(string); ok && text == "" {
				continue
			}
			v.Set(key, value)
		}
	}
	return nil
}

func validLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}
