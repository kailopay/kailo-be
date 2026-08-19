package platform

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

func NewLogger(level, format string, output io.Writer) (*slog.Logger, error) {
	if output == nil {
		return nil, errors.New("logger output is nil")
	}

	minimum, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	options := &slog.HandlerOptions{
		Level:       minimum,
		ReplaceAttr: redactAttr,
	}
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		handler = slog.NewJSONHandler(output, options)
	case "text":
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}

	return slog.New(handler), nil
}

func WithMetadata(logger *slog.Logger, service, process, environment, version string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(
		slog.String("service", service),
		slog.String("process", process),
		slog.String("environment", environment),
		slog.String("release_version", version),
	)
}

func redactAttr(_ []string, attr slog.Attr) slog.Attr {
	key := strings.ToLower(strings.TrimSpace(attr.Key))
	key = strings.NewReplacer("-", "_", ".", "_").Replace(key)
	for _, sensitive := range []string{
		"password",
		"passwd",
		"secret",
		"token",
		"authorization",
		"cookie",
		"api_key",
		"apikey",
		"private_key",
		"dsn",
	} {
		if key == sensitive || strings.HasSuffix(key, "_"+sensitive) {
			return slog.String(attr.Key, "[REDACTED]")
		}
	}
	return attr
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", level)
	}
}
