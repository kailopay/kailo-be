package platform

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	gormlogger "gorm.io/gorm/logger"
)

func TestOpenRejectsEmptyDSN(t *testing.T) {
	cfg := DBConfig{}

	_, _, err := Open(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("Open() error = nil, want empty DSN error")
	}
}

func TestGORMLoggerFormatsPrintfArgumentsForSlog(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	gormLogger := &gormSlogLogger{logger: logger, level: gormlogger.Error}

	gormLogger.Error(context.Background(), "failed to initialize database, got error %v", errors.New("connection refused"))

	line := output.String()
	if !strings.Contains(line, "msg=\"failed to initialize database, got error connection refused\"") {
		t.Fatalf("log line = %q, want formatted error message", line)
	}
	if strings.Contains(line, "!BADKEY") {
		t.Fatalf("log line contains malformed slog attribute: %q", line)
	}
}
