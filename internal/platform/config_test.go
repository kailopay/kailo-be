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
		IssuerURL:                "https://tenant.example.com/",
		ClientID:                 "client-id",
		ClientSecret:             "client-secret",
		RedirectURL:              "https://api.example.com/auth/callback",
		EmailConnection:          "email",
		SuccessRedirectURL:       "https://app.example.com/",
		TransactionEncryptionKey: key,
		SessionHMACKey:           key,
		SessionAbsoluteLifetime:  8 * time.Hour,
		SessionIdleLifetime:      30 * time.Minute,
		TransactionLifetime:      10 * time.Minute,
		CookieName:               "__Host-kailopay_session",
		CookieSecure:             true,
	}
}

func setValidAuthEnv(t *testing.T) {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	t.Setenv("AUTH0_ISSUER_URL", "https://tenant.example.com/")
	t.Setenv("AUTH0_CLIENT_ID", "client-id")
	t.Setenv("AUTH0_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH0_REDIRECT_URL", "https://api.example.com/auth/callback")
	t.Setenv("AUTH0_EMAIL_CONNECTION", "email")
	t.Setenv("AUTH_SUCCESS_REDIRECT_URL", "https://app.example.com/")
	t.Setenv("AUTH_TRANSACTION_ENCRYPTION_KEY", key)
	t.Setenv("AUTH_SESSION_HMAC_KEY", key)
	t.Setenv("AUTH_COOKIE_SECURE", "true")
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	setValidAuthEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDRESS", ":9090")
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

func TestLoadRejectsMissingDatabaseDSN(t *testing.T) {
	setValidAuthEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_DSN", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing database DSN error")
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

func TestAuthConfigValidateRejectsInvalidSettings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AuthConfig)
	}{
		{name: "missing issuer", mutate: func(cfg *AuthConfig) { cfg.IssuerURL = "" }},
		{name: "invalid issuer", mutate: func(cfg *AuthConfig) { cfg.IssuerURL = "http://tenant.example.com" }},
		{name: "missing client credentials", mutate: func(cfg *AuthConfig) { cfg.ClientID = "" }},
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

func TestLoadReadsAuthEnvironmentOverrides(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	t.Setenv("APP_ENV", "local")
	t.Setenv("DATABASE_DSN", "host=test-db user=tester password=secret dbname=test port=5432")
	t.Setenv("AUTH0_ISSUER_URL", "https://tenant.example.com/")
	t.Setenv("AUTH0_CLIENT_ID", "client-id")
	t.Setenv("AUTH0_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH0_REDIRECT_URL", "https://api.example.com/auth/callback")
	t.Setenv("AUTH0_EMAIL_CONNECTION", "email")
	t.Setenv("AUTH_SUCCESS_REDIRECT_URL", "http://localhost:3000/")
	t.Setenv("AUTH_TRANSACTION_ENCRYPTION_KEY", key)
	t.Setenv("AUTH_SESSION_HMAC_KEY", key)
	t.Setenv("AUTH_COOKIE_SECURE", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.IssuerURL != "https://tenant.example.com" {
		t.Fatalf("issuer = %q", cfg.Auth.IssuerURL)
	}
	if cfg.Auth.SessionAbsoluteLifetime != 8*time.Hour {
		t.Fatalf("absolute lifetime = %v", cfg.Auth.SessionAbsoluteLifetime)
	}
	if cfg.Auth.CookieSecure {
		t.Fatal("cookie secure = true, want false for local")
	}
}
