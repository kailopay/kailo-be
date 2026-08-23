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
	App           AppConfig
	HTTP          HTTPConfig
	Health        HealthConfig
	Database      DatabaseConfig
	Logging       LoggingConfig
	Auth          AuthConfig
	ObjectStorage ObjectStorageConfig
	Week1         Week1Config
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

type ObjectStorageConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

type AuthConfig struct {
	Google                   GoogleConfig
	EmailLinkBaseURL         string
	SuccessRedirectURL       string
	TransactionEncryptionKey string
	SessionHMACKey           string
	SessionAbsoluteLifetime  time.Duration
	SessionIdleLifetime      time.Duration
	TransactionLifetime      time.Duration
	CookieName               string
	CookieSecure             bool
	AvatarMaxBytes           int64
}

// GoogleConfig holds the optional Google OIDC application. Empty credentials
// disable Google sign-in.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func Load() (Config, error) {
	v, err := loadSettings()
	if err != nil {
		return Config{}, err
	}

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
			Google: GoogleConfig{
				ClientID:     strings.TrimSpace(v.GetString("google.client_id")),
				ClientSecret: strings.TrimSpace(v.GetString("google.client_secret")),
				RedirectURL:  strings.TrimSpace(v.GetString("google.redirect_url")),
			},
			EmailLinkBaseURL:         strings.TrimSpace(v.GetString("auth.email_link_base_url")),
			SuccessRedirectURL:       strings.TrimSpace(v.GetString("auth.success_redirect_url")),
			TransactionEncryptionKey: strings.TrimSpace(v.GetString("auth.transaction_encryption_key")),
			SessionHMACKey:           strings.TrimSpace(v.GetString("auth.session_hmac_key")),
			SessionAbsoluteLifetime:  v.GetDuration("auth.session_absolute_lifetime"),
			SessionIdleLifetime:      v.GetDuration("auth.session_idle_lifetime"),
			TransactionLifetime:      v.GetDuration("auth.transaction_lifetime"),
			CookieName:               strings.TrimSpace(v.GetString("auth.cookie_name")),
			CookieSecure:             v.GetBool("auth.cookie_secure"),
			AvatarMaxBytes:           v.GetInt64("auth.avatar_max_bytes"),
		},
		ObjectStorage: ObjectStorageConfig{
			Endpoint:  strings.TrimSpace(v.GetString("minio.endpoint")),
			AccessKey: strings.TrimSpace(v.GetString("minio.access_key")),
			SecretKey: strings.TrimSpace(v.GetString("minio.secret_key")),
			Bucket:    strings.TrimSpace(v.GetString("minio.bucket")),
			Region:    strings.TrimSpace(v.GetString("minio.region")),
			UseSSL:    v.GetBool("minio.use_ssl"),
		},
		Week1: Week1Config{
			APIKeyPepper: v.GetString("api_key.pepper"),
			Onramp: OnrampConfig{
				QuoteTTL:       v.GetDuration("onramp.quote_ttl"),
				QuoteMaxAge:    v.GetDuration("onramp.quote_max_age"),
				QuoteSpreadBPS: v.GetInt("onramp.quote_spread_bps"),
				MinIDR:         v.GetInt64("onramp.min_idr"),
				MaxIDR:         v.GetInt64("onramp.max_idr"),
			},
			CoinMarketCap: CoinMarketCapConfig{
				BaseURL: strings.TrimRight(v.GetString("coinmarketcap.base_url"), "/"),
				APIKey:  v.GetString("coinmarketcap.api_key"),
				Timeout: v.GetDuration("coinmarketcap.timeout"),
			},
			Xendit: XenditConfig{
				BaseURL:       strings.TrimRight(v.GetString("xendit.base_url"), "/"),
				SecretKey:     v.GetString("xendit.secret_key"),
				CallbackToken: v.GetString("xendit.callback_token"),
				APIVersion:    v.GetString("xendit.api_version"),
				QRISChannel:   v.GetString("xendit.qris_channel"),
				VAChannel:     v.GetString("xendit.va_channel"),
				Timeout:       v.GetDuration("xendit.timeout"),
			},
			Stellar: StellarConfig{
				HorizonURL:             strings.TrimRight(v.GetString("stellar.horizon_url"), "/"),
				NetworkPassphrase:      v.GetString("stellar.network_passphrase"),
				TreasuryAccount:        v.GetString("stellar.treasury_account"),
				OperatingBufferStroops: v.GetInt64("stellar.operating_buffer_stroops"),
				Timeout:                v.GetDuration("stellar.timeout"),
			},
			Worker: WorkerConfig{
				PollInterval:      v.GetDuration("worker.poll_interval"),
				LeaseDuration:     v.GetDuration("worker.lease_duration"),
				RetryDelay:        v.GetDuration("worker.retry_delay"),
				SubmissionTimeout: v.GetDuration("worker.submission_timeout"),
				MaxAttempts:       v.GetInt("worker.max_attempts"),
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadSettings() (*viper.Viper, error) {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)
	if err := readConfigFile(v); err != nil {
		return nil, err
	}
	bindEnvironment(v)
	return v, nil
}

// LoadWorkerTreasurySecret reads the worker-only Stellar treasury signing key.
// The secret is intentionally excluded from the shared configuration so the
// API process never loads it into memory.
func LoadWorkerTreasurySecret() (string, error) {
	v, err := loadSettings()
	if err != nil {
		return "", err
	}
	secret := strings.TrimSpace(v.GetString("stellar.treasury_secret"))
	if secret == "" {
		return "", errors.New("STELLAR_TREASURY_SECRET is required for the settlement worker")
	}
	return secret, nil
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
	if err := c.ObjectStorage.Validate(c.App.Environment); err != nil {
		return err
	}
	if err := c.Week1.Validate(); err != nil {
		return err
	}
	return nil
}

func (c ObjectStorageConfig) Validate(environment string) error {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" || strings.Contains(endpoint, "://") {
		return errors.New("MinIO endpoint must be host:port without a URL scheme")
	}
	parsed, err := url.Parse("http://" + endpoint)
	if err != nil || parsed.Host != endpoint || parsed.Path != "" {
		return errors.New("MinIO endpoint is invalid")
	}
	if strings.TrimSpace(c.AccessKey) == "" || strings.TrimSpace(c.SecretKey) == "" {
		return errors.New("MinIO credentials are required")
	}
	if !validBucketName(c.Bucket) {
		return errors.New("MinIO bucket name is invalid")
	}
	if strings.TrimSpace(c.Region) == "" {
		return errors.New("MinIO region is required")
	}
	if !strings.EqualFold(environment, "local") && !c.UseSSL {
		return errors.New("MinIO TLS must be enabled outside local")
	}
	return nil
}

func (c AuthConfig) Validate(environment string) error {
	if err := validateAuthURL("auth email link base URL", c.EmailLinkBaseURL, environment); err != nil {
		return err
	}
	if err := validateAuthURL("auth success redirect URL", c.SuccessRedirectURL, environment); err != nil {
		return err
	}
	google := c.Google
	if google.ClientID == "" && google.ClientSecret == "" {
		// Google sign-in is optional; empty means disabled.
	} else if google.ClientID == "" || google.ClientSecret == "" {
		return errors.New("google client id and secret must be configured together")
	} else if err := validateAuthURL("google redirect URL", google.RedirectURL, environment); err != nil {
		return err
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
	if c.AvatarMaxBytes <= 0 {
		return errors.New("auth avatar maximum bytes must be positive")
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
	v.SetDefault("auth.avatar_max_bytes", int64(5<<20))
	v.SetDefault("auth.email_link_base_url", "http://localhost:3001")
	v.SetDefault("minio.bucket", "kailopay-profile")
	v.SetDefault("minio.region", "us-east-1")
	v.SetDefault("minio.use_ssl", false)
	v.SetDefault("onramp.quote_ttl", 5*time.Minute)
	v.SetDefault("onramp.quote_max_age", 2*time.Minute)
	v.SetDefault("onramp.quote_spread_bps", 0)
	v.SetDefault("onramp.min_idr", int64(10_000))
	v.SetDefault("onramp.max_idr", int64(10_000_000))
	v.SetDefault("coinmarketcap.base_url", "https://pro-api.coinmarketcap.com")
	v.SetDefault("coinmarketcap.timeout", 5*time.Second)
	v.SetDefault("xendit.base_url", "https://api.xendit.co")
	v.SetDefault("xendit.api_version", "2024-11-11")
	v.SetDefault("xendit.qris_channel", "QRIS")
	v.SetDefault("xendit.va_channel", "BRI_VIRTUAL_ACCOUNT")
	v.SetDefault("xendit.timeout", 10*time.Second)
	v.SetDefault("stellar.horizon_url", "https://horizon-testnet.stellar.org")
	v.SetDefault("stellar.network_passphrase", StellarTestnetPassphrase)
	v.SetDefault("stellar.operating_buffer_stroops", int64(10_000_000))
	v.SetDefault("stellar.timeout", 10*time.Second)
	v.SetDefault("worker.poll_interval", time.Second)
	v.SetDefault("worker.lease_duration", 30*time.Second)
	v.SetDefault("worker.retry_delay", 5*time.Second)
	v.SetDefault("worker.submission_timeout", time.Minute)
	v.SetDefault("worker.max_attempts", 5)
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
		"app.environment":                  "APP_ENV",
		"app.version":                      "APP_VERSION",
		"http.address":                     "HTTP_ADDRESS",
		"http.read_header_timeout":         "HTTP_READ_HEADER_TIMEOUT",
		"http.read_timeout":                "HTTP_READ_TIMEOUT",
		"http.write_timeout":               "HTTP_WRITE_TIMEOUT",
		"http.idle_timeout":                "HTTP_IDLE_TIMEOUT",
		"http.shutdown_timeout":            "HTTP_SHUTDOWN_TIMEOUT",
		"health.check_timeout":             "HEALTH_CHECK_TIMEOUT",
		"database.dsn":                     "DATABASE_DSN",
		"database.max_open_conns":          "DATABASE_MAX_OPEN_CONNS",
		"database.max_idle_conns":          "DATABASE_MAX_IDLE_CONNS",
		"database.conn_max_lifetime":       "DATABASE_CONN_MAX_LIFETIME",
		"database.conn_max_idle_time":      "DATABASE_CONN_MAX_IDLE_TIME",
		"database.ping_timeout":            "DATABASE_PING_TIMEOUT",
		"logging.level":                    "LOG_LEVEL",
		"logging.format":                   "LOG_FORMAT",
		"auth.success_redirect_url":        "AUTH_SUCCESS_REDIRECT_URL",
		"auth.email_link_base_url":         "AUTH_EMAIL_LINK_BASE_URL",
		"auth.transaction_encryption_key":  "AUTH_TRANSACTION_ENCRYPTION_KEY",
		"auth.session_hmac_key":            "AUTH_SESSION_HMAC_KEY",
		"auth.session_absolute_lifetime":   "AUTH_SESSION_ABSOLUTE_LIFETIME",
		"auth.session_idle_lifetime":       "AUTH_SESSION_IDLE_LIFETIME",
		"auth.transaction_lifetime":        "AUTH_TRANSACTION_LIFETIME",
		"auth.cookie_name":                 "AUTH_COOKIE_NAME",
		"auth.cookie_secure":               "AUTH_COOKIE_SECURE",
		"auth.avatar_max_bytes":            "PROFILE_AVATAR_MAX_BYTES",
		"google.client_id":                 "GOOGLE_CLIENT_ID",
		"google.client_secret":             "GOOGLE_CLIENT_SECRET",
		"google.redirect_url":              "GOOGLE_REDIRECT_URL",
		"minio.endpoint":                   "MINIO_ENDPOINT",
		"minio.access_key":                 "MINIO_ACCESS_KEY",
		"minio.secret_key":                 "MINIO_SECRET_KEY",
		"minio.bucket":                     "MINIO_BUCKET",
		"minio.region":                     "MINIO_REGION",
		"minio.use_ssl":                    "MINIO_USE_SSL",
		"api_key.pepper":                   "API_KEY_PEPPER",
		"onramp.quote_ttl":                 "QUOTE_TTL",
		"onramp.quote_max_age":             "QUOTE_MAX_AGE",
		"onramp.quote_spread_bps":          "QUOTE_SPREAD_BPS",
		"onramp.min_idr":                   "ORDER_MIN_IDR",
		"onramp.max_idr":                   "ORDER_MAX_IDR",
		"coinmarketcap.base_url":           "COINMARKETCAP_BASE_URL",
		"coinmarketcap.api_key":            "COINMARKETCAP_API_KEY",
		"coinmarketcap.timeout":            "COINMARKETCAP_TIMEOUT",
		"xendit.base_url":                  "XENDIT_BASE_URL",
		"xendit.secret_key":                "XENDIT_SECRET_KEY",
		"xendit.callback_token":            "XENDIT_CALLBACK_TOKEN",
		"xendit.api_version":               "XENDIT_API_VERSION",
		"xendit.qris_channel":              "XENDIT_QRIS_CHANNEL",
		"xendit.va_channel":                "XENDIT_VA_CHANNEL",
		"xendit.timeout":                   "XENDIT_TIMEOUT",
		"stellar.horizon_url":              "STELLAR_HORIZON_URL",
		"stellar.network_passphrase":       "STELLAR_NETWORK_PASSPHRASE",
		"stellar.treasury_account":         "STELLAR_TREASURY_ACCOUNT",
		"stellar.treasury_secret":          "STELLAR_TREASURY_SECRET",
		"stellar.operating_buffer_stroops": "STELLAR_OPERATING_BUFFER_STROOPS",
		"stellar.timeout":                  "STELLAR_TIMEOUT",
		"worker.poll_interval":             "WORKER_POLL_INTERVAL",
		"worker.lease_duration":            "WORKER_LEASE_DURATION",
		"worker.retry_delay":               "WORKER_RETRY_DELAY",
		"worker.submission_timeout":        "WORKER_SUBMISSION_TIMEOUT",
		"worker.max_attempts":              "WORKER_MAX_ATTEMPTS",
	}
}

func validBucketName(name string) bool {
	if len(name) < 3 || len(name) > 63 || name != strings.ToLower(name) ||
		strings.Contains(name, "..") || strings.Contains(name, ".-") || strings.Contains(name, "-.") {
		return false
	}
	for index, char := range name {
		isAlphaNumeric := char >= 'a' && char <= 'z' || char >= '0' && char <= '9'
		if !isAlphaNumeric && char != '-' && char != '.' {
			return false
		}
		if (index == 0 || index == len(name)-1) && !isAlphaNumeric {
			return false
		}
	}
	return true
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
