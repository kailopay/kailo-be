package platform

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
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
	Auth     AuthConfig
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

type AuthConfig struct {
	IssuerURL                string
	ClientID                 string
	ClientSecret             string
	RedirectURL              string
	EmailConnection          string
	SuccessRedirectURL       string
	TransactionEncryptionKey string
	SessionHMACKey           string
	SessionAbsoluteLifetime  time.Duration
	SessionIdleLifetime      time.Duration
	TransactionLifetime      time.Duration
	CookieName               string
	CookieSecure             bool
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
		Auth: AuthConfig{
			IssuerURL:                strings.TrimRight(strings.TrimSpace(v.GetString("auth0.issuer_url")), "/"),
			ClientID:                 strings.TrimSpace(v.GetString("auth0.client_id")),
			ClientSecret:             strings.TrimSpace(v.GetString("auth0.client_secret")),
			RedirectURL:              strings.TrimSpace(v.GetString("auth0.redirect_url")),
			EmailConnection:          strings.TrimSpace(v.GetString("auth0.email_connection")),
			SuccessRedirectURL:       strings.TrimSpace(v.GetString("auth.success_redirect_url")),
			TransactionEncryptionKey: strings.TrimSpace(v.GetString("auth.transaction_encryption_key")),
			SessionHMACKey:           strings.TrimSpace(v.GetString("auth.session_hmac_key")),
			SessionAbsoluteLifetime:  v.GetDuration("auth.session_absolute_lifetime"),
			SessionIdleLifetime:      v.GetDuration("auth.session_idle_lifetime"),
			TransactionLifetime:      v.GetDuration("auth.transaction_lifetime"),
			CookieName:               strings.TrimSpace(v.GetString("auth.cookie_name")),
			CookieSecure:             v.GetBool("auth.cookie_secure"),
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
	if err := c.Auth.Validate(c.App.Environment); err != nil {
		return err
	}
	return nil
}

func (c AuthConfig) Validate(environment string) error {
	if strings.TrimSpace(c.IssuerURL) == "" || strings.TrimSpace(c.ClientID) == "" ||
		strings.TrimSpace(c.ClientSecret) == "" {
		return errors.New("auth0 issuer and client credentials are required")
	}
	if strings.TrimSpace(c.EmailConnection) == "" {
		return errors.New("auth0 email connection is required")
	}
	for name, rawURL := range map[string]string{
		"auth0 issuer URL":          c.IssuerURL,
		"auth0 redirect URL":        c.RedirectURL,
		"auth success redirect URL": c.SuccessRedirectURL,
	} {
		if err := validateAuthURL(name, rawURL, environment); err != nil {
			return err
		}
	}
	if err := validateKey("auth transaction encryption key", c.TransactionEncryptionKey); err != nil {
		return err
	}
	if err := validateKey("auth session HMAC key", c.SessionHMACKey); err != nil {
		return err
	}
	if c.SessionAbsoluteLifetime <= 0 || c.SessionIdleLifetime <= 0 || c.TransactionLifetime <= 0 {
		return errors.New("auth durations must be positive")
	}
	if c.SessionIdleLifetime > c.SessionAbsoluteLifetime {
		return errors.New("auth session idle lifetime cannot exceed absolute lifetime")
	}
	if strings.TrimSpace(c.CookieName) == "" {
		return errors.New("auth cookie name is required")
	}
	if !strings.EqualFold(environment, "local") && !c.CookieSecure {
		return errors.New("auth cookie must be secure outside local")
	}
	return nil
}

func validateAuthURL(name, rawURL, environment string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.IsAbs() == false || parsed.Host == "" ||
		(parsed.Scheme != "https" && !(strings.EqualFold(environment, "local") && parsed.Scheme == "http")) {
		return fmt.Errorf("%s must be an absolute URL using HTTPS outside local", name)
	}
	return nil
}

func validateKey(name, encoded string) error {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("%s must be base64-encoded 32 bytes", name)
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
	v.SetDefault("auth.session_absolute_lifetime", 8*time.Hour)
	v.SetDefault("auth.session_idle_lifetime", 30*time.Minute)
	v.SetDefault("auth.transaction_lifetime", 10*time.Minute)
	v.SetDefault("auth.cookie_name", "kailopay_session")
	v.SetDefault("auth.cookie_secure", false)
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
		"app.environment":                 "APP_ENV",
		"app.version":                     "APP_VERSION",
		"http.address":                    "HTTP_ADDRESS",
		"http.read_header_timeout":        "HTTP_READ_HEADER_TIMEOUT",
		"http.read_timeout":               "HTTP_READ_TIMEOUT",
		"http.write_timeout":              "HTTP_WRITE_TIMEOUT",
		"http.idle_timeout":               "HTTP_IDLE_TIMEOUT",
		"http.shutdown_timeout":           "HTTP_SHUTDOWN_TIMEOUT",
		"health.check_timeout":            "HEALTH_CHECK_TIMEOUT",
		"database.dsn":                    "DATABASE_DSN",
		"database.max_open_conns":         "DATABASE_MAX_OPEN_CONNS",
		"database.max_idle_conns":         "DATABASE_MAX_IDLE_CONNS",
		"database.conn_max_lifetime":      "DATABASE_CONN_MAX_LIFETIME",
		"database.conn_max_idle_time":     "DATABASE_CONN_MAX_IDLE_TIME",
		"database.ping_timeout":           "DATABASE_PING_TIMEOUT",
		"logging.level":                   "LOG_LEVEL",
		"logging.format":                  "LOG_FORMAT",
		"auth0.issuer_url":                "AUTH0_ISSUER_URL",
		"auth0.client_id":                 "AUTH0_CLIENT_ID",
		"auth0.client_secret":             "AUTH0_CLIENT_SECRET",
		"auth0.redirect_url":              "AUTH0_REDIRECT_URL",
		"auth0.email_connection":          "AUTH0_EMAIL_CONNECTION",
		"auth.success_redirect_url":       "AUTH_SUCCESS_REDIRECT_URL",
		"auth.transaction_encryption_key": "AUTH_TRANSACTION_ENCRYPTION_KEY",
		"auth.session_hmac_key":           "AUTH_SESSION_HMAC_KEY",
		"auth.session_absolute_lifetime":  "AUTH_SESSION_ABSOLUTE_LIFETIME",
		"auth.session_idle_lifetime":      "AUTH_SESSION_IDLE_LIFETIME",
		"auth.transaction_lifetime":       "AUTH_TRANSACTION_LIFETIME",
		"auth.cookie_name":                "AUTH_COOKIE_NAME",
		"auth.cookie_secure":              "AUTH_COOKIE_SECURE",
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
