package platform

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validAuthConfig() AuthConfig {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	return AuthConfig{
		EmailLinkBaseURL:         "https://app.example.com/",
		SuccessRedirectURL:       "https://app.example.com/",
		TransactionEncryptionKey: key,
		SessionHMACKey:           key,
		SessionAbsoluteLifetime:  8 * time.Hour,
		SessionIdleLifetime:      30 * time.Minute,
		TransactionLifetime:      10 * time.Minute,
		CookieName:               "__Host-kailopay_session",
		CookieSecure:             true,
		AvatarMaxBytes:           5 << 20,
	}
}

func validObjectStorageConfig() ObjectStorageConfig {
	return ObjectStorageConfig{
		Endpoint:  "minio.example.com:9000",
		AccessKey: "access-key",
		SecretKey: "secret-key",
		Bucket:    "kailopay-profile",
		Region:    "us-east-1",
		UseSSL:    true,
	}
}

func setValidAuthEnv(t *testing.T) {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	t.Setenv("AUTH_SUCCESS_REDIRECT_URL", "https://app.example.com/")
	t.Setenv("AUTH_EMAIL_LINK_BASE_URL", "https://app.example.com/")
	t.Setenv("AUTH_TRANSACTION_ENCRYPTION_KEY", key)
	t.Setenv("AUTH_SESSION_HMAC_KEY", key)
	t.Setenv("AUTH_COOKIE_SECURE", "true")
	t.Setenv("HTTP_ALLOWED_ORIGINS", "https://app.example.com")
	t.Setenv("MINIO_ENDPOINT", "minio.example.com:9000")
	t.Setenv("MINIO_ACCESS_KEY", "access-key")
	t.Setenv("MINIO_SECRET_KEY", "secret-key")
	t.Setenv("MINIO_BUCKET", "kailopay-profile")
	t.Setenv("MINIO_USE_SSL", "true")
	setValidWeek1Env(t)
}

func setValidWeek1Env(t *testing.T) {
	t.Helper()
	t.Setenv("API_KEY_PEPPER", "01234567890123456789012345678901")
	t.Setenv("COINMARKETCAP_API_KEY", "test-market-data-key")
	t.Setenv("XENDIT_SECRET_KEY", "xnd_development_test")
	t.Setenv("XENDIT_CALLBACK_TOKEN", "01234567890123456789012345678901")
	t.Setenv("STELLAR_TREASURY_ACCOUNT", "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF")
	t.Setenv("OFFRAMP_DEPOSIT_ACCOUNT", "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF")
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	setValidAuthEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDRESS", ":9090")
	t.Setenv("HTTP_ALLOWED_ORIGINS", "https://app.example.com")
	t.Setenv("DATABASE_DSN", "host=test-db user=tester password=secret dbname=test port=5432")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Environment != "test" {
		t.Fatalf("environment = %q, want %q", cfg.App.Environment, "test")
	}
	if cfg.HTTP.Address != ":9090" {
		t.Fatalf("address = %q, want %q", cfg.HTTP.Address, ":9090")
	}
	if cfg.Database.DSN == "" {
		t.Fatal("database DSN should be set")
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("log level = %q, want %q", cfg.Logging.Level, "debug")
	}
}

func TestLoadAcceptsOptionalGoogleAndReadsOverrides(t *testing.T) {
	setValidAuthEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_DSN", "host=test-db user=tester password=secret dbname=test port=5432")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() without google error = %v", err)
	}
	if cfg.Auth.Google.ClientID != "" {
		t.Fatalf("google client id = %q, want empty when unset", cfg.Auth.Google.ClientID)
	}

	t.Setenv("GOOGLE_CLIENT_ID", "google-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "google-client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "https://api.example.com/auth/google/callback")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() with google error = %v", err)
	}
	if cfg.Auth.Google.ClientID != "google-client-id" || cfg.Auth.Google.RedirectURL != "https://api.example.com/auth/google/callback" {
		t.Fatalf("google config = %+v", cfg.Auth.Google)
	}

	// Half-configured google credentials must fail startup.
	t.Setenv("GOOGLE_CLIENT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() with google client id only error = nil, want both-or-neither validation")
	}
}

func TestLoadRejectsMissingDatabaseDSN(t *testing.T) {
	setValidAuthEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_DSN", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing database DSN error")
	}
}

func TestLoadWorkerTreasurySecretRequiresSecret(t *testing.T) {
	if _, err := LoadWorkerTreasurySecret(); err == nil {
		t.Fatal("LoadWorkerTreasurySecret() error = nil, want required-secret error")
	}
	t.Setenv("STELLAR_TREASURY_SECRET", "SCZANGBA5YHTNYVVV4C3U5HISTYVZKUCJ5CZ2YMYFX4F7TRISPVZZ5AL")
	secret, err := LoadWorkerTreasurySecret()
	if err != nil {
		t.Fatalf("LoadWorkerTreasurySecret() error = %v", err)
	}
	if secret != "SCZANGBA5YHTNYVVV4C3U5HISTYVZKUCJ5CZ2YMYFX4F7TRISPVZZ5AL" {
		t.Fatalf("secret = %q", secret)
	}
}

func TestLoadReadsDotenvConfigFile(t *testing.T) {
	setValidAuthEnv(t)
	path := filepath.Join(t.TempDir(), ".env")
	contents := "APP_ENV=test\nDATABASE_DSN=host=file-db user=tester password=secret dbname=test port=5432\nLOG_FORMAT=text\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}
	t.Setenv("CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.App.Environment != "test" {
		t.Fatalf("environment = %q, want test", cfg.App.Environment)
	}
	if cfg.Database.DSN == "" {
		t.Fatal("database DSN should be loaded from config file")
	}
}

func TestAuthConfigValidateAcceptsProductionSettings(t *testing.T) {
	cfg := Config{App: AppConfig{Environment: "production"}, Auth: validAuthConfig()}

	if err := cfg.Auth.Validate(cfg.App.Environment); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestObjectStorageConfigValidateRejectsUnsafeSettings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ObjectStorageConfig)
	}{
		{name: "missing endpoint", mutate: func(cfg *ObjectStorageConfig) { cfg.Endpoint = "" }},
		{name: "endpoint includes scheme", mutate: func(cfg *ObjectStorageConfig) { cfg.Endpoint = "https://minio.example.com" }},
		{name: "missing credentials", mutate: func(cfg *ObjectStorageConfig) { cfg.SecretKey = "" }},
		{name: "invalid bucket", mutate: func(cfg *ObjectStorageConfig) { cfg.Bucket = "Invalid_Bucket" }},
		{name: "insecure outside local", mutate: func(cfg *ObjectStorageConfig) { cfg.UseSSL = false }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validObjectStorageConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate("production"); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestLoadReadsMinIOEnvironmentOverrides(t *testing.T) {
	setValidAuthEnv(t)
	t.Setenv("APP_ENV", "local")
	t.Setenv("DATABASE_DSN", "host=test-db user=tester password=secret dbname=test port=5432")
	t.Setenv("MINIO_ENDPOINT", "localhost:9000")
	t.Setenv("MINIO_USE_SSL", "false")
	t.Setenv("PROFILE_AVATAR_MAX_BYTES", "1048576")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ObjectStorage.Endpoint != "localhost:9000" || cfg.ObjectStorage.UseSSL {
		t.Fatalf("object storage endpoint/SSL = %q/%t", cfg.ObjectStorage.Endpoint, cfg.ObjectStorage.UseSSL)
	}
	if cfg.Auth.AvatarMaxBytes != 1048576 {
		t.Fatalf("avatar maximum bytes = %d", cfg.Auth.AvatarMaxBytes)
	}
}

func TestAuthConfigValidateRejectsInvalidSettings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AuthConfig)
	}{
		{name: "missing email link base URL", mutate: func(cfg *AuthConfig) { cfg.EmailLinkBaseURL = "" }},
		{name: "insecure email link base URL", mutate: func(cfg *AuthConfig) { cfg.EmailLinkBaseURL = "http://app.example.com" }},
		{name: "google client id without secret", mutate: func(cfg *AuthConfig) {
			cfg.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "", RedirectURL: "https://api.example.com/callback"}
		}},
		{name: "google missing redirect URL", mutate: func(cfg *AuthConfig) { cfg.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret"} }},
		{name: "invalid encryption key", mutate: func(cfg *AuthConfig) { cfg.TransactionEncryptionKey = "a" }},
		{name: "invalid hmac key", mutate: func(cfg *AuthConfig) { cfg.SessionHMACKey = "a" }},
		{name: "non-positive lifetime", mutate: func(cfg *AuthConfig) { cfg.SessionIdleLifetime = 0 }},
		{name: "insecure production cookie", mutate: func(cfg *AuthConfig) { cfg.CookieSecure = false }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validAuthConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate("production"); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestEmailConfigValidate(t *testing.T) {
	tests := []struct {
		name   string
		config EmailConfig
		valid  bool
	}{
		{name: "console provider", config: EmailConfig{Provider: "console"}, valid: true},
		{name: "gmail provider", config: EmailConfig{Provider: "gmail", Gmail: GmailConfig{
			Username: "sender@gmail.com", AppPassword: "abcdefghijklmnop", Timeout: time.Second,
		}}, valid: true},
		{name: "unsupported provider", config: EmailConfig{Provider: "smtp"}},
		{name: "gmail missing username", config: EmailConfig{Provider: "gmail", Gmail: GmailConfig{
			AppPassword: "abcdefghijklmnop", Timeout: time.Second,
		}}},
		{name: "gmail missing app password", config: EmailConfig{Provider: "gmail", Gmail: GmailConfig{
			Username: "sender@gmail.com", Timeout: time.Second,
		}}},
		{name: "gmail missing timeout", config: EmailConfig{Provider: "gmail", Gmail: GmailConfig{
			Username: "sender@gmail.com", AppPassword: "abcdefghijklmnop",
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.valid && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !tt.valid && err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestLoadReadsEmailEnvironmentOverrides(t *testing.T) {
	setValidAuthEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_DSN", "host=test-db user=tester password=secret dbname=test port=5432")
	t.Setenv("EMAIL_PROVIDER", "gmail")
	t.Setenv("GMAIL_USERNAME", "sender@gmail.com")
	t.Setenv("GMAIL_APP_PASSWORD", "abcdefghijklmnop")
	t.Setenv("GMAIL_FROM_NAME", "KailoPay")
	t.Setenv("GMAIL_SMTP_TIMEOUT", "7s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Email.Provider != "gmail" || cfg.Email.Gmail.Username != "sender@gmail.com" ||
		cfg.Email.Gmail.FromName != "KailoPay" || cfg.Email.Gmail.Timeout != 7*time.Second {
		t.Fatalf("email config = %+v", cfg.Email)
	}
}

func TestLoadReadsAuthEnvironmentOverrides(t *testing.T) {
	setValidWeek1Env(t)
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	t.Setenv("APP_ENV", "local")
	t.Setenv("DATABASE_DSN", "host=test-db user=tester password=secret dbname=test port=5432")
	t.Setenv("AUTH_SUCCESS_REDIRECT_URL", "http://localhost:3000/")
	t.Setenv("AUTH_TRANSACTION_ENCRYPTION_KEY", key)
	t.Setenv("AUTH_SESSION_HMAC_KEY", key)
	t.Setenv("AUTH_COOKIE_SECURE", "false")
	t.Setenv("MINIO_ENDPOINT", "localhost:9000")
	t.Setenv("MINIO_ACCESS_KEY", "access-key")
	t.Setenv("MINIO_SECRET_KEY", "secret-key")
	t.Setenv("MINIO_BUCKET", "kailopay-profile")
	t.Setenv("MINIO_USE_SSL", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.EmailLinkBaseURL != "http://localhost:3001" {
		t.Fatalf("email link base URL = %q, want default", cfg.Auth.EmailLinkBaseURL)
	}
	if cfg.Auth.SessionAbsoluteLifetime != 8*time.Hour {
		t.Fatalf("absolute lifetime = %v", cfg.Auth.SessionAbsoluteLifetime)
	}
	if cfg.Auth.CookieSecure {
		t.Fatal("cookie secure = true, want false for local")
	}
}

func TestLoadNormalizesAllowedOrigins(t *testing.T) {
	setValidAuthEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_DSN", "host=test-db user=tester password=secret dbname=test port=5432")
	t.Setenv("HTTP_ALLOWED_ORIGINS", " https://App.Example.com/ , https://app.example.com,https://admin.example.com ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{"https://app.example.com", "https://admin.example.com"}
	if len(cfg.HTTP.AllowedOrigins) != len(want) {
		t.Fatalf("allowed origins = %#v, want %#v", cfg.HTTP.AllowedOrigins, want)
	}
	for index := range want {
		if cfg.HTTP.AllowedOrigins[index] != want[index] {
			t.Fatalf("allowed origins = %#v, want %#v", cfg.HTTP.AllowedOrigins, want)
		}
	}
}

func TestHTTPConfigValidateRejectsUnsafeAllowedOrigins(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		origins     []string
	}{
		{name: "missing origin", environment: "production"},
		{name: "insecure production origin", environment: "production", origins: []string{"http://app.example.com"}},
		{name: "origin has path", environment: "production", origins: []string{"https://app.example.com/checkout"}},
		{name: "origin has query", environment: "production", origins: []string{"https://app.example.com?state=1"}},
		{name: "unsupported scheme", environment: "production", origins: []string{"ftp://app.example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := (HTTPConfig{AllowedOrigins: tt.origins}).Validate(tt.environment); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestHTTPConfigValidateAcceptsLocalHTTPOrigin(t *testing.T) {
	if err := (HTTPConfig{AllowedOrigins: []string{"http://localhost:3000"}}).Validate("local"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
