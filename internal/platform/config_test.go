package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
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
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_DSN", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing database DSN error")
	}
}

func TestLoadReadsDotenvConfigFile(t *testing.T) {
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
